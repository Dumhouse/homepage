// Package facts collects a read-only snapshot of a host over SSH: OS, kernel,
// uptime, disk, failed units, reboot flag, sshd auth flags and key fingerprints.
package facts

import (
	"context"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/dawidof/home-services/internal/inventory"
)

type Collector struct {
	KeyPath string
	User    string
	Timeout time.Duration
}

func New(keyPath, user string) *Collector {
	if user == "" {
		user = "inventory"
	}
	return &Collector{KeyPath: keyPath, User: user, Timeout: 10 * time.Second}
}

// Collect dials the host with the configured key and returns the snapshot.
func (c *Collector) Collect(ctx context.Context, address string) (*inventory.HostFacts, error) {
	if c.KeyPath == "" {
		return nil, fmt.Errorf("SSH facts are not configured (set HS_SSH_KEY)")
	}
	raw, err := os.ReadFile(c.KeyPath)
	if err != nil {
		return nil, fmt.Errorf("read SSH key: %w", err)
	}
	signer, err := ssh.ParsePrivateKey(raw)
	if err != nil {
		return nil, fmt.Errorf("parse SSH key: %w", err)
	}
	if !strings.Contains(address, ":") {
		address += ":22"
	}
	cfg := &ssh.ClientConfig{
		User: c.User, Auth: []ssh.AuthMethod{ssh.PublicKeys(signer)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), Timeout: c.Timeout,
	}
	d := net.Dialer{Timeout: c.Timeout}
	conn, err := d.DialContext(ctx, "tcp", address)
	if err != nil {
		return nil, err
	}
	cc, chans, reqs, err := ssh.NewClientConn(conn, address, cfg)
	if err != nil {
		conn.Close()
		return nil, err
	}
	client := ssh.NewClient(cc, chans, reqs)
	defer client.Close()
	run := func(cmd string) (string, error) {
		session, err := client.NewSession()
		if err != nil {
			return "", err
		}
		defer session.Close()
		out, err := session.Output(cmd)
		return string(out), err
	}
	return c.Parse(run, 0), nil
}

// Parse runs the fact commands through run and parses the output. It is exported
// for tests; failures are tolerated and leave the field empty.
func (c *Collector) Parse(run func(string) (string, error), hostID int64) *inventory.HostFacts {
	f := &inventory.HostFacts{HostID: hostID, PasswordAuth: "unknown", RootLogin: "unknown"}
	if out, err := run(`grep -m1 '^PRETTY_NAME=' /etc/os-release 2>/dev/null`); err == nil {
		f.OS = strings.Trim(strings.TrimPrefix(strings.TrimSpace(out), "PRETTY_NAME="), `"`)
	}
	if out, err := run("uname -r 2>/dev/null"); err == nil {
		f.Kernel = strings.TrimSpace(out)
	}
	if out, err := run("hostname 2>/dev/null"); err == nil {
		f.Hostname = strings.TrimSpace(out)
	}
	if out, err := run("uptime -p 2>/dev/null"); err == nil {
		f.Uptime = strings.TrimSpace(out)
	}
	if out, err := run("df -P / 2>/dev/null | tail -1"); err == nil {
		lines := strings.Split(strings.TrimSpace(out), "\n")
		fields := strings.Fields(lines[len(lines)-1])
		if len(fields) >= 5 {
			f.DiskUsedPct, _ = strconv.Atoi(strings.TrimSuffix(fields[4], "%"))
		}
	}
	if out, err := run("systemctl --failed --no-legend --plain 2>/dev/null | wc -l"); err == nil {
		f.FailedUnits, _ = strconv.Atoi(strings.TrimSpace(out))
	}
	if out, err := run("test -f /var/run/reboot-required && echo yes || echo no"); err == nil {
		f.RebootRequired = strings.TrimSpace(out)
	}
	sshdOut, sshdErr := run(`sshd -T 2>/dev/null | grep -Ei '^(passwordauthentication|kbdinteractiveauthentication|permitrootlogin)'`)
	if sshdErr != nil || strings.TrimSpace(sshdOut) == "" {
		sshdOut, _ = run(`grep -Ehsi '^(PasswordAuthentication|KbdInteractiveAuthentication|PermitRootLogin)' /etc/ssh/sshd_config /etc/ssh/sshd_config.d/*.conf 2>/dev/null`)
	}
	f.PasswordAuth, f.RootLogin = parseSSHD(sshdOut)
	if out, err := run(`cat /home/*/.ssh/authorized_keys /root/.ssh/authorized_keys 2>/dev/null | ssh-keygen -lf - 2>/dev/null | head -5`); err == nil {
		f.KeyFingerprints = strings.TrimSpace(out)
	}
	if f.OS == "" && f.Kernel == "" && f.Hostname == "" {
		f.Error = "no fact commands returned output (is the read-only user allowed?)"
	}
	return f
}

func parseSSHD(out string) (password, root string) {
	password, root = "unknown", "unknown"
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) < 2 {
			continue
		}
		key, val := strings.ToLower(fields[0]), strings.ToLower(fields[1])
		switch key {
		case "passwordauthentication", "kbdinteractiveauthentication":
			if val == "yes" {
				password = "yes"
			} else if password != "yes" {
				password = "no"
			}
		case "permitrootlogin":
			root = val
		}
	}
	return password, root
}
