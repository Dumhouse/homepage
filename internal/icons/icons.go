// Package icons fetches and caches site favicons into data/icons.
//
// SSRF guard: private/loopback/link-local destinations are refused unless the
// favicon_private setting is on. Responses are size-capped and must look like
// images. The app never proxies anything else.
package icons

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/Dumhouse/homepage/internal/inventory"
	"github.com/Dumhouse/homepage/internal/settings"
)

var (
	ErrDisabled = errors.New("favicon fetch disabled")
	ErrPrivate  = errors.New("refusing to fetch a private address")
	ErrTooLarge = errors.New("favicon too large")
	ErrNotImage = errors.New("response is not an image")
)

type Fetcher struct {
	Dir      string
	Settings *settings.Store
	MaxBytes int64
}

func New(dir string, set *settings.Store) *Fetcher {
	return &Fetcher{Dir: dir, Settings: set, MaxBytes: 256 << 10}
}

// ForService fetches the icon for a service domain (if enabled) and stores the
// path on the service. It returns the icon path or an explanatory error.
func (f *Fetcher) ForService(ctx context.Context, inv *inventory.Store, svc *inventory.Service) (string, error) {
	if f == nil || f.Dir == "" {
		return "", ErrDisabled
	}
	src := siteFor(ctx, inv, svc)
	if src == "" {
		return "", ErrDisabled
	}
	if f.Settings != nil {
		if v, _ := f.Settings.Get(ctx, "favicon_fetch", "on"); v == "off" {
			return "", ErrDisabled
		}
	}
	allowPrivate := false
	if f.Settings != nil {
		if v, _ := f.Settings.Get(ctx, "favicon_private", "off"); v == "on" {
			allowPrivate = true
		}
	}
	icon, err := f.Fetch(ctx, src, allowPrivate)
	if err != nil {
		return "", err
	}
	if err := inv.SetServiceIcon(ctx, "icon-fetch", svc.ID, icon); err != nil {
		return "", err
	}
	return icon, nil
}

// siteFor is the page to read the icon from: the service domain, or else the
// host's primary address with the service port (LAN services need "allow private").
func siteFor(ctx context.Context, inv *inventory.Store, svc *inventory.Service) string {
	scheme := "https"
	if svc.Protocol == "http" {
		scheme = "http"
	}
	if svc.Domain != "" {
		return scheme + "://" + svc.Domain
	}
	if svc.Port == 0 || (svc.Protocol != "http" && svc.Protocol != "https") {
		return ""
	}
	h, err := inv.GetHost(ctx, svc.HostID)
	if err != nil {
		return ""
	}
	addr := ""
	for _, a := range h.Addresses {
		if a.IsPrimary || addr == "" {
			addr = a.Address
		}
	}
	if addr == "" {
		return ""
	}
	return scheme + "://" + net.JoinHostPort(addr, strconv.Itoa(svc.Port))
}

// Queue runs ForService in the background, ignoring failures.
func (f *Fetcher) Queue(inv *inventory.Store, svc *inventory.Service) {
	if f == nil || svc == nil {
		return
	}
	copy := *svc
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, _ = f.ForService(ctx, inv, &copy)
	}()
}

var (
	linkTag = regexp.MustCompile(`(?i)<link\b[^>]*>`)
	relAttr = regexp.MustCompile(`(?i)\brel\s*=\s*["']([^"']*)["']`)
	hrefAtt = regexp.MustCompile(`(?i)\bhref\s*=\s*["']([^"']+)["']`)
)

// Fetch returns a cached /icons/... path for the site, downloading it if needed.
func (f *Fetcher) Fetch(ctx context.Context, siteURL string, allowPrivate bool) (string, error) {
	base, err := url.Parse(siteURL)
	if err != nil || (base.Scheme != "http" && base.Scheme != "https") || base.Host == "" {
		return "", fmt.Errorf("invalid site URL %q", siteURL)
	}
	key := hashKey(base.Scheme + "://" + base.Host)
	if cached := f.cached(key); cached != "" {
		return cached, nil
	}
	if err := f.allowed(ctx, base, allowPrivate); err != nil {
		return "", err
	}

	var candidates []string
	if href := f.pageIcon(ctx, base, allowPrivate); href != "" {
		candidates = append(candidates, href)
	}
	candidates = append(candidates, base.Scheme+"://"+base.Host+"/favicon.ico")

	var lastErr error
	for _, candidate := range candidates {
		u, err := url.Parse(candidate)
		if err != nil || u.Host == "" {
			continue
		}
		if err := f.allowed(ctx, u, allowPrivate); err != nil {
			lastErr = err
			continue
		}
		if p, err := f.download(ctx, u, key, allowPrivate); err == nil {
			return p, nil
		} else if errors.Is(err, ErrTooLarge) {
			return "", err
		} else {
			lastErr = err
		}
	}
	if lastErr != nil {
		return "", lastErr
	}
	return "", errors.New("no favicon found")
}

