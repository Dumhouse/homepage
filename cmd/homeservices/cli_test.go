package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Dumhouse/homepage/internal/inventory"
)

// capture runs fn with stdout and stderr redirected and returns both.
func capture(t *testing.T, fn func()) (string, string) {
	t.Helper()
	oldOut, oldErr := os.Stdout, os.Stderr
	ro, wo, _ := os.Pipe()
	re, we, _ := os.Pipe()
	os.Stdout, os.Stderr = wo, we
	outC, errC := make(chan string), make(chan string)
	go func() { b, _ := io.ReadAll(ro); outC <- string(b) }()
	go func() { b, _ := io.ReadAll(re); errC <- string(b) }()
	defer func() { os.Stdout, os.Stderr = oldOut, oldErr }()
	fn()
	wo.Close()
	we.Close()
	return <-outC, <-errC
}

func TestCLITokens(t *testing.T) {
	dir := t.TempDir()
	out, errOut := capture(t, func() { tokenCmd([]string{"create", "-data", dir, "--name", "agent", "--scope", "propose"}) })
	if !strings.HasPrefix(strings.TrimSpace(out), "hs_") || !strings.Contains(errOut, `Created "agent" (propose scope`) {
		t.Fatalf("create: %q %q", out, errOut)
	}
	out, _ = capture(t, func() { tokenCmd([]string{"list", "-data", dir}) })
	if !strings.Contains(out, "agent") || !strings.Contains(out, "propose") || !strings.Contains(out, "active") {
		t.Fatalf("list: %q", out)
	}
	_, errOut = capture(t, func() { tokenCmd([]string{"revoke", "-data", dir, "agent"}) })
	if !strings.Contains(errOut, `Revoked "agent"`) {
		t.Fatalf("revoke: %q", errOut)
	}
	out, _ = capture(t, func() { tokenCmd([]string{"list", "-data", dir}) })
	if !strings.Contains(out, "revoked") {
		t.Fatalf("list after revoke: %q", out)
	}
}

func TestCLIDemoMigrateImportBackup(t *testing.T) {
	dir := t.TempDir()
	out, _ := capture(t, func() { demoCmd([]string{"-data", dir}) })
	if !strings.Contains(out, "demo lab seeded") {
		t.Fatalf("demo: %q", out)
	}
	out, _ = capture(t, func() { migrateCmd([]string{"-data", dir, "-check"}) })
	if !strings.HasPrefix(out, "schema ") {
		t.Fatalf("migrate -check: %q", out)
	}
	out, _ = capture(t, func() { migrateCmd([]string{"-data", dir}) })
	if !strings.Contains(out, "migrations applied") {
		t.Fatalf("migrate: %q", out)
	}

	// Export the demo, import it into an empty install: dry run, then apply.
	d := openDB(dir)
	exp, err := inventory.New(d).Export(context.Background())
	d.Close()
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(t.TempDir(), "inv.json")
	b, _ := json.Marshal(exp)
	if err := os.WriteFile(file, b, 0o600); err != nil {
		t.Fatal(err)
	}
	empty := t.TempDir()
	out, errOut := capture(t, func() { importCmd([]string{"-data", empty, file}) })
	if !strings.Contains(errOut, "dry run") || !strings.Contains(out, "{") {
		t.Fatalf("import dry run: %q %q", out, errOut)
	}
	d = openDB(empty)
	hosts, _ := inventory.New(d).ListHosts(context.Background())
	d.Close()
	if len(hosts) != 0 {
		t.Fatal("a dry run must not write")
	}
	capture(t, func() { importCmd([]string{"-data", empty, "-apply", file}) })
	d = openDB(empty)
	hosts, _ = inventory.New(d).ListHosts(context.Background())
	d.Close()
	if len(hosts) != len(exp.Hosts) || len(hosts) == 0 {
		t.Fatalf("imported %d hosts, want %d", len(hosts), len(exp.Hosts))
	}

	out, _ = capture(t, func() { backupCmd([]string{"-data", dir, "-keep", "3"}) })
	if !strings.Contains(out, "homeservices-") || !strings.HasSuffix(strings.TrimSpace(strings.Split(out, "\n")[0]), ".db.gz") {
		t.Fatalf("backup: %q", out)
	}
}

func TestUsageAndEnv(t *testing.T) {
	var buf bytes.Buffer
	usage(&buf)
	for _, want := range []string{"homeservices import", "token create", "-copy-to", "HS_TRUSTED_PROXIES", "HS_UPDATE_CHECK"} {
		if !strings.Contains(buf.String(), want) {
			t.Errorf("usage lacks %q", want)
		}
	}
	t.Setenv("HS_TEST_X", "")
	if env("HS_TEST_X", "def") != "def" {
		t.Fatal("empty env uses the default")
	}
	t.Setenv("HS_TEST_X", "v")
	if env("HS_TEST_X", "def") != "v" {
		t.Fatal("env value")
	}
}
