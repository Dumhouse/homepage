// Package discovery sweeps a subnet for open TCP ports and guesses services.
// It is read-only and bounded: /24-sized networks by default, short timeouts,
// a small worker pool, and no packets beyond a TCP connect/banner/HTTP title.
package discovery

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/dawidof/home-services/internal/inventory"
)

var DefaultPorts = []int{22, 80, 443, 2019, 3000, 5432, 6379, 8006, 8080, 8081, 8443, 9000, 9090, 32400}

var portNames = map[int]string{
	22: "ssh", 80: "http", 443: "https", 2019: "caddy", 3000: "app", 5432: "postgres",
	6379: "redis", 8006: "proxmox", 8080: "http-alt", 8081: "http-alt", 8443: "https-alt",
	9000: "portainer", 9090: "prometheus", 32400: "plex", 1883: "mqtt", 445: "smb",
	3306: "mysql", 27017: "mongodb",
}

var webPorts = map[int]bool{80: true, 443: true, 2019: true, 3000: true, 8006: true, 8080: true, 8081: true, 8443: true, 9000: true, 9090: true, 32400: true}

type Scanner struct {
	Timeout  time.Duration
	Workers  int
	Ports    []int
	WebPorts map[int]bool // extra ports to try HTTP titles on
	// Progress, when set, is called as port probes finish (done of total).
	Progress func(done, total int)
	// Identified, when set, is called as each responding address has been
	// identified (reverse DNS, page titles, banners): done of total.
	Identified func(done, total int)
}

func New() *Scanner {
	return &Scanner{Timeout: 400 * time.Millisecond, Workers: 96, Ports: DefaultPorts}
}

type Result struct {
	Address  string
	Hostname string
	Ports    []inventory.PortInfo
}

// Scan returns hosts with at least one open port.
func (s *Scanner) Scan(ctx context.Context, subnet string) ([]Result, error) {
	_, network, err := net.ParseCIDR(strings.TrimSpace(subnet))
	if err != nil {
		return nil, fmt.Errorf("%w: bad subnet %q", inventory.ErrInvalid, subnet)
	}
	ips := hostsIn(network)
	if len(ips) == 0 || len(ips) > 1024 {
		return nil, fmt.Errorf("%w: subnet must contain 1–1024 addresses", inventory.ErrInvalid)
	}
	ports := s.Ports
	if len(ports) == 0 {
		ports = DefaultPorts
	}
	workers := s.Workers
	if workers <= 0 {
		workers = 64
	}
	type job struct {
		ip   string
		port int
	}
	type hit struct {
		ip   string
		port int
	}
	jobs := make(chan job)
	hits := make(chan hit)
	total := len(ips) * len(ports)
	var probed atomic.Int64
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				d := net.Dialer{Timeout: s.Timeout}
				c, err := d.DialContext(ctx, "tcp", net.JoinHostPort(j.ip, strconv.Itoa(j.port)))
				if n := probed.Add(1); s.Progress != nil && (n%32 == 0 || int(n) == total) {
					s.Progress(int(n), total)
				}
				if err != nil {
					continue
				}
				c.Close()
				hits <- hit(j)
			}
		}()
	}
	go func() {
	feed:
		for _, ip := range ips {
			for _, p := range ports {
				select {
				case <-ctx.Done():
					break feed
				case jobs <- job{ip: ip, port: p}:
				}
			}
		}
		close(jobs)
		wg.Wait()
		close(hits)
	}()

	open := map[string][]int{}
	for h := range hits {
		open[h.ip] = append(open[h.ip], h.port)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	// Identify responding addresses in parallel: reverse DNS and HTTP titles
	// can each take seconds, so doing them one by one stalled big subnets.
	type found struct {
		ip    string
		ports []int
	}
	todo := make(chan found)
	results := make(chan Result)
	var idWG sync.WaitGroup
	// progress counts one step per address (name lookup) and per open port
	units := 0
	for _, ports := range open {
		units += 1 + len(ports)
	}
	var identified atomic.Int64
	tick := func() {
		if n := identified.Add(1); s.Identified != nil {
			s.Identified(int(n), units)
		}
	}
	if s.Identified != nil {
		s.Identified(0, units)
	}
	for i := 0; i < 16; i++ {
		idWG.Add(1)
		go func() {
			defer idWG.Done()
			for f := range todo {
				results <- s.identify(ctx, f.ip, f.ports, tick)
			}
		}()
	}
	go func() {
		for ip, ports := range open {
			todo <- found{ip, ports}
		}
		close(todo)
		idWG.Wait()
		close(results)
	}()

	out := []Result{}
	for res := range results {
		out = append(out, res)
	}
	sort.Slice(out, func(i, j int) bool {
		return bytesLess(out[i].Address, out[j].Address)
	})
	return out, ctx.Err()
}

