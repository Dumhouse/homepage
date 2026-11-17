package main

import (
	"bytes"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Dumhouse/homepage/internal/db"
)

func gunzip(t *testing.T, path, dst string) {
	t.Helper()
	in, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer in.Close()
	gz, err := gzip.NewReader(in)
	if err != nil {
		t.Fatal(err)
	}
	defer gz.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(out, gz); err != nil {
		t.Fatal(err)
	}
	out.Close()
}

func TestBackupIncludesSecretsWithoutMigrating(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "homeservices.db")
	d, err := db.OpenNoMigrate(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := d.Exec(`CREATE TABLE marker (x TEXT)`); err != nil {
		t.Fatal(err)
	}
	d.Close()
	secrets := []byte("encrypted-secret-bytes")
	if err := os.WriteFile(filepath.Join(dir, "secrets.enc"), secrets, 0o600); err != nil {
		t.Fatal(err)
	}

	backupCmd([]string{"-data", dir, "-keep", "2"})

	dbs, _ := filepath.Glob(filepath.Join(dir, "backups", "*.db.gz"))
	secs, _ := filepath.Glob(filepath.Join(dir, "backups", "*.secrets.enc.gz"))
	if len(dbs) != 1 || len(secs) != 1 {
		t.Fatalf("backups: db=%v secrets=%v", dbs, secs)
	}
	for _, p := range append(dbs, secs...) {
		info, _ := os.Stat(p)
		if info.Mode().Perm() != 0o600 {
			t.Errorf("%s mode = %o, want 600", filepath.Base(p), info.Mode().Perm())
		}
	}

	restored := filepath.Join(t.TempDir(), "restored.db")
	gunzip(t, dbs[0], restored)
	rd, err := db.OpenNoMigrate(restored)
	if err != nil {
		t.Fatal(err)
	}
	defer rd.Close()
	var n int
	if err := rd.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='marker'`).Scan(&n); err != nil || n != 1 {
		t.Errorf("marker table missing: %v %d", err, n)
	}
	if err := rd.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='goose_db_version'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Error("backup migrated the database; it must only copy")
	}

	restoredSecret := filepath.Join(t.TempDir(), "secrets.enc")
	gunzip(t, secs[0], restoredSecret)
	got, err := os.ReadFile(restoredSecret)
	if err != nil || string(got) != string(secrets) {
		t.Errorf("secrets round-trip: %v %q", err, got)
	}
}

func TestUsageListsCommands(t *testing.T) {
	var b bytes.Buffer
	usage(&b)
	for _, want := range []string{"homeservices version", "homeservices migrate", "homeservices token create",
		"homeservices backup", "homeservices scan", "homeservices hash-password", "HS_MASTER_KEY"} {
		if !strings.Contains(b.String(), want) {
			t.Errorf("usage is missing %q", want)
		}
	}
}
