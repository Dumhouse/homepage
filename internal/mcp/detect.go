package mcp

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"

	sdk "github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/dawidof/home-services/internal/changesets"
	"github.com/dawidof/home-services/internal/discovery"
)

type detectArgs struct {
	Target string `json:"target" jsonschema:"an address, host:port or URL to fingerprint"`
	Ports  []int  `json:"ports,omitempty" jsonschema:"extra ports to probe (default: common ports plus the URL port)"`
}

type detectOut struct {
	Address  string       `json:"address"`
	Hostname string       `json:"hostname,omitempty"`
	Ports    []detectPort `json:"ports"`
}

type detectPort struct {
	Port    int    `json:"port"`
	Service string `json:"service,omitempty"`
	Title   string `json:"title,omitempty"`
	Banner  string `json:"banner,omitempty"`
}

type requestConnectionArgs struct {
	Project    string `json:"project" jsonschema:"project slug or name"`
	TargetKind string `json:"target_kind" jsonschema:"host, service, provider or url"`
	TargetID   int64  `json:"target_id,omitempty"`
	URL        string `json:"url,omitempty"`
	Role       string `json:"role,omitempty"`
	Detail     string `json:"detail,omitempty"`
	ImpactNote string `json:"impact_note,omitempty"`
}

func addDetectTools(s *sdk.Server, cs *changesets.Store) {
	addRead(s, "detect", "Fingerprint what answers at an address or URL: open ports, service guess, HTTP title, banner.",
		func(ctx context.Context, _ *sdk.CallToolRequest, a detectArgs) (*sdk.CallToolResult, detectOut, error) {
			out, err := detect(ctx, a.Target, a.Ports)
			if err != nil {
				return nil, detectOut{}, err
			}
			return text(fmt.Sprintf("%s: %d open port(s)", out.Address, len(out.Ports)), out)
		})

	addWrite(s, "request_connection", "Propose that a project depends on a service/host/URL; the user reviews it in Changes.", false,
		func(ctx context.Context, _ *sdk.CallToolRequest, a requestConnectionArgs) (*sdk.CallToolResult, changesets.ChangeSet, error) {
			if err := needPropose(ctx); err != nil {
				return nil, changesets.ChangeSet{}, err
			}
			data := map[string]string{
				"project": a.Project, "target_kind": a.TargetKind, "url": a.URL,
				"role": a.Role, "detail": a.Detail, "impact_note": a.ImpactNote,
			}
			if a.TargetID != 0 {
				data["target_id"] = strconv.FormatInt(a.TargetID, 10)
			}
			created, err := cs.Create(ctx, actor(ctx), "Connection: "+a.Project+" → "+a.TargetKind,
				"mcp", []changesets.Op{{
					Action: "create", Entity: "connection", Ref: a.Project, Data: data,
					Note: "Requested by an agent",
				}})
			if err != nil {
				return nil, changesets.ChangeSet{}, err
			}
			return text(fmt.Sprintf("Proposed connection #%d — review on /changesets", created.ID), *created)
		})
}

func detect(ctx context.Context, target string, extra []int) (detectOut, error) {
	target = strings.TrimSpace(target)
	if target == "" {
		return detectOut{}, fmt.Errorf("target is required")
	}
	port := 0
	host := target
	webPort := false
	if strings.Contains(target, "://") {
		u, err := url.Parse(target)
		if err != nil || u.Hostname() == "" {
			return detectOut{}, fmt.Errorf("invalid URL %q", target)
		}
		host = u.Hostname()
		if p := u.Port(); p != "" {
			port, _ = strconv.Atoi(p)
		}
		webPort = true
	} else if h, p, err := net.SplitHostPort(target); err == nil {
		host, port = h, atoi(p)
		webPort = true
	}
	ips, err := net.LookupHost(host)
	if err != nil || len(ips) == 0 {
		return detectOut{}, fmt.Errorf("cannot resolve %q", host)
	}
	ip := ips[0]
	ports := []int{}
	seen := map[int]bool{}
	for _, p := range append(append([]int{}, discovery.DefaultPorts...), extra...) {
		if p > 0 && !seen[p] {
			seen[p] = true
			ports = append(ports, p)
		}
	}
	if port > 0 && !seen[port] {
		ports = append(ports, port)
	}
	sc := discovery.New()
	sc.Timeout = 500 * time.Millisecond
	sc.Ports = ports
	if webPort && port > 0 {
		sc.WebPorts = map[int]bool{port: true}
	}
	res, err := sc.Scan(ctx, ip+"/32")
	if err != nil {
		return detectOut{}, err
	}
	if len(res) == 0 {
		return detectOut{Address: ip}, nil
	}
	out := detectOut{Address: res[0].Address, Hostname: res[0].Hostname}
	for _, p := range res[0].Ports {
		out.Ports = append(out.Ports, detectPort{Port: p.Port, Service: p.Service, Title: p.Title, Banner: p.Banner})
	}
	return out, nil
}

func atoi(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}
