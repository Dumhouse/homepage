package inventory

import (
	"context"
	"fmt"
)

// Conflicts lists non-blocking warnings for a host: an address already used by
// another host, or a port already served on the same address.
func (s *Store) Conflicts(ctx context.Context, h *Host) ([]string, error) {
	warnings := []string{}
	seen := map[string]bool{}
	warn := func(format string, a ...any) {
		msg := fmt.Sprintf(format, a...)
		if !seen[msg] {
			seen[msg] = true
			warnings = append(warnings, msg)
		}
	}
	for _, a := range h.Addresses {
		rows, err := s.db.QueryContext(ctx,
			`SELECT h.name FROM host_addresses ha JOIN hosts h ON h.id=ha.host_id
			 WHERE ha.address=? AND ha.host_id<>? AND h.deleted_at IS NULL`, a.Address, h.ID)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var name string
			if err := rows.Scan(&name); err != nil {
				rows.Close()
				return nil, err
			}
			warn("%s is also used by %s", a.Address, name)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}
	}
	for _, a := range h.Addresses {
		for _, v := range h.Services {
			if v.Port == 0 || v.Protocol == "udp" {
				continue
			}
			rows, err := s.db.QueryContext(ctx,
				`SELECT h.name,s.name FROM services s
				 JOIN hosts h ON h.id=s.host_id AND h.deleted_at IS NULL
				 JOIN host_addresses ha ON ha.host_id=h.id
				 WHERE ha.address=? AND s.port=? AND s.id<>? AND s.deleted_at IS NULL
				   AND s.protocol<>'udp' AND h.id<>?`, a.Address, v.Port, v.ID, h.ID)
			if err != nil {
				return nil, err
			}
			for rows.Next() {
				var host, svc string
				if err := rows.Scan(&host, &svc); err != nil {
					rows.Close()
					return nil, err
				}
				warn("%s:%d is also used by %s/%s", a.Address, v.Port, host, svc)
			}
			rows.Close()
			if err := rows.Err(); err != nil {
				return nil, err
			}
		}
	}
	return warnings, nil
}

func (s *Store) withWarnings(ctx context.Context, h *Host, err error) (*Host, error) {
	if err != nil {
		return nil, err
	}
	if h.Warnings, err = s.Conflicts(ctx, h); err != nil {
		return nil, err
	}
	if len(h.Warnings) == 0 {
		h.Warnings = nil
	}
	return h, nil
}

// FreePort returns the first free TCP port at or above near on the same
// addresses as hostID (so two hosts on one bridge don't collide).
func (s *Store) FreePort(ctx context.Context, hostID int64, near int) (int, error) {
	if _, err := s.GetHost(ctx, hostID); err != nil {
		return 0, err
	}
	if near < 1 || near > 65535 {
		near = 3000
	}
	used := map[int]bool{}
	rows, err := s.db.QueryContext(ctx,
		`SELECT s.port FROM services s
		 JOIN hosts h ON h.id=s.host_id AND h.deleted_at IS NULL
		 WHERE s.deleted_at IS NULL AND s.port IS NOT NULL AND s.port>0
		   AND (h.id=? OR h.id IN (
		     SELECT a2.host_id FROM host_addresses a1 JOIN host_addresses a2 ON a2.address=a1.address
		     WHERE a1.host_id=?
		   ))`, hostID, hostID)
	if err != nil {
		return 0, err
	}
	for rows.Next() {
		var p int
		if err := rows.Scan(&p); err != nil {
			rows.Close()
			return 0, err
		}
		used[p] = true
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}
	for p := near; p <= 65535; p++ {
		if !used[p] {
			return p, nil
		}
	}
	return 0, invalid("no free port above %d", near)
}

// itoa avoids importing strconv in every small file.
func itoa(n int64) string { return fmt.Sprintf("%d", n) }
