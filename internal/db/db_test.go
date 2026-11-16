package db_test

import (
	"testing"

	"github.com/dawidof/home-services/internal/db"
)

func TestPending(t *testing.T) {
	path := t.TempDir() + "/t.db"
	current, latest, err := db.Pending(path)
	if err != nil || current != 0 || latest < 1 {
		t.Fatalf("fresh: %v current=%d latest=%d", err, current, latest)
	}
	d, err := db.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	d.Close()
	current, latest, err = db.Pending(path)
	if err != nil || current != latest {
		t.Fatalf("after migrate: %v current=%d latest=%d", err, current, latest)
	}
}
