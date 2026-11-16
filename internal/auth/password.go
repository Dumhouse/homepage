package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"html"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/argon2"
)

const (
	cookieName = "hs_session"
	sessionTTL = 30 * 24 * time.Hour
)

// HashPassword returns an argon2id hash: "argon2id$<salt>$<hash>" (base64, no padding).
func HashPassword(pw string) (string, error) {
	salt := make([]byte, 16)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	h := argon2.IDKey([]byte(pw), salt, 3, 64*1024, 2, 32)
	e := base64.RawStdEncoding
	return "argon2id$" + e.EncodeToString(salt) + "$" + e.EncodeToString(h), nil
}

func checkPassword(encoded, pw string) bool {
	p := strings.Split(encoded, "$")
	if len(p) != 3 || p[0] != "argon2id" {
		return false
	}
	e := base64.RawStdEncoding
	salt, err1 := e.DecodeString(p[1])
	want, err2 := e.DecodeString(p[2])
	if err1 != nil || err2 != nil {
		return false
	}
	got := argon2.IDKey([]byte(pw), salt, 3, 64*1024, 2, 32)
	return subtle.ConstantTimeCompare(got, want) == 1
}

type password struct {
	hash    string
	key     []byte
	secure  bool
	limiter *loginLimiter
}

// loginLimiter is a small in-memory brute-force guard: five failed attempts
// per IP in five minutes. Successful sign-in resets it.
type loginLimiter struct {
	mu      sync.Mutex
	entries map[string]loginAttempt
}

type loginAttempt struct {
	n     int
	reset time.Time
}

const (
	loginMaxAttempts = 5
	loginWindow      = 5 * time.Minute
)

func newLoginLimiter() *loginLimiter {
	return &loginLimiter{entries: map[string]loginAttempt{}}
}

func (l *loginLimiter) blocked(key string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	a, ok := l.entries[key]
	if !ok || now.After(a.reset) {
		delete(l.entries, key)
		return false
	}
	return a.n >= loginMaxAttempts
}

func (l *loginLimiter) fail(key string, now time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	a := l.entries[key]
	if now.After(a.reset) {
		a = loginAttempt{reset: now.Add(loginWindow)}
	}
	a.n++
	l.entries[key] = a
}

func (l *loginLimiter) success(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.entries, key)
}

func clientKey(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil || host == "" {
		return "unknown"
	}
	return host
}

func newPassword(c Config) (Provider, error) {
	if !strings.HasPrefix(c.PasswordHash, "argon2id$") {
		return nil, errors.New("HS_AUTH=password needs HS_PASSWORD_HASH (create one with: homeservices hash-password)")
	}
	key := c.SessionKey
	if len(key) < 16 { // no stable key configured: sessions end on restart
		key = make([]byte, 32)
		if _, err := rand.Read(key); err != nil {
			return nil, err
		}
	}
	return &password{hash: c.PasswordHash, key: key, secure: c.Secure, limiter: newLoginLimiter()}, nil
}

func (p *password) Name() string { return "password" }

// ValidSession reports whether the request carries a valid session cookie.
func (p *password) ValidSession(r *http.Request) bool {
	c, err := r.Cookie(cookieName)
	return err == nil && p.valid(c.Value)
}

func (p *password) sign(exp int64) string {
	m := hmac.New(sha256.New, p.key)
	fmt.Fprint(m, exp)
	return strconv.FormatInt(exp, 10) + "." + base64.RawURLEncoding.EncodeToString(m.Sum(nil))
}

func (p *password) valid(v string) bool {
	i := strings.IndexByte(v, '.')
	if i < 0 {
		return false
	}
	exp, err := strconv.ParseInt(v[:i], 10, 64)
	if err != nil || time.Now().Unix() > exp {
		return false
	}
	return hmac.Equal([]byte(v), []byte(p.sign(exp)))
}

func (p *password) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/healthz" || strings.HasPrefix(r.URL.Path, "/mcp") || strings.HasPrefix(r.URL.Path, "/api/v1"):
			next.ServeHTTP(w, r) // health + token-protected MCP/API stay reachable
		case r.URL.Path == "/login":
			p.login(w, r)
		case r.URL.Path == "/logout":
			http.SetCookie(w, &http.Cookie{Name: cookieName, Path: "/", MaxAge: -1})
			http.Redirect(w, r, "/login", http.StatusSeeOther)
		default:
			if c, err := r.Cookie(cookieName); err == nil && p.valid(c.Value) {
				next.ServeHTTP(w, r)
				return
			}
			http.Redirect(w, r, "/login", http.StatusSeeOther)
		}
	})
}

func (p *password) login(w http.ResponseWriter, r *http.Request) {
	key := clientKey(r)
	msg := ""
	if p.limiter.blocked(key, time.Now()) {
		w.Header().Set("Retry-After", "300")
		w.WriteHeader(http.StatusTooManyRequests)
		msg = "Too many attempts — wait five minutes and try again."
	} else if r.Method == http.MethodPost {
		time.Sleep(300 * time.Millisecond) // blunt brute-force slowdown
		if checkPassword(p.hash, r.FormValue("password")) {
			p.limiter.success(key)
			exp := time.Now().Add(sessionTTL).Unix()
			http.SetCookie(w, &http.Cookie{
				Name: cookieName, Value: p.sign(exp), Path: "/", HttpOnly: true,
				SameSite: http.SameSiteLaxMode, Secure: p.secure, Expires: time.Unix(exp, 0),
			})
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}
		p.limiter.fail(key, time.Now())
		msg = "Wrong password"
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, `<!doctype html><meta charset=utf-8><meta name=viewport content="width=device-width,initial-scale=1"><title>Sign in</title>
<link rel=stylesheet href="/assets/app.css"><body style="display:grid;place-items:center;min-height:100vh">
<form method=post class="card" style="padding:24px;width:min(340px,92vw);display:flex;flex-direction:column;gap:12px">
<h2>Home Services</h2><input class=input type=password name=password placeholder=Password autofocus required>
<div class=bad-t>%s</div><button class="btn primary">Sign in</button></form>`, html.EscapeString(msg))
}
