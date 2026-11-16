package checks

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/dawidof/home-services/internal/inventory"
)

// Reconcile makes sure every service, SSH login and health-checked app link has
// an auto check with fresh resolved params, and removes checks for vanished
// targets. Manual checks are never touched.
func (s *Store) Reconcile(ctx context.Context, inv *inventory.Store) (int, error) {
	changed := 0
	hosts, err := inv.ListHosts(ctx)
	if err != nil {
		return changed, err
	}
	for _, row := range hosts {
		h, err := inv.GetHost(ctx, row.ID)
		if err != nil {
			continue
		}
		addr := primaryAddress(h)
		for _, v := range h.Services {
			roles := []string{}
			for _, sc := range serviceChecks(v, addr) {
				c, err := s.upsertAuto(ctx, "service", v.ID, sc.kind, 300, sc.params)
				if err != nil {
					return changed, err
				}
				if c {
					changed++
				}
				roles = append(roles, sc.params["role"].(string))
			}
			if n, err := s.pruneAuto(ctx, "service", v.ID, roles); err != nil {
				return changed, err
			} else if n > 0 {
				changed++
			}
		}
		if addr == "" {
			continue
		}
		for _, x := range h.SSH {
			c, err := s.upsertAuto(ctx, "ssh_access", x.ID, "ssh", 3600, map[string]any{
				"address": addr, "port": x.Port, "user": x.User,
			})
			if err != nil {
				return changed, err
			}
			if c {
				changed++
			}
		}
	}

	projects, err := inv.ListProjects(ctx)
	if err != nil {
		return changed, err
	}
	seenLinks := map[int64]bool{}
	for _, row := range projects {
		p, err := inv.GetProject(ctx, row.ID)
		if err != nil {
			continue
		}
		for _, env := range p.Envs {
			for _, link := range env.Links {
				if !link.HealthCheck || link.URL == "" || seenLinks[link.ID] {
					continue
				}
				seenLinks[link.ID] = true
				c, err := s.upsertAuto(ctx, "app_link", link.ID, "http", 300, map[string]any{"url": link.URL})
				if err != nil {
					return changed, err
				}
				if c {
					changed++
				}
			}
		}
	}

	if _, err := s.db.ExecContext(ctx, `DELETE FROM checks WHERE auto=1 AND (
		(target_entity='service' AND target_id NOT IN (SELECT id FROM services WHERE deleted_at IS NULL))
		OR (target_entity='app_link' AND target_id NOT IN (SELECT id FROM app_links WHERE deleted_at IS NULL))
		OR (target_entity='host' AND target_id NOT IN (SELECT id FROM hosts WHERE deleted_at IS NULL))
		OR (target_entity='ssh_access' AND target_id NOT IN (
			SELECT x.id FROM ssh_access x JOIN hosts h ON h.id=x.host_id WHERE h.deleted_at IS NULL))
	)`); err != nil {
		return changed, err
	}
	return changed, nil
}

func primaryAddress(h *inventory.Host) string {
	if len(h.Addresses) == 0 {
		return ""
	}
	return h.Addresses[0].Address
}

type autoCheck struct {
	kind   string
	params map[string]any
}

// serviceChecks returns the automatic checks for a service: "lan" probes the
// host address (HTTP for web services, else a TCP connect) and "public" probes
// the domain. A service with both gets both, so the dashboard can tell a LAN
// outage from a broken public route.
func serviceChecks(v inventory.Service, addr string) []autoCheck {
	out := []autoCheck{}
	path := v.Path
	if path != "" && path[0] != '/' {
		path = "/" + path
	}
	if v.Port > 0 && addr != "" && v.Protocol != "udp" {
		if v.Protocol == "http" || v.Protocol == "https" {
			out = append(out, autoCheck{"http", map[string]any{"role": "lan", "expect_status": "200-499",
				"url": v.Protocol + "://" + net.JoinHostPort(addr, strconv.Itoa(v.Port)) + path}})
		} else {
			out = append(out, autoCheck{"tcp", map[string]any{"role": "lan", "address": addr, "port": v.Port}})
		}
	}
	if v.Domain != "" {
		scheme := "https"
		if v.Protocol == "http" {
			scheme = "http"
		}
		out = append(out, autoCheck{"http", map[string]any{"role": "public", "expect_status": "200-499",
			"url": scheme + "://" + v.Domain + path}})
	}
	return out
}

