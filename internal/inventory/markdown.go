package inventory

import (
	"context"
	"fmt"
	"strings"
)

// HostMarkdown renders a compact host summary for MCP resources and /inventory.md.
func (s *Store) HostMarkdown(ctx context.Context, id int64) (string, error) {
	h, err := s.GetHost(ctx, id)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n%s · %s", h.Name, h.Kind, h.Lifecycle)
	if h.Location != nil {
		fmt.Fprintf(&b, " · %s", h.Location.Name)
	}
	b.WriteString("\n")
	if h.Hostname != "" {
		fmt.Fprintf(&b, "Hostname: `%s`\n", h.Hostname)
	}
	if len(h.Tags) > 0 {
		fmt.Fprintf(&b, "Tags: %s\n", strings.Join(h.Tags, ", "))
	}
	if h.Description != "" {
		fmt.Fprintf(&b, "\n%s\n", h.Description)
	}
	if len(h.Addresses) > 0 {
		b.WriteString("\n## Addresses\n")
		for _, a := range h.Addresses {
			primary := ""
			if a.IsPrimary {
				primary = " (primary)"
			}
			fmt.Fprintf(&b, "- %s `%s`%s\n", a.Kind, a.Address, primary)
		}
	}
	if len(h.SSH) > 0 {
		b.WriteString("\n## SSH\n")
		for _, x := range h.SSH {
			host := ""
			if len(h.Addresses) > 0 {
				host = h.Addresses[0].Address
			}
			fmt.Fprintf(&b, "- `ssh %s@%s -p %d` — %s", x.User, host, x.Port, x.Status)
			if x.Vault != "" {
				fmt.Fprintf(&b, " ([Vaultwarden](%s))", x.Vault)
			}
			b.WriteString("\n")
		}
	}
	if len(h.Services) > 0 {
		b.WriteString("\n## Services\n\n| name | port | protocol | domain |\n|---|---|---|---|\n")
		for _, v := range h.Services {
			fmt.Fprintf(&b, "| %s | %d | %s | %s |\n", v.Name, v.Port, v.Protocol, v.Domain)
		}
	}
	if len(h.Projects) > 0 {
		b.WriteString("\n## Projects\n")
		for _, p := range h.Projects {
			fmt.Fprintf(&b, "- %s (`%s`)", p.Name, p.Slug)
			if p.Env != "" {
				fmt.Fprintf(&b, " — %s env", p.Env)
			}
			b.WriteString("\n")
		}
	}
	writeAccess(&b, h.Access)
	return b.String(), nil
}

// ProjectMarkdown renders a compact project summary for MCP resources.
func (s *Store) ProjectMarkdown(ctx context.Context, slug string) (string, error) {
	p, err := s.GetProjectBySlug(ctx, slug)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n`%s` · %s", p.Name, p.Slug, p.Status)
	if p.Group != "" {
		fmt.Fprintf(&b, " · %s", p.Group)
	}
	if p.OpenSource {
		b.WriteString(" · open source")
	}
	b.WriteString("\n")
	if p.Description != "" {
		fmt.Fprintf(&b, "\n%s\n", p.Description)
	}
	if len(p.Links) > 0 {
		b.WriteString("\n## Links\n")
		for _, l := range p.Links {
			fmt.Fprintf(&b, "- %s: %s\n", l.Label, l.URL)
		}
	}
	if len(p.Envs) > 0 {
		b.WriteString("\n## Environments\n")
		for _, e := range p.Envs {
			fmt.Fprintf(&b, "\n### %s", e.Name)
			if e.BaseURL != "" {
				fmt.Fprintf(&b, " — %s", e.BaseURL)
			}
			b.WriteString("\n")
			if len(e.Links) > 0 {
				b.WriteString("\n| link | url | kind |\n|---|---|---|\n")
				for _, l := range e.Links {
					fmt.Fprintf(&b, "| %s | %s | %s |\n", l.Label, l.URL, l.Kind)
				}
			}
			if len(e.Hosts) > 0 {
				b.WriteString("\nHosts: ")
				for i, h := range e.Hosts {
					if i > 0 {
						b.WriteString(", ")
					}
					b.WriteString(h.Name)
				}
				b.WriteString("\n")
			}
			if len(e.Services) > 0 {
				b.WriteString("Services: ")
				for i, v := range e.Services {
					if i > 0 {
						b.WriteString(", ")
					}
					fmt.Fprintf(&b, "%s:%d", v.Name, v.Port)
				}
				b.WriteString("\n")
			}
		}
	}
	if len(p.Connections) > 0 {
		b.WriteString("\n## Connected services\n")
		for _, c := range p.Connections {
			fmt.Fprintf(&b, "- %s: %s", c.Role, c.TargetKind)
			if c.TargetID != nil {
				fmt.Fprintf(&b, " #%d", *c.TargetID)
			}
			if c.URL != "" {
				fmt.Fprintf(&b, " %s", c.URL)
			}
			if c.Detail != "" {
				fmt.Fprintf(&b, " — %s", c.Detail)
			}
			if c.ImpactNote != "" {
				fmt.Fprintf(&b, " (if down: %s)", c.ImpactNote)
			}
			b.WriteString("\n")
		}
	}
	writeAccess(&b, p.Access)
	return b.String(), nil
}

