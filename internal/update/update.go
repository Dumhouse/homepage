// Package update checks GitHub for a newer release. It is off by default —
// Home Services makes no outbound calls you did not ask for — and only reads
// the public "latest release" endpoint: nothing about your install is sent.
package update

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/Dumhouse/homepage/internal/settings"
)

// Settings keys.
const (
	KeyEnabled   = "update_check"      // "on" | "off" (default off)
	KeyLatest    = "update_latest"     // e.g. v0.3.0
	KeyURL       = "update_url"        // release page
	KeyCheckedAt = "update_checked_at" // RFC 3339
	KeyError     = "update_error"
)

// DefaultURL is the GitHub API endpoint for the newest release.
const DefaultURL = "https://api.github.com/repos/Dumhouse/homepage/releases/latest"

// Release is the part of GitHub's answer we use.
type Release struct {
	Tag string `json:"tag_name"`
	URL string `json:"html_url"`
}

// Fetch asks endpoint for the latest release.
func Fetch(ctx context.Context, c *http.Client, endpoint string) (Release, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return Release{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "homeservices-update-check")
	res, err := c.Do(req)
	if err != nil {
		return Release{}, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return Release{}, fmt.Errorf("release check: HTTP %d", res.StatusCode)
	}
	var r Release
	if err := json.NewDecoder(http.MaxBytesReader(nil, res.Body, 1<<20)).Decode(&r); err != nil {
		return Release{}, err
	}
	if r.Tag == "" {
		return Release{}, fmt.Errorf("release check: no tag in the answer")
	}
	return r, nil
}

// Enabled reports whether the user switched the check on (setting or
// HS_UPDATE_CHECK=on). The environment wins so an admin can force it off.
func Enabled(ctx context.Context, set *settings.Store) bool {
	switch strings.ToLower(os.Getenv("HS_UPDATE_CHECK")) {
	case "on", "1", "true":
		return true
	case "off", "0", "false":
		return false
	}
	v, _ := set.Get(ctx, KeyEnabled, "off")
	return v == "on"
}

// Check fetches the latest release and stores the result.
func Check(ctx context.Context, set *settings.Store, c *http.Client, endpoint string) (Release, error) {
	r, err := Fetch(ctx, c, endpoint)
	now := time.Now().UTC().Format(time.RFC3339)
	_ = set.Set(ctx, "system:update", KeyCheckedAt, now)
	if err != nil {
		_ = set.Set(ctx, "system:update", KeyError, err.Error())
		return r, err
	}
	_ = set.Set(ctx, "system:update", KeyError, "")
	_ = set.Set(ctx, "system:update", KeyLatest, r.Tag)
	_ = set.Set(ctx, "system:update", KeyURL, r.URL)
	return r, nil
}

// Loop checks once a day while the check is enabled. It returns a stop function.
func Loop(ctx context.Context, set *settings.Store) func() {
	ctx, cancel := context.WithCancel(ctx)
	c := &http.Client{Timeout: 15 * time.Second}
	go func() {
		t := time.NewTicker(time.Hour)
		defer t.Stop()
		for {
			if Enabled(ctx, set) && due(ctx, set, time.Now()) {
				_, _ = Check(ctx, set, c, DefaultURL)
			}
			select {
			case <-ctx.Done():
				return
			case <-t.C:
			}
		}
	}()
	return cancel
}

func due(ctx context.Context, set *settings.Store, now time.Time) bool {
	raw, _ := set.Get(ctx, KeyCheckedAt, "")
	last, err := time.Parse(time.RFC3339, raw)
	return err != nil || now.Sub(last) >= 24*time.Hour
}

// Available returns the newer release tag and its URL, or "" when current is
// up to date, unknown, or the check is off.
func Available(ctx context.Context, set *settings.Store, current string) (string, string) {
	if !Enabled(ctx, set) {
		return "", ""
	}
	latest, _ := set.Get(ctx, KeyLatest, "")
	if latest == "" || !Newer(latest, current) {
		return "", ""
	}
	u, _ := set.Get(ctx, KeyURL, "")
	return latest, u
}

// Newer reports whether latest is a higher version than current. Development
// builds ("dev", a bare commit hash) never nag. A build ahead of a tag
// (v0.2.0-3-gabc) counts as that tag.
func Newer(latest, current string) bool {
	l, ok1 := parse(latest)
	c, ok2 := parse(current)
	if !ok1 || !ok2 {
		return false
	}
	for i := 0; i < 3; i++ {
		if l[i] != c[i] {
			return l[i] > c[i]
		}
	}
	return false
}

func parse(v string) ([3]int, bool) {
	var out [3]int
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		v = v[:i]
	}
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return out, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return out, false
		}
		out[i] = n
	}
	return out, true
}
