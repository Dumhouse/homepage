package settings_test

import (
	"context"
	"errors"
	"testing"

	"github.com/dawidof/home-services/internal/db"
	"github.com/dawidof/home-services/internal/settings"
)

func TestSettings(t *testing.T) {
	d, err := db.Open(t.TempDir() + "/t.db")
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	ctx, s := context.Background(), settings.New(d)

	if v, err := s.Get(ctx, "sidebar", "open"); err != nil || v != "open" {
		t.Fatalf("default: %v %q", err, v)
	}
	if err := s.Set(ctx, "ui", "sidebar", "collapsed"); err != nil {
		t.Fatal(err)
	}
	if v, _ := s.Get(ctx, "sidebar", "open"); v != "collapsed" {
		t.Errorf("after set: %q", v)
	}
	if err := s.Set(ctx, "ui", "sidebar", "open"); err != nil {
		t.Fatal(err)
	}
	all, err := s.All(ctx)
	if err != nil || all["sidebar"] != "open" {
		t.Fatalf("all: %v %+v", err, all)
	}
	if err := s.Set(ctx, "ui", " ", "x"); !errors.Is(err, settings.ErrInvalid) {
		t.Errorf("blank key: %v", err)
	}

	var n int
	if err := d.QueryRow(`SELECT COUNT(*) FROM audit_log WHERE entity='setting'`).Scan(&n); err != nil || n != 2 {
		t.Errorf("audit rows: %v %d", err, n)
	}
}
