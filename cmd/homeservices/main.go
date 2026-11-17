package main

import (
	"bufio"
	"compress/gzip"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/Dumhouse/homepage/internal/alerts"
	"github.com/Dumhouse/homepage/internal/auth"
	"github.com/Dumhouse/homepage/internal/checks"
	"github.com/Dumhouse/homepage/internal/db"
	"github.com/Dumhouse/homepage/internal/digest"
	"github.com/Dumhouse/homepage/internal/discovery"
	"github.com/Dumhouse/homepage/internal/expiry"
	"github.com/Dumhouse/homepage/internal/facts"
	"github.com/Dumhouse/homepage/internal/fx"
	"github.com/Dumhouse/homepage/internal/integrations"
	"github.com/Dumhouse/homepage/internal/inventory"
	"github.com/Dumhouse/homepage/internal/power"
	"github.com/Dumhouse/homepage/internal/settings"
	"github.com/Dumhouse/homepage/internal/tokens"
	"github.com/Dumhouse/homepage/internal/web"
	"golang.org/x/term"
)

// version is set by the release build: -ldflags "-X main.version=v0.1.0".
var version = "dev"

func main() {
	web.Version = version
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "help", "-h", "--help":
			usage(os.Stdout)
			return
		case "version":
			fmt.Println(version)
			return
		case "migrate":
			migrateCmd(os.Args[2:])
			return
		case "backup":
			backupCmd(os.Args[2:])
			return
		case "scan":
			scanCmd(os.Args[2:])
			return
		case "demo":
			demoCmd(os.Args[2:])
			return
		case "import":
			importCmd(os.Args[2:])
			return
		case "hash-password":
			fmt.Fprint(os.Stderr, "Password: ")
			var pw string
			if term.IsTerminal(int(os.Stdin.Fd())) {
				b, err := term.ReadPassword(int(os.Stdin.Fd()))
				fmt.Fprintln(os.Stderr)
				if err != nil {
					log.Fatal(err)
				}
				pw = string(b)
			} else {
				pw, _ = bufio.NewReader(os.Stdin).ReadString('\n')
			}
			h, err := auth.HashPassword(strings.TrimRight(pw, "\r\n"))
			if err != nil {
				log.Fatal(err)
			}
			fmt.Println(h)
			return
		case "token":
			tokenCmd(os.Args[2:])
			return
		default:
			if !strings.HasPrefix(os.Args[1], "-") {
				fmt.Fprintf(os.Stderr, "unknown command %q\n\n", os.Args[1])
				usage(os.Stderr)
				os.Exit(2)
			}
		}
	}
	addr := flag.String("addr", env("HS_ADDR", ":8080"), "listen address")
	data := flag.String("data", env("HS_DATA", "./data"), "data directory (SQLite lives here)")
	flag.Parse()

	d := openDB(*data)
	defer d.Close()

	ap, err := auth.New(auth.Config{
		Mode: os.Getenv("HS_AUTH"), PasswordHash: os.Getenv("HS_PASSWORD_HASH"),
		SessionKey: []byte(os.Getenv("HS_SESSION_KEY")), Secure: os.Getenv("HS_COOKIE_SECURE") == "1",
	})
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("UI auth: %s", ap.Name())
	inv := inventory.New(d)
	sched := checks.NewScheduler(d, inv)
	secrets, err := integrations.NewFileSecrets(filepath.Join(*data, "secrets.enc"), os.Getenv("HS_MASTER_KEY"))
	if errors.Is(err, integrations.ErrNoMasterKey) {
		log.Printf("HS_MASTER_KEY not set: integration secrets are in-memory only")
		secrets = integrations.NewMemorySecrets()
	} else if err != nil {
		log.Fatal(err)
	}
	im := integrations.NewManager(d, secrets)
	im.Register(integrations.Caddy())
	im.Register(integrations.Proxmox())
	im.Register(integrations.Cloudflare())
	im.Register(integrations.Docker())
	im.Register(integrations.Hetzner())
	im.Register(integrations.DigitalOcean())
	im.Register(integrations.OVH())
	im.Register(integrations.Tailscale())
	im.Register(integrations.Headscale())
	im.Register(integrations.Prometheus())
	im.Register(integrations.Netdata())
	im.Register(integrations.Beszel())
	im.Register(integrations.Glances())
	im.Register(integrations.TrueNAS())
	im.Register(integrations.Synology())
	im.Register(integrations.OpenMediaVault())
	im.Register(integrations.NUT())
	im.Register(integrations.Redfish())
	im.Register(integrations.Scrutiny())
	im.Register(integrations.UniFi())
	im.Register(integrations.OPNsense())
	im.Register(integrations.MikroTik())
	im.Register(integrations.OpenWrt())
	im.Register(integrations.PiHole())
	im.Register(integrations.AdGuard())
	im.Register(integrations.NginxProxyManager())
	im.Register(integrations.Traefik())
	im.Register(integrations.Power())
	im.Register(integrations.Ntfy())
	im.Register(integrations.Gotify())
	im.Register(integrations.Webhook())

	// Alerts go to HS_NTFY_URL / HS_WEBHOOK_URL (if set) and to every
	// notification plugin set up in Settings → Integrations.
	dispatcher := alerts.NewDispatcher(alerts.FromEnv(), alerts.MinDownFromEnv())
	dispatcher.Source = im.Notifiers
	sched.SetNotifier(dispatcher)

	scanner := discovery.NewRunner(inv, settings.New(d))
	scanner.OnNew = func(ctx context.Context, address, hostname string, ports []inventory.PortInfo) {
		label, list := address, []string{}
		if hostname != "" {
			label = hostname + " (" + address + ")"
		}
		for _, p := range ports {
			list = append(list, strconv.Itoa(p.Port))
		}
		_ = dispatcher.Broadcast(ctx, alerts.Event{Entity: "discovery", Label: label, State: "discovered",
			Message: "Open ports: " + strings.Join(list, ", ") + ". Register or ignore it on the Discovery page.", At: time.Now()})
	}
	pw := power.NewStore(d)
	opts := []web.Option{web.WithPower(pw), web.WithChecker(sched), web.WithDataDir(*data),
		web.WithIntegrations(im), web.WithScanner(scanner), web.WithTrustedProxies(os.Getenv("HS_TRUSTED_PROXIES"))}
	if key := os.Getenv("HS_SSH_KEY"); key != "" {
		opts = append(opts, web.WithFacts(facts.New(key, env("HS_SSH_FACTS_USER", "inventory"))))
	}
	h, err := web.New(d, ap, opts...)
	if err != nil {
		log.Fatal(err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	stopChecks := sched.Start(ctx)
	defer stopChecks()
	stopScans := scanner.Loop(ctx)
	defer stopScans()
	stopFX := fx.Loop(ctx, settings.New(d))
	defer stopFX()
	stopPower := pw.Loop(ctx, func(ctx context.Context) power.Config { return web.PowerConfig(ctx, im) })
	defer stopPower()
	stopExpiry := expiry.Start(ctx, inv, settings.New(d), dispatcher, sched.Store(), sched.Label)
	defer stopExpiry()
	if os.Getenv("HS_DIGEST") != "off" {
		stopDigest := startDigest(ctx, d, inv, sched, dispatcher)
		defer stopDigest()
	}

	srv := &http.Server{Addr: *addr, Handler: h, ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdown)
	}()
	log.Printf("Home Services listening on %s (data: %s)", *addr, *data)
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
	log.Print("stopped")
}