// InventoryMarkdown is the whole inventory as one Markdown page for agents
// without MCP support (/inventory.md).
func (s *Store) InventoryMarkdown(ctx context.Context) (string, error) {
	data, err := s.Export(ctx)
	if err != nil {
		return "", err
	}
	var b strings.Builder
	b.WriteString("# Home Services inventory\n\n")
	fmt.Fprintf(&b, "_Generated %s. %d hosts, %d projects, %d locations._\n",
		data.ExportedAt, len(data.Hosts), len(data.Projects), len(data.Locations))

	b.WriteString("\n## Hosts\n\n| host | kind | location | primary address | services |\n|---|---|---|---|---|\n")
	for _, h := range data.Hosts {
		loc := ""
		if h.Location != nil {
			loc = h.Location.Name
		}
		addr := ""
		if len(h.Addresses) > 0 {
			addr = h.Addresses[0].Address
		}
		var svcs []string
		for _, v := range h.Services {
			s := v.Name
			if v.Port != 0 {
				s += fmt.Sprintf(":%d", v.Port)
			}
			svcs = append(svcs, s)
		}
		fmt.Fprintf(&b, "| %s | %s | %s | %s | %s |\n", h.Name, h.Kind, loc, addr, strings.Join(svcs, ", "))
	}

	b.WriteString("\n## Projects\n\n| project | slug | group | envs |\n|---|---|---|---|\n")
	for _, p := range data.Projects {
		var envs []string
		for _, e := range p.Envs {
			envs = append(envs, e.Name)
		}
		fmt.Fprintf(&b, "| %s | %s | %s | %s |\n", p.Name, p.Slug, p.Group, strings.Join(envs, ", "))
	}

	if len(data.Locations) > 0 {
		b.WriteString("\n## Locations\n")
		for _, l := range data.Locations {
			fmt.Fprintf(&b, "- %s (%s) — %d host(s)\n", l.Name, l.Kind, l.HostCount)
		}
	}
	if len(data.Tags) > 0 {
		var tags []string
		for _, t := range data.Tags {
			tags = append(tags, t.Name)
		}
		fmt.Fprintf(&b, "\nTags: %s\n", strings.Join(tags, ", "))
	}
	return b.String(), nil
}

func writeAccess(b *strings.Builder, list []Access) {
	if len(list) == 0 {
		return
	}
	b.WriteString("\n## Access\n")
	for _, a := range list {
		fmt.Fprintf(b, "- %s — %s", a.Label, a.Method)
		if a.Account != "" {
			fmt.Fprintf(b, " as `%s`", a.Account)
		}
		if a.TwoFA != "" && a.TwoFA != "none" {
			fmt.Fprintf(b, ", 2FA %s", a.TwoFA)
		}
		if a.URL != "" {
			fmt.Fprintf(b, " — %s", a.URL)
		}
		if a.VaultItemURL != "" {
			fmt.Fprintf(b, " ([Vaultwarden](%s))", a.VaultItemURL)
		}
		b.WriteString("\n")
	}
}