// identify reads the name, page titles and banners of one address.
func (s *Scanner) identify(ctx context.Context, ip string, ports []int, tick func()) Result {
	{
		sort.Ints(ports)
		res := Result{Address: ip}
		lctx, cancel := context.WithTimeout(ctx, 2*time.Second)
		names, err := net.DefaultResolver.LookupAddr(lctx, ip)
		cancel()
		if err == nil && len(names) > 0 {
			res.Hostname = strings.TrimSuffix(names[0], ".")
		}
		tick()
		for _, p := range ports {
			info := inventory.PortInfo{Port: p, Service: portNames[p]}
			if webPorts[p] || (s.WebPorts != nil && s.WebPorts[p]) {
				title, scheme := fetchTitle(ctx, ip, p)
				info.Title = title
				switch {
				case scheme == "https" && !strings.HasPrefix(info.Service, "https") && info.Service != "proxmox":
					info.Service = "https" // remember TLS so the inbox links open the right scheme
				case info.Service == "" && scheme != "":
					info.Service = "http"
				}
			} else {
				info.Banner = grabBanner(ip, p, s.Timeout)
			}
			if svc := fingerprintService(p, info.Title, info.Banner); svc != "" {
				info.Service = svc
			}
			res.Ports = append(res.Ports, info)
			tick()
		}
		return res
	}
}

func bytesLess(a, b string) bool {
	pa, pb := net.ParseIP(a).To4(), net.ParseIP(b).To4()
	if pa == nil || pb == nil {
		return a < b
	}
	for i := 0; i < 4; i++ {
		if pa[i] != pb[i] {
			return pa[i] < pb[i]
		}
	}
	return false
}

func hostsIn(n *net.IPNet) []string {
	var out []string
	ip := n.IP.Mask(n.Mask).To4()
	if ip == nil {
		return out
	}
	start := int(ip[0])<<24 | int(ip[1])<<16 | int(ip[2])<<8 | int(ip[3])
	ones, bits := n.Mask.Size()
	if bits != 32 {
		return out
	}
	if ones == 32 {
		return []string{fmt.Sprintf("%d.%d.%d.%d", ip[0], ip[1], ip[2], ip[3])}
	}
	count := 1 << (32 - ones)
	if count == 2 {
		// /31: both addresses are usable point-to-point
		return []string{
			fmt.Sprintf("%d.%d.%d.%d", ip[0], ip[1], ip[2], ip[3]),
			fmt.Sprintf("%d.%d.%d.%d", ip[0], ip[1], ip[2], byte(ip[3]+1)),
		}
	}
	for i := 1; i < count-1; i++ {
		v := start + i
		out = append(out, fmt.Sprintf("%d.%d.%d.%d", byte(v>>24), byte(v>>16), byte(v>>8), byte(v)))
	}
	return out
}

// PortURL is the address to open for a discovered port, or "" when it is
// not a web port.
func PortURL(addr string, port int, service, title string) string {
	scheme := ""
	switch {
	case service == "https" || service == "https-alt" || service == "proxmox" || port == 443 || port == 8443:
		scheme = "https"
	case title != "" || service == "http" || service == "http-alt" || webPorts[port]:
		scheme = "http"
	}
	if scheme == "" {
		return ""
	}
	return scheme + "://" + net.JoinHostPort(addr, strconv.Itoa(port)) + "/"
}

var titleRE = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)

func fetchTitle(ctx context.Context, ip string, port int) (string, string) {
	for _, scheme := range []string{"http", "https"} {
		cctx, cancel := context.WithTimeout(ctx, 900*time.Millisecond)
		client := &http.Client{
			Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}},
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
		}
		req, _ := http.NewRequestWithContext(cctx, http.MethodGet, scheme+"://"+net.JoinHostPort(ip, strconv.Itoa(port))+"/", nil)
		req.Header.Set("User-Agent", "home-services-discovery/1")
		resp, err := client.Do(req)
		if err == nil {
			body, _ := io.ReadAll(io.LimitReader(resp.Body, 32<<10))
			resp.Body.Close()
			cancel()
			title := ""
			if m := titleRE.FindSubmatch(body); len(m) > 1 {
				title = strings.TrimSpace(strings.Join(strings.Fields(string(m[1])), " "))
				if len(title) > 120 {
					title = title[:120]
				}
			}
			if resp.StatusCode < 500 {
				return title, scheme
			}
		}
		cancel()
	}
	return "", ""
}

func grabBanner(ip string, port int, timeout time.Duration) string {
	c, err := net.DialTimeout("tcp", net.JoinHostPort(ip, strconv.Itoa(port)), timeout)
	if err != nil {
		return ""
	}
	defer c.Close()
	c.SetReadDeadline(time.Now().Add(timeout))
	buf := make([]byte, 128)
	n, _ := c.Read(buf)
	if n == 0 {
		return ""
	}
	out := strings.Map(func(r rune) rune {
		if r >= 32 && r < 127 {
			return r
		}
		return -1
	}, string(buf[:n]))
	if len(out) > 120 {
		out = out[:120]
	}
	return out
}
