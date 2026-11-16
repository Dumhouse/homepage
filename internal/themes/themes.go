// Package themes validates shareable theme JSON files (PLAN §22): schema, CSS
// sanitiser and a minimum contrast check for the main text/background pair.
package themes

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

type Theme struct {
	ID     string                       `json:"id"`
	Name   string                       `json:"name"`
	Modes  map[string]map[string]string `json:"modes"`
	CSS    string                       `json:"css"`
	Radius *int                         `json:"radius"`
}

var (
	cssImport = regexp.MustCompile(`(?i)@import[^;]*;?`)
	cssURL    = regexp.MustCompile(`(?i)url\(\s*['"]?([^)'"]*)`)
)

// Validate returns an error when the theme would be rejected by the loader.
func Validate(raw []byte) error {
	var t Theme
	if err := json.Unmarshal(raw, &t); err != nil {
		return fmt.Errorf("json: %w", err)
	}
	if strings.TrimSpace(t.ID) == "" || strings.TrimSpace(t.Name) == "" {
		return fmt.Errorf("id and name are required")
	}
	for _, mode := range []string{"dark", "light"} {
		for k := range t.Modes[mode] {
			if !strings.HasPrefix(k, "--") {
				return fmt.Errorf("%s: %q is not a CSS variable", mode, k)
			}
		}
	}
	if cssImport.MatchString(t.CSS) || hasExternalURL(t.CSS) {
		return fmt.Errorf("css may not use @import or external url()")
	}
	if t.Radius != nil && (*t.Radius < 0 || *t.Radius > 32) {
		return fmt.Errorf("radius out of range")
	}
	// Contrast: if both tokens are hex, require >= 4.5:1.
	for mode, vars := range t.Modes {
		bg, text := vars["--bg"], vars["--text"]
		if bg == "" || text == "" {
			continue
		}
		cr, ok := contrast(bg, text)
		if ok && cr < 4.5 {
			return fmt.Errorf("%s: text/background contrast %.1f is below 4.5", mode, cr)
		}
	}
	return nil
}

func hasExternalURL(css string) bool {
	for _, m := range cssURL.FindAllStringSubmatch(css, -1) {
		if len(m) < 2 {
			continue
		}
		v := strings.TrimSpace(strings.ToLower(m[1]))
		if strings.HasPrefix(v, "data:") || strings.HasPrefix(v, "assets/") || strings.HasPrefix(v, "./") {
			continue
		}
		return true
	}
	return false
}

func contrast(a, b string) (float64, bool) {
	la, oka := luminance(a)
	lb, okb := luminance(b)
	if !oka || !okb {
		return 0, false
	}
	hi, lo := math.Max(la, lb), math.Min(la, lb)
	return (hi + 0.05) / (lo + 0.05), true
}

func luminance(hex string) (float64, bool) {
	hex = strings.TrimPrefix(strings.TrimSpace(hex), "#")
	if len(hex) == 3 {
		hex = string([]byte{hex[0], hex[0], hex[1], hex[1], hex[2], hex[2]})
	}
	if len(hex) != 6 {
		return 0, false
	}
	n, err := strconv.ParseUint(hex, 16, 32)
	if err != nil {
		return 0, false
	}
	r, g, b := float64(n>>16&0xff)/255, float64(n>>8&0xff)/255, float64(n&0xff)/255
	lin := func(c float64) float64 {
		if c <= 0.03928 {
			return c / 12.92
		}
		return math.Pow((c+0.055)/1.055, 2.4)
	}
	return 0.2126*lin(r) + 0.7152*lin(g) + 0.0722*lin(b), true
}

// ValidateDir validates every *.json file in dir (used by the theme CI test).
func ValidateDir(dir string) error {
	files, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return err
	}
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			return err
		}
		if err := Validate(raw); err != nil {
			return fmt.Errorf("%s: %w", filepath.Base(f), err)
		}
	}
	return nil
}
