package discovery

import (
	"bytes"
	"context"
	"crypto/tls"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"
)

//go:embed favicons.json
var faviconsJSON []byte

type faviconRule struct {
	Service string `json:"service"`
	Hash    int32  `json:"hash"`
}

var (
	faviconOnce sync.Once
	faviconSums map[int32]string
)

func loadFavicons() {
	faviconOnce.Do(func() {
		var rules []faviconRule
		_ = json.Unmarshal(faviconsJSON, &rules)
		faviconSums = make(map[int32]string, len(rules))
		for _, r := range rules {
			faviconSums[r.Hash] = r.Service
		}
	})
}

// faviconService maps a Shodan-style favicon hash to a service name.
func faviconService(sum int32) string {
	loadFavicons()
	return faviconSums[sum]
}

// faviconFingerprint fetches /favicon.ico and matches its hash. It is the last
// hint because it costs one extra request per unidentified web port.
func faviconFingerprint(ctx context.Context, scheme, ip string, port int) string {
	if scheme == "" {
		return ""
	}
	data := fetchFavicon(ctx, scheme, ip, port)
	if len(data) == 0 {
		return ""
	}
	return faviconService(faviconHash(data))
}

func fetchFavicon(ctx context.Context, scheme, ip string, port int) []byte {
	cctx, cancel := context.WithTimeout(ctx, 800*time.Millisecond)
	defer cancel()
	req, err := http.NewRequestWithContext(cctx, http.MethodGet,
		scheme+"://"+net.JoinHostPort(ip, strconv.Itoa(port))+"/favicon.ico", nil)
	if err != nil {
		return nil
	}
	req.Header.Set("User-Agent", "home-services-discovery/1")
	client := &http.Client{
		// #nosec G402 -- read-only fingerprinting must also see self-signed LAN endpoints.
		Transport:     &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}},
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if err != nil || len(data) == 0 {
		return nil
	}
	return data
}

// faviconHash follows Shodan's convention: base64 (encodebytes-style 76-column
// lines plus a trailing newline) hashed with MurmurHash3 x86_32, signed.
func faviconHash(data []byte) int32 {
	enc := base64.StdEncoding.EncodeToString(data)
	if enc == "" {
		return 0
	}
	var b bytes.Buffer
	for len(enc) > 76 {
		b.WriteString(enc[:76])
		b.WriteByte('\n')
		enc = enc[76:]
	}
	b.WriteString(enc)
	b.WriteByte('\n')
	return int32(murmur3(b.Bytes(), 0))
}

// murmur3 implements MurmurHash3 x86_32.
func murmur3(data []byte, seed uint32) uint32 {
	const (
		c1 = 0xcc9e2d51
		c2 = 0x1b873593
	)
	h := seed
	n := len(data)
	for len(data) >= 4 {
		k := uint32(data[0]) | uint32(data[1])<<8 | uint32(data[2])<<16 | uint32(data[3])<<24
		k *= c1
		k = k<<15 | k>>17
		k *= c2
		h ^= k
		h = h<<13 | h>>19
		h = h*5 + 0xe6546b64
		data = data[4:]
	}
	var k uint32
	switch len(data) {
	case 3:
		k ^= uint32(data[2]) << 16
		fallthrough
	case 2:
		k ^= uint32(data[1]) << 8
		fallthrough
	case 1:
		k ^= uint32(data[0])
		k *= c1
		k = k<<15 | k>>17
		k *= c2
		h ^= k
	}
	h ^= uint32(n)
	h ^= h >> 16
	h *= 0x85ebca6b
	h ^= h >> 13
	h *= 0xc2b2ae35
	h ^= h >> 16
	return h
}
