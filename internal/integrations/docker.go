package integrations

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/ssh"
)

// Docker lists running containers over SSH and proposes services for their
// published ports. Read-only: only `docker ps`/`docker version` are run.
func Docker() *Plugin {
	return &Plugin{
		ID:          "docker",
		Name:        "Docker",
		Description: "Read running containers over SSH and propose a service per published port.",
		Fields: []Field{
			{Key: "targets", Label: "SSH targets", Kind: "text", Required: true, Help: "Comma-separated user@host[:port] with Docker access, e.g. deploy@192.168.1.240"},
			{Key: "key", Label: "Private key", Kind: "password", Secret: true, Required: true, Help: "PEM key for those users (stored encrypted, used read-only)."},
		},
		Test: func(ctx context.Context, cfg Config) error {
			targets, err := parseDockerTargets(cfg["targets"])
			if err != nil {
				return err
			}
			if _, err := dockerKey(cfg); err != nil {
				return err
			}
			for _, t := range targets {
				if _, err := dockerRun(ctx, cfg, t, "docker version --format '{{.Server.Version}}'"); err != nil {
					return fmt.Errorf("%s: %w", t.raw, err)
				}
			}
			return nil
		},
		Sync: func(ctx context.Context, cfg Config) ([]Op, error) {
			targets, err := parseDockerTargets(cfg["targets"])
			if err != nil {
				return nil, err
			}
			out := []Op{}
			var failures []string
			for _, t := range targets {
				stdout, err := dockerRun(ctx, cfg, t, `docker ps --format '{{json .}}'`)
				if err != nil {
					failures = append(failures, t.raw+": "+err.Error())
					continue
				}
				for _, line := range strings.Split(stdout, "\n") {
					line = strings.TrimSpace(line)
					if line == "" {
						continue
					}
					var c struct {
						Names  string `json:"Names"`
						Image  string `json:"Image"`
						State  string `json:"State"`
						Status string `json:"Status"`
						Ports  string `json:"Ports"`
					}
					if json.Unmarshal([]byte(line), &c) != nil {
						continue
					}
					seen := map[int]bool{}
					for _, pm := range parseDockerPorts(c.Ports) {
						if seen[pm.HostPort] {
							continue
						}
						seen[pm.HostPort] = true
						name := c.Names
						if name == "" {
							name = c.Image
						}
						if len(parseDockerPorts(c.Ports)) > 1 {
							name += fmt.Sprintf("-%d", pm.ContainerPort)
						}
						protocol := "tcp"
						if pm.ContainerPort == 80 || pm.ContainerPort == 3000 || pm.ContainerPort == 8080 ||
							pm.ContainerPort == 8000 || pm.ContainerPort == 9090 {
							protocol = "http"
						}
						if pm.ContainerPort == 443 || pm.ContainerPort == 8443 {
							protocol = "https"
						}
						out = append(out, Op{
							Action: "create", Entity: "service", Ref: t.host + ":" + strconv.Itoa(pm.HostPort),
							Data: map[string]string{
								"host": t.host, "name": name, "port": strconv.Itoa(pm.HostPort),
								"protocol": protocol, "type": c.Image, "notes": c.Status,
							},
							Note: "Docker container " + c.Names + " on " + t.raw,
						})
					}
				}
			}
			if len(out) == 0 && len(failures) > 0 {
				return nil, fmt.Errorf("docker: %s", strings.Join(failures, "; "))
			}
			if len(failures) > 0 {
				return out, fmt.Errorf("some targets failed: %s", strings.Join(failures, "; "))
			}
			return out, nil
		},
	}
}

type dockerTarget struct {
	raw, user, host string
	port            int
}

func parseDockerTargets(raw string) ([]dockerTarget, error) {
	parts := strings.FieldsFunc(raw, func(r rune) bool { return r == ',' || r == '\n' || r == ' ' || r == ';' })
	out := []dockerTarget{}
	for _, p := range parts {
		user, rest, ok := strings.Cut(strings.TrimSpace(p), "@")
		if !ok || user == "" || rest == "" {
			return nil, fmt.Errorf("%w: write targets as user@host[:port]", ErrInvalid)
		}
		t := dockerTarget{raw: p, user: user, host: rest, port: 22}
		if h, ps, err := net.SplitHostPort(rest); err == nil {
			t.host = h
			t.port, _ = strconv.Atoi(ps)
		}
		out = append(out, t)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%w: at least one target is required", ErrInvalid)
	}
	return out, nil
}

func dockerKey(cfg Config) (ssh.Signer, error) {
	if strings.TrimSpace(cfg["key"]) == "" {
		return nil, fmt.Errorf("%w: private key is required", ErrInvalid)
	}
	signer, err := ssh.ParsePrivateKey([]byte(cfg["key"]))
	if err != nil {
		return nil, fmt.Errorf("private key: %w", err)
	}
	return signer, nil
}

func dockerRun(ctx context.Context, cfg Config, t dockerTarget, command string) (string, error) {
	signer, err := dockerKey(cfg)
	if err != nil {
		return "", err
	}
	addr := net.JoinHostPort(t.host, strconv.Itoa(t.port))
	d := net.Dialer{Timeout: 8 * time.Second}
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return "", err
	}
	cc, chans, reqs, err := ssh.NewClientConn(conn, addr, &ssh.ClientConfig{
		User: t.user, Auth: []ssh.AuthMethod{ssh.PublicKeys(signer)},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(), Timeout: 8 * time.Second,
	})
	if err != nil {
		conn.Close()
		return "", err
	}
	client := ssh.NewClient(cc, chans, reqs)
	defer client.Close()
	session, err := client.NewSession()
	if err != nil {
		return "", err
	}
	defer session.Close()
	out, err := session.Output(command)
	return string(out), err
}

type dockerPort struct {
	HostPort      int
	ContainerPort int
}

func parseDockerPorts(s string) []dockerPort {
	out := []dockerPort{}
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		i := strings.Index(part, "->")
		if i < 0 {
			continue
		}
		left, right := part[:i], part[i+2:]
		hp := left[strings.LastIndex(left, ":")+1:]
		cp := strings.SplitN(right, "/", 2)[0]
		hostPort, err1 := strconv.Atoi(hp)
		containerPort, err2 := strconv.Atoi(cp)
		if err1 == nil && err2 == nil {
			out = append(out, dockerPort{HostPort: hostPort, ContainerPort: containerPort})
		}
	}
	return out
}