func serviceHTTPURL(v inventory.Service, addr string) string {
	path := v.Path
	if path != "" && path[0] != '/' {
		path = "/" + path
	}
	if v.Domain != "" {
		scheme := "https"
		if v.Protocol == "http" {
			scheme = "http"
		}
		return scheme + "://" + v.Domain + path
	}
	if v.Port > 0 && (v.Protocol == "http" || v.Protocol == "https") && addr != "" {
		return v.Protocol + "://" + net.JoinHostPort(addr, strconv.Itoa(v.Port)) + path
	}
	return ""
}

// upsertAuto creates or refreshes the single auto check for a target. It returns
// true when something changed.
func (s *Store) upsertAuto(ctx context.Context, entity string, id int64, kind string, interval int, params map[string]any) (bool, error) {
	raw, _ := json.Marshal(params)
	role, _ := params["role"].(string)
	var checkID int64
	var oldKind, oldParams string
	var oldInterval int
	// one auto check per target and role; a role-less legacy check is adopted
	err := s.db.QueryRowContext(ctx,
		`SELECT id,kind,params,interval_s FROM checks
		 WHERE target_entity=? AND target_id=? AND auto=1 AND deleted_at IS NULL
		   AND COALESCE(json_extract(params,'$.role'),'') IN (?, '')
		 ORDER BY COALESCE(json_extract(params,'$.role'),'') = ? DESC, id LIMIT 1`, entity, id, role, role).
		Scan(&checkID, &oldKind, &oldParams, &oldInterval)
	switch {
	case err == nil:
		if oldKind == kind && oldParams == string(raw) && oldInterval == interval {
			return false, nil
		}
		_, err = s.db.ExecContext(ctx,
			`UPDATE checks SET kind=?,params=?,interval_s=?,enabled=1 WHERE id=?`,
			kind, string(raw), interval, checkID)
		return true, err
	case err != sql.ErrNoRows:
		return false, err
	}
	// No auto check yet: don't duplicate a manual check of the same kind and role.
	var n int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM checks WHERE target_entity=? AND target_id=? AND kind=? AND auto=0 AND deleted_at IS NULL
		   AND COALESCE(json_extract(params,'$.role'),'') IN (?, '')`,
		entity, id, kind, role).Scan(&n); err != nil {
		return false, err
	}
	if n > 0 {
		return false, nil
	}
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO checks(target_entity,target_id,kind,interval_s,params,enabled,auto,state)
		 VALUES(?,?,?,?,?,1,1,'unknown')`, entity, id, kind, interval, string(raw))
	if err != nil {
		return false, fmt.Errorf("auto check %s #%d: %w", entity, id, err)
	}
	return true, nil
}

// pruneAuto deletes auto checks of a target whose role is no longer wanted
// (e.g. the domain was removed from a service).
func (s *Store) pruneAuto(ctx context.Context, entity string, id int64, keep []string) (int64, error) {
	q := `DELETE FROM checks WHERE target_entity=? AND target_id=? AND auto=1`
	args := []any{entity, id}
	if len(keep) > 0 {
		q += ` AND COALESCE(json_extract(params,'$.role'),'') NOT IN (` + strings.TrimSuffix(strings.Repeat("?,", len(keep)), ",") + `)`
		for _, k := range keep {
			args = append(args, k)
		}
	}
	res, err := s.db.ExecContext(ctx, q, args...)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// CheckRole is "lan", "public" or "" for a check's params.
func (c Check) Role() string {
	r, _ := c.Params["role"].(string)
	return r
}