func usage(w io.Writer) {
	fmt.Fprint(w, `Home Services — home-lab inventory in one binary.

usage:
  homeservices [-addr :8080] [-data ./data]     start the server
  homeservices version
  homeservices migrate [-check] [-data dir]
  homeservices backup [-keep 14] [-data dir]
  homeservices scan -subnet 192.168.1.0/24 [-ports 22,80,443] [-data dir]
  homeservices demo [-data dir]
  homeservices import [-apply] [-mode merge|replace] [-data dir] file.json
  homeservices token create --name <name> [--scope read|propose|write] [-data dir]
  homeservices token list|revoke <name> [-data dir]
  homeservices hash-password

Environment: HS_ADDR, HS_DATA, HS_AUTH, HS_PASSWORD_HASH, HS_SESSION_KEY,
HS_COOKIE_SECURE, HS_MASTER_KEY, HS_NTFY_URL, HS_NTFY_TOKEN, HS_WEBHOOK_URL,
HS_ALERTS_MIN_DOWN, HS_DIGEST, HS_DIGEST_DAYS, HS_SSH_KEY, HS_SSH_FACTS_USER.
`)
}

// tokenCmd implements `homeservices token create|list|revoke` so tokens work
// before any settings page exists.
func tokenCmd(args []string) {
	if len(args) == 0 {
		tokenUsage()
	}
	fs := flag.NewFlagSet("token "+args[0], flag.ExitOnError)
	dataDir := dataFlag(fs)
	name := fs.String("name", "", "token name, e.g. claude-code (create)")
	scope := fs.String("scope", "read", "read, propose or write (create)")
	switch args[0] {
	case "create", "list", "revoke":
	default:
		tokenUsage()
	}
	fs.Parse(args[1:])
	// Flags are parsed before the database is opened, so a typo never creates ./data.
	d := openDB(*dataDir)
	defer d.Close()
	ts := tokens.New(d)
	ctx := context.Background()

	switch args[0] {
	case "create":
		t, err := ts.Create(ctx, *name, *scope)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Println(t.Plaintext)
		fmt.Fprintf(os.Stderr, "Created %q (%s scope, id %s). The token is shown once — store it in Vaultwarden.\n",
			t.Name, t.Scope, t.PublicID)
	case "list":
		list, err := ts.List(ctx)
		if err != nil {
			log.Fatal(err)
		}
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		fmt.Fprintln(w, "NAME\tSCOPE\tCREATED\tLAST USED\tSTATE")
		for _, t := range list {
			last := "never"
			if t.LastUsedAt != nil {
				last = *t.LastUsedAt
			}
			state := "active"
			if t.RevokedAt != nil {
				state = "revoked " + *t.RevokedAt
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", t.Name, t.Scope, t.CreatedAt, last, state)
		}
		w.Flush()
	case "revoke":
		if fs.NArg() < 1 {
			tokenUsage()
		}
		if err := ts.Revoke(ctx, fs.Arg(0)); err != nil {
			log.Fatal(err)
		}
		fmt.Fprintf(os.Stderr, "Revoked %q.\n", fs.Arg(0))
	default:
		tokenUsage()
	}
}

func tokenUsage() {
	fmt.Fprint(os.Stderr, `usage:
  homeservices token create --name <name> [--scope read|propose|write]
  homeservices token list
  homeservices token revoke [-data dir] <name>

Every command accepts -data <dir> (default: $HS_DATA, else ./data).
`)
	os.Exit(2)
}

// startDigest sends a summary every HS_DIGEST_DAYS (default 7) through the same
// notifiers. The first run only records a baseline, so a new install waits a
// full period before the first digest.
func startDigest(ctx context.Context, d *sql.DB, inv *inventory.Store, sched *checks.Scheduler, dispatcher *alerts.Dispatcher) func() {
	every := 7 * 24 * time.Hour
	if v := os.Getenv("HS_DIGEST_DAYS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			every = time.Duration(n) * 24 * time.Hour
		}
	}
	set := settings.New(d)
	send := func() {
		now := time.Now().UTC()
		raw, _ := set.Get(ctx, "digest_last_at", "")
		last, _ := strconv.ParseInt(raw, 10, 64)
		if last == 0 {
			_ = set.Set(ctx, "system:digest", "digest_last_at", strconv.FormatInt(now.Unix(), 10))
			return
		}
		if !digest.ShouldSend(now, last, every) {
			return
		}
		stats, _ := inv.Stats(ctx)
		cc, _ := sched.Store().CheckCounts(ctx)
		probs, _ := sched.Store().ListProblems(ctx, 20)
		windows, _ := sched.Store().ListMaintenance(ctx)
		active := 0
		for _, w := range windows {
			if w.Active {
				active++
			}
		}
		in := digest.Input{
			At: now,
			Counts: digest.Counts{
				Hosts: stats.Hosts, Services: stats.Services, Projects: stats.Projects, Locations: stats.Locations,
			},
			Checks: digest.CheckCounts{
				Up: cc.Up, Failing: cc.Failing, Down: cc.Down, Recovering: cc.Recovering,
				Unknown: cc.Unknown, Disabled: cc.Disabled,
			},
			Maintenance: active,
		}
		if expiring, err := inv.ExpiringDomains(ctx, 30); err == nil {
			for _, dd := range expiring {
				in.Expiring = append(in.Expiring, fmt.Sprintf("%s — %d days (%s)", dd.Name, dd.DaysLeft, dd.ExpiresAt))
			}
		}
		for _, p := range probs {
			in.Problems = append(in.Problems, digest.Problem{
				Title: sched.Label(ctx, p.TargetEntity, p.TargetID), State: p.State, Message: p.LastError,
			})
		}
		ev := alerts.Event{
			Entity: "digest", State: "digest", Label: digest.Subject(),
			Message: digest.Build(in), At: now,
		}
		if err := dispatcher.Broadcast(ctx, ev); err != nil {
			log.Printf("digest: %v", err)
		}
		_ = set.Set(ctx, "system:digest", "digest_last_at", strconv.FormatInt(now.Unix(), 10))
	}
	ctx, cancel := context.WithCancel(ctx)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		send()
		t := time.NewTicker(time.Hour)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				send()
			}
		}
	}()
	return func() { cancel(); wg.Wait() }
}

