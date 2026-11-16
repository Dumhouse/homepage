package auth

import (
	"net/http"
	"testing"
)

func TestClientKeyHandlesBareAndBracketedAddresses(t *testing.T) {
	for _, tc := range []struct{ remote, want string }{
		{"203.0.113.4:5555", "203.0.113.4"},
		{"203.0.113.4", "203.0.113.4"},
		{"[2001:db8::1]:443", "2001:db8::1"},
		{"[2001:db8::1]", "2001:db8::1"},
		{"", "unknown"},
	} {
		r := &http.Request{RemoteAddr: tc.remote}
		if got := clientKey(r); got != tc.want {
			t.Errorf("clientKey(%q) = %q, want %q", tc.remote, got, tc.want)
		}
	}
}