func (f *Fetcher) pageIcon(ctx context.Context, base *url.URL, allowPrivate bool) string {
	body, err := f.get(ctx, base.String(), allowPrivate, 128<<10)
	if err != nil {
		return ""
	}
	page := string(body)
	for _, tag := range linkTag.FindAllString(page, -1) {
		rel := relAttr.FindStringSubmatch(tag)
		href := hrefAtt.FindStringSubmatch(tag)
		if len(rel) < 2 || len(href) < 2 {
			continue
		}
		if !strings.Contains(strings.ToLower(rel[1]), "icon") {
			continue
		}
		u, err := base.Parse(strings.TrimSpace(href[1]))
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") {
			continue
		}
		return u.String()
	}
	return ""
}

func (f *Fetcher) download(ctx context.Context, u *url.URL, key string, allowPrivate bool) (string, error) {
	client := f.client(allowPrivate)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "image/*")
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, f.MaxBytes+1))
	if err != nil {
		return "", err
	}
	if int64(len(data)) > f.MaxBytes {
		return "", ErrTooLarge
	}
	if len(data) == 0 {
		return "", ErrNotImage
	}
	ct := strings.ToLower(strings.Split(resp.Header.Get("Content-Type"), ";")[0])
	if ct != "" && !strings.HasPrefix(ct, "image/") && ct != "application/octet-stream" {
		return "", ErrNotImage
	}
	ext := iconExt(ct, u.Path)
	if ext == "" {
		if !strings.HasPrefix(ct, "image/") {
			return "", ErrNotImage
		}
		ext = ".img"
	}
	name := key + ext
	if err := os.MkdirAll(f.Dir, 0o750); err != nil {
		return "", err
	}
	tmp := filepath.Join(f.Dir, ".tmp-"+name)
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return "", err
	}
	if err := os.Rename(tmp, filepath.Join(f.Dir, name)); err != nil {
		return "", err
	}
	return "/icons/" + name, nil
}

func (f *Fetcher) cached(key string) string {
	matches, _ := filepath.Glob(filepath.Join(f.Dir, key+".*"))
	for _, m := range matches {
		if !strings.Contains(filepath.Base(m), ".tmp-") {
			return "/icons/" + filepath.Base(m)
		}
	}
	return ""
}

func (f *Fetcher) get(ctx context.Context, raw string, allowPrivate bool, limit int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "homeservices-favicon/1")
	resp, err := f.client(allowPrivate).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, limit))
}

func (f *Fetcher) client(allowPrivate bool) *http.Client {
	return &http.Client{
		Timeout: 8 * time.Second,
		// LAN apps mostly use self-signed certificates; an icon is public data, so allow them there only.
		Transport: &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: allowPrivate}},
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 3 {
				return errors.New("too many redirects")
			}
			return f.allowed(req.Context(), req.URL, allowPrivate)
		},
	}
}

func (f *Fetcher) allowed(ctx context.Context, u *url.URL, allowPrivate bool) error {
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("unsupported scheme %q", u.Scheme)
	}
	host := u.Hostname()
	if host == "" {
		return errors.New("missing host")
	}
	if allowPrivate {
		return nil
	}
	if strings.EqualFold(host, "localhost") {
		return ErrPrivate
	}
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return err
	}
	for _, ip := range ips {
		if ip.IP.IsLoopback() || ip.IP.IsPrivate() || ip.IP.IsLinkLocalUnicast() ||
			ip.IP.IsUnspecified() || ip.IP.IsMulticast() {
			return ErrPrivate
		}
	}
	return nil
}

func iconExt(contentType, p string) string {
	switch contentType {
	case "image/png":
		return ".png"
	case "image/jpeg":
		return ".jpg"
	case "image/svg+xml":
		return ".svg"
	case "image/webp":
		return ".webp"
	case "image/gif":
		return ".gif"
	case "image/x-icon", "image/vnd.microsoft.icon":
		return ".ico"
	}
	switch strings.ToLower(path.Ext(p)) {
	case ".ico", ".png", ".svg", ".jpg", ".jpeg", ".webp", ".gif":
		return strings.ToLower(path.Ext(p))
	}
	return ""
}

func hashKey(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:8])
}
