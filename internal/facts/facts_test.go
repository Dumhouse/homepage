package facts_test

import (
	"strings"
	"testing"

	"github.com/dawidof/home-services/internal/facts"
)

func TestParseFacts(t *testing.T) {
	outputs := map[string]string{
		"PRETTY_NAME":     `PRETTY_NAME="Debian GNU/Linux 13 (trixie)"`,
		"uname -r":        "6.12.0-1-amd64",
		"hostname":        "acme",
		"uptime -p":       "up 12 days, 3 hours",
		"df -P /":         "Filesystem 1024-blocks Used Available Capacity Mounted on\n/dev/sda1 1000 410 590 41% /",
		"failed":          "2",
		"reboot-required": "yes",
		"sshd -T":         "passwordauthentication no\nkbdinteractiveauthentication no\npermitrootlogin prohibit-password",
		"ssh-keygen":      "256 SHA256:abc deploy (ED25519)",
	}
	run := func(cmd string) (string, error) {
		for key, out := range outputs {
			if strings.Contains(cmd, key) {
				return out, nil
			}
		}
		return "", nil
	}
	c := facts.New("", "")
	f := c.Parse(run, 7)
	if f.HostID != 7 || f.OS != "Debian GNU/Linux 13 (trixie)" || f.Kernel != "6.12.0-1-amd64" || f.Hostname != "acme" {
		t.Errorf("base facts: %+v", f)
	}
	if f.DiskUsedPct != 41 || f.FailedUnits != 2 || f.RebootRequired != "yes" {
		t.Errorf("numbers: %+v", f)
	}
	if f.PasswordAuth != "no" || f.RootLogin != "prohibit-password" {
		t.Errorf("sshd: %+v", f)
	}
	if !strings.Contains(f.KeyFingerprints, "SHA256:abc") {
		t.Errorf("keys: %+v", f)
	}
}

func TestParsePasswordAuth(t *testing.T) {
	run := func(cmd string) (string, error) {
		if strings.Contains(cmd, "sshd -T") {
			return "passwordauthentication no\nkbdinteractiveauthentication yes\npermitrootlogin yes", nil
		}
		return "", nil
	}
	f := facts.New("", "").Parse(run, 1)
	if f.PasswordAuth != "yes" || f.RootLogin != "yes" {
		t.Errorf("kbd-interactive should count as password auth: %+v", f)
	}
}
