package web

import "testing"

func TestSafeBack(t *testing.T) {
	for in, want := range map[string]string{
		"/hosts/1":             "/hosts/1",
		"/projects/a?env=prod": "/projects/a?env=prod",
		"":                     "/def",
		"https://evil.example": "/def",
		"//evil.example":       "/def",
		`/\evil.example`:       "/def",
		"/\tevil":              "/def",
		"javascript:alert(1)":  "/def",
		"hosts/1":              "/def",
	} {
		if got := safeBack(in, "/def"); got != want {
			t.Errorf("safeBack(%q) = %q, want %q", in, got, want)
		}
	}
}
