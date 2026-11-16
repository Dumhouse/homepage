// Package rdap looks up a domain's public registration data (registrar,
// expiry) over RDAP — the JSON successor of WHOIS. rdap.org redirects each
// query to the registry that is authoritative for the TLD.
package rdap

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Info is what Lookup could find. Empty fields were not published.
type Info struct {
	Registrar string
	ExpiresAt string // YYYY-MM-DD
	Status    []string
}

// Endpoint is the bootstrap service; tests point it at a fake server.
var Endpoint = "https://rdap.org/domain/"

var client = &http.Client{Timeout: 12 * time.Second}

// Lookup fetches registration data for name (e.g. "example.com").
func Lookup(ctx context.Context, name string) (*Info, error) {
	name = strings.TrimSuffix(strings.ToLower(strings.TrimSpace(name)), ".")
	if name == "" || !strings.Contains(name, ".") {
		return nil, errors.New("not a domain name")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, Endpoint+url.PathEscape(name), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/rdap+json, application/json")
	res, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("RDAP lookup failed: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusNotFound {
		return nil, errors.New("the registry has no record of this domain (or its TLD has no RDAP service)")
	}
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("RDAP lookup failed: HTTP %d", res.StatusCode)
	}
	var doc struct {
		Status []string `json:"status"`
		Events []struct {
			Action string `json:"eventAction"`
			Date   string `json:"eventDate"`
		} `json:"events"`
		Entities []struct {
			Roles []string          `json:"roles"`
			VCard []json.RawMessage `json:"vcardArray"`
		} `json:"entities"`
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 2<<20)).Decode(&doc); err != nil {
		return nil, fmt.Errorf("RDAP answer unreadable: %w", err)
	}
	info := &Info{Status: doc.Status}
	for _, e := range doc.Events {
		if e.Action == "expiration" {
			if t, err := time.Parse(time.RFC3339, e.Date); err == nil {
				info.ExpiresAt = t.UTC().Format("2006-01-02")
			}
		}
	}
	for _, e := range doc.Entities {
		if !contains(e.Roles, "registrar") || len(e.VCard) < 2 {
			continue
		}
		info.Registrar = vcardFN(e.VCard[1])
	}
	return info, nil
}

// vcardFN pulls the "fn" (full name) out of a jCard property list.
func vcardFN(raw json.RawMessage) string {
	var props [][]any
	if json.Unmarshal(raw, &props) != nil {
		return ""
	}
	for _, p := range props {
		if len(p) >= 4 {
			if k, _ := p[0].(string); k == "fn" {
				v, _ := p[3].(string)
				return v
			}
		}
	}
	return ""
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}