// migrateCmd applies migrations, or with --check only reports pending ones
// (exit 2) so update.sh can dry-run before replacing the binary.
func migrateCmd(args []string) {
	fs := flag.NewFlagSet("migrate", flag.ExitOnError)
	dataDir := dataFlag(fs)
	check := fs.Bool("check", false, "only report the schema version")
	fs.Parse(args)
	data := *dataDir
	path := filepath.Join(data, "homeservices.db")
	if *check {
		if err := os.MkdirAll(data, 0o750); err != nil {
			log.Fatal(err)
		}
		current, latest, err := db.Pending(path)
		if err != nil {
			log.Fatal(err)
		}
		fmt.Printf("schema %d of %d\n", current, latest)
		if current < latest {
			os.Exit(2)
		}
		return
	}
	d := openDB(data)
	defer d.Close()
	fmt.Println("migrations applied")
}

// scanCmd sweeps a subnet and stores results in the discovery inbox.
func scanCmd(args []string) {
	fs := flag.NewFlagSet("scan", flag.ExitOnError)
	dataDir := dataFlag(fs)
	subnet := fs.String("subnet", env("HS_SCAN_SUBNET", ""), "CIDR to scan, e.g. 192.168.1.0/24")
	portsFlag := fs.String("ports", "", "comma-separated port override")
	fs.Parse(args)
	if *subnet == "" {
		log.Fatal("usage: homeservices scan -subnet 192.168.1.0/24 [-ports 22,80,443]")
	}
	s := discovery.New()
	if *portsFlag != "" {
		s.Ports = nil
		for _, p := range strings.Split(*portsFlag, ",") {
			if n, err := strconv.Atoi(strings.TrimSpace(p)); err == nil && n > 0 && n < 65536 {
				s.Ports = append(s.Ports, n)
			}
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	fmt.Printf("scanning %s (%d ports)…\n", *subnet, len(s.Ports))
	results, err := s.Scan(ctx, *subnet)
	if err != nil {
		log.Fatal(err)
	}
	d := openDB(*dataDir)
	defer d.Close()
	inv := inventory.New(d)
	newHosts, newPorts := 0, 0
	for _, r := range results {
		nh, np, err := inv.UpsertDiscovery(ctx, r.Address, r.Hostname, r.Ports)
		if err != nil {
			log.Printf("%s: %v", r.Address, err)
			continue
		}
		newHosts += nh
		newPorts += np
		names := make([]string, 0, len(r.Ports))
		for _, p := range r.Ports {
			name := p.Service
			if name == "" {
				name = strconv.Itoa(p.Port)
			}
			names = append(names, name+"/"+strconv.Itoa(p.Port))
		}
		fmt.Printf("%-15s %-24s %s\n", r.Address, r.Hostname, strings.Join(names, " "))
	}
	fmt.Printf("\n%d host(s) with open ports · %d new host(s) · %d new port(s) — open /discovery\n",
		len(results), newHosts, newPorts)
}

// backupCmd writes an online SQLite copy to data/backups, gzipped, keeping the
// newest N files. Safe to run while the service is up.
func backupCmd(args []string) {
	fs := flag.NewFlagSet("backup", flag.ExitOnError)
	dataDir := dataFlag(fs)
	keep := fs.Int("keep", 14, "how many backups to keep")
	fs.Parse(args)
	data := *dataDir
	dbPath := filepath.Join(data, "homeservices.db")
	if _, err := os.Stat(dbPath); err != nil {
		log.Fatalf("database not found at %s", dbPath)
	}
	// No migrations here: a backup must never change the schema, especially
	// during update.sh's dry-run before the new binary starts.
	d, err := db.OpenNoMigrate(dbPath)
	if err != nil {
		log.Fatal(err)
	}
	defer d.Close()

	dir := filepath.Join(data, "backups")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		log.Fatal(err)
	}
	stamp := time.Now().UTC().Format("20060102-150405")
	tmp := filepath.Join(dir, ".tmp-"+stamp+".db")
	defer os.Remove(tmp)
	if _, err := d.Exec(`VACUUM INTO ?`, tmp); err != nil {
		log.Fatal(err)
	}
	out := filepath.Join(dir, "homeservices-"+stamp+".db.gz")
	if err := gzipFile(tmp, out); err != nil {
		log.Fatal(err)
	}
	fmt.Println(out)
	pruneBackups(dir, "homeservices-*.db.gz", *keep)

	// Integration tokens live outside SQLite; without this file a restore
	// leaves every integration disconnected.
	if _, err := os.Stat(filepath.Join(data, "secrets.enc")); err == nil {
		secretOut := filepath.Join(dir, "homeservices-"+stamp+".secrets.enc.gz")
		if err := gzipFile(filepath.Join(data, "secrets.enc"), secretOut); err != nil {
			log.Fatal(err)
		}
		fmt.Println(secretOut)
		pruneBackups(dir, "homeservices-*.secrets.enc.gz", *keep)
	}
}

func pruneBackups(dir, pattern string, keep int) {
	files, _ := filepath.Glob(filepath.Join(dir, pattern))
	sort.Strings(files)
	for len(files) > keep {
		os.Remove(files[0])
		files = files[1:]
	}
}

func gzipFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	gz := gzip.NewWriter(out)
	if _, err := io.Copy(gz, in); err != nil {
		out.Close()
		return err
	}
	if err := gz.Close(); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

// dataFlag adds the shared -data flag to a subcommand (default $HS_DATA, else ./data).
func dataFlag(fs *flag.FlagSet) *string {
	return fs.String("data", env("HS_DATA", "./data"), "data directory (SQLite lives here)")
}

func openDB(dataDir string) *sql.DB {
	if err := os.MkdirAll(dataDir, 0o750); err != nil {
		log.Fatal(err)
	}
	d, err := db.Open(filepath.Join(dataDir, "homeservices.db"))
	if err != nil {
		log.Fatal(err)
	}
	return d
}

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

// importCmd loads an export document (the same JSON as /export/inventory.json
// and /api/v1/import) straight into the database, for seeding a fresh install
// from a file without creating an API token. Dry run unless -apply is given.
func importCmd(args []string) {
	fs := flag.NewFlagSet("import", flag.ExitOnError)
	dataDir := dataFlag(fs)
	apply := fs.Bool("apply", false, "write the changes (default: only print what would change)")
	mode := fs.String("mode", "merge", "merge (match by name/slug, add the rest) or replace (wipe the inventory first)")
	fs.Parse(args)
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: homeservices import [-apply] [-mode merge|replace] [-data dir] file.json")
		os.Exit(2)
	}
	b, err := os.ReadFile(fs.Arg(0))
	if err != nil {
		log.Fatal(err)
	}
	var in inventory.ExportData
	if err := json.Unmarshal(b, &in); err != nil {
		log.Fatalf("%s: %v", fs.Arg(0), err)
	}
	d := openDB(*dataDir)
	defer d.Close()
	sum, err := inventory.New(d).Import(context.Background(), "cli:import", &in, *mode, !*apply)
	if err != nil {
		log.Fatal(err)
	}
	out, _ := json.MarshalIndent(sum, "", "  ")
	fmt.Println(string(out))
	if !*apply {
		fmt.Fprintln(os.Stderr, "dry run — nothing written; add -apply to import")
	}
}
