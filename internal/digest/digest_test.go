package digest_test

import (
	"strings"
	"testing"
	"time"

	"github.com/dawidof/home-services/internal/digest"
)

func TestBuild(t *testing.T) {
	at := time.Date(2026, 11, 16, 9, 0, 0, 0, time.UTC)
	in := digest.Input{
		At:     at,
		Counts: digest.Counts{Hosts: 3, Services: 12, Projects: 2, Locations: 1},
		Checks: digest.CheckCounts{Up: 14, Failing: 1, Down: 1, Unknown: 2, Disabled: 3},
		Problems: []digest.Problem{
			{Title: "acme/web", State: "down", Message: "connection refused"},
			{Title: "ssh deploy on acme", State: "failing"},
		},
		Expiring:    []string{"soon.example — 6 days (2026-11-22)"},
		Maintenance: 1,
	}
	text := digest.Build(in)
	for _, want := range []string{
		"weekly digest (16 Nov 2026)",
		"3 hosts · 12 services · 2 projects · 1 locations",
		"14 up · 1 failing · 1 down · 0 recovering · 2 unknown · 3 paused",
		"Problems (2):",
		"acme/web — down: connection refused",
		"ssh deploy on acme — failing",
		"Expiring soon:",
		"soon.example — 6 days (2026-11-22)",
		"Maintenance: 1 window(s) active now.",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q in:\n%s", want, text)
		}
	}
	empty := digest.Build(digest.Input{At: at})
	if !strings.Contains(empty, "No problems right now.") {
		t.Errorf("empty digest: %s", empty)
	}
}

func TestShouldSend(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	if digest.ShouldSend(now, 0, 7*24*time.Hour) {
		t.Error("no baseline should not send")
	}
	if digest.ShouldSend(now, now.Add(-6*24*time.Hour).Unix(), 7*24*time.Hour) {
		t.Error("six days is too early")
	}
	if !digest.ShouldSend(now, now.Add(-8*24*time.Hour).Unix(), 7*24*time.Hour) {
		t.Error("eight days should send")
	}
}
