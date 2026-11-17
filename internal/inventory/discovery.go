package inventory

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

// DiscoveredHost is an address seen by a subnet scan, with its open ports.
type DiscoveredHost struct {
	ID          int64            `json:"id"`
	Address     string           `json:"address"`
	Hostname    string           `json:"hostname,omitempty"`
	FirstSeenAt string           `json:"first_seen_at"`
	LastSeenAt  string           `json:"last_seen_at"`
	Status      string           `json:"status"` // new | ignored | registered
	HostID      *int64           `json:"host_id,omitempty"`
	Note        string           `json:"note,omitempty"`
	Ports       []DiscoveredPort `json:"ports,omitempty"`
}

// DiscoveredPort is one open TCP port with whatever banner/title we saw.
type DiscoveredPort struct {
	ID           int64  `json:"id"`
	Address      string `json:"address"`
	Port         int    `json:"port"`
	Protocol     string `json:"protocol"`
	ServiceGuess string `json:"service_guess,omitempty"`
	Title        string `json:"title,omitempty"`
	Banner       string `json:"banner,omitempty"`
	LastSeenAt   string `json:"last_seen_at"`
	Ignored      bool   `json:"ignored"`
}

// PortInfo is what the scanner hands to UpsertDiscovery.
type PortInfo struct {
	Port    int
	Service string
	Title   string
	Banner  string
}

// UpsertDiscovery stores a scan result and returns how many new hosts/ports were seen.
func (s *Store) UpsertDiscovery(ctx context.Context, address, hostname string, ports []PortInfo) (newHosts, newPorts int, err error) {
	err = s.tx(ctx, func(tx *sql.Tx) error {
		var id int64
		var status string
		e := tx.QueryRowContext(ctx, `SELECT id,status FROM discovered_hosts WHERE address=?`, address).Scan(&id, &status)
		if errors.Is(e, sql.ErrNoRows) {
			r, err := tx.ExecContext(ctx,
				`INSERT INTO discovered_hosts(address,hostname) VALUES(?,?)`, address, hostname)
			if err != nil {
				return err
			}
			id, _ = r.LastInsertId()
			newHosts = 1
			var known int
			if err := tx.QueryRowContext(ctx,
				`SELECT COUNT(*) FROM host_addresses a JOIN hosts h ON h.id=a.host_id AND h.deleted_at IS NULL WHERE a.address=?`, address).Scan(&known); err != nil {
				return err
			}
			if known > 0 { // already documented: record the ports, but do not announce a new device
				newHosts = 0
			}
		} else if e != nil {
			return e
		} else {
			if _, err := tx.ExecContext(ctx,
				`UPDATE discovered_hosts SET last_seen_at=datetime('now'),
				 hostname=CASE WHEN ?<>'' THEN ? ELSE hostname END WHERE id=?`, hostname, hostname, id); err != nil {
				return err
			}
		}
		for _, p := range ports {
			var exists int
			if err := tx.QueryRowContext(ctx,
				`SELECT COUNT(*) FROM discovered_ports WHERE address=? AND port=? AND protocol='tcp'`, address, p.Port).Scan(&exists); err != nil {
				return err
			}
			if exists == 0 {
				newPorts++
			}
			if _, err := tx.ExecContext(ctx,
				`INSERT INTO discovered_ports(address,port,service_guess,title,banner,last_seen_at)
				 VALUES(?,?,?,?,?,datetime('now'))
				 ON CONFLICT(address,port,protocol) DO UPDATE SET
				   last_seen_at=datetime('now'),
				   service_guess=COALESCE(NULLIF(excluded.service_guess,''), service_guess),
				   title=COALESCE(NULLIF(excluded.title,''), title),
				   banner=COALESCE(NULLIF(excluded.banner,''), banner)`,
				address, p.Port, p.Service, p.Title, p.Banner); err != nil {
				return err
			}
		}
		return nil
	})
	return newHosts, newPorts, err
}

// markKnownDiscovered moves inbox entries whose address already belongs to a live host to
// "registered", so hosts documented by hand, imported or created by an integration do not
// keep showing up as new devices.
func (s *Store) markKnownDiscovered(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE discovered_hosts
		   SET status='registered',
		       host_id=(SELECT a.host_id FROM host_addresses a JOIN hosts h ON h.id=a.host_id AND h.deleted_at IS NULL
		                 WHERE a.address=discovered_hosts.address LIMIT 1)
		 WHERE status='new'
		   AND EXISTS (SELECT 1 FROM host_addresses a JOIN hosts h ON h.id=a.host_id AND h.deleted_at IS NULL
		                WHERE a.address=discovered_hosts.address)`)
	return err
}

// ListDiscovered returns hosts by status (new, ignored, registered, or "" for all).
func (s *Store) ListDiscovered(ctx context.Context, status string) ([]DiscoveredHost, error) {
	if status == "" || status == "new" {
		if err := s.markKnownDiscovered(ctx); err != nil {
			return nil, err
		}
	}
	q := `SELECT id,address,COALESCE(hostname,''),first_seen_at,last_seen_at,status,host_id,COALESCE(note,'')
	      FROM discovered_hosts`
	var args []any
	if status != "" {
		q += ` WHERE status=?`
		args = append(args, status)
	}
	q += ` ORDER BY address`
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	var out []DiscoveredHost
	for rows.Next() {
		var d DiscoveredHost
		var hostID sql.NullInt64
		if err := rows.Scan(&d.ID, &d.Address, &d.Hostname, &d.FirstSeenAt, &d.LastSeenAt, &d.Status, &hostID, &d.Note); err != nil {
			rows.Close()
			return nil, err
		}
		if hostID.Valid {
			d.HostID = &hostID.Int64
		}
		out = append(out, d)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range out {
		list, err := s.discoveredPorts(ctx, out[i].Address)
		if err != nil {
			return nil, err
		}
		out[i].Ports = list
	}
	if out == nil {
		out = []DiscoveredHost{}
	}
	return out, nil
}

func (s *Store) discoveredPorts(ctx context.Context, address string) ([]DiscoveredPort, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id,address,port,protocol,COALESCE(service_guess,''),COALESCE(title,''),COALESCE(banner,''),last_seen_at,ignored
		 FROM discovered_ports WHERE address=? ORDER BY port`, address)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []DiscoveredPort{}
	for rows.Next() {
		var p DiscoveredPort
		if err := rows.Scan(&p.ID, &p.Address, &p.Port, &p.Protocol, &p.ServiceGuess, &p.Title, &p.Banner, &p.LastSeenAt, &p.Ignored); err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// IgnoreDiscovered hides one discovery result from the inbox.
func (s *Store) IgnoreDiscovered(ctx context.Context, actor string, id int64) error {
	return s.tx(ctx, func(tx *sql.Tx) error {
		r, err := tx.ExecContext(ctx,
			`UPDATE discovered_hosts SET status='ignored' WHERE id=? AND status='new'`, id)
		if err != nil {
			return err
		}
		if n, _ := r.RowsAffected(); n == 0 {
			return ErrNotFound
		}
		return s.audit(ctx, tx, actor, "ignore", "discovered_host", id, nil)
	})
}

// RegisterDiscovered creates (or reuses) a host from a scan result, adds
// services for its open ports and marks the result registered.
func (s *Store) RegisterDiscovered(ctx context.Context, actor string, id int64, name string) (*Host, error) {
	var address, hostname string
	var hostID sql.NullInt64
	err := s.db.QueryRowContext(ctx,
		`SELECT address,COALESCE(hostname,''),host_id FROM discovered_hosts WHERE id=?`, id).Scan(&address, &hostname, &hostID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	ports, err := s.discoveredPorts(ctx, address)
	if err != nil {
		return nil, err
	}
	name = strings.TrimSpace(name)
	if name == "" {
		name = hostname
	}
	if name == "" {
		name = "discovered-" + strings.ReplaceAll(address, ".", "-")
	}

	var host *Host
	if existing := s.hostByAddress(ctx, address); existing != 0 {
		host, err = s.GetHost(ctx, existing)
	} else {
		host, err = s.CreateHost(ctx, actor, Host{
			Name: name, Tags: []string{"discovered"},
			Addresses: []Address{{Kind: "lan", Address: address}},
		})
	}
	if err != nil {
		return nil, err
	}
	used := map[int]bool{}
	if list, err := s.ListServices(ctx, host.ID); err == nil {
		for _, v := range list {
			used[v.Port] = true
		}
	}
	for _, p := range ports {
		if p.Ignored || used[p.Port] {
			continue
		}
		protocol, svcName := portProtocol(p.Port, p.ServiceGuess)
		if _, err := s.AddService(ctx, actor, host.ID, Service{
			Name: svcName, Port: p.Port, Protocol: protocol, Notes: p.Title,
		}); err != nil {
			return nil, fmt.Errorf("port %d: %w", p.Port, err)
		}
	}
	err = s.tx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx,
			`UPDATE discovered_hosts SET status='registered',host_id=?,note=COALESCE(NULLIF(?,''),note) WHERE id=?`,
			host.ID, name, id); err != nil {
			return err
		}
		return s.audit(ctx, tx, actor, "register", "discovered_host", id, map[string]any{"host_id": host.ID})
	})
	return host, err
}

func (s *Store) hostByAddress(ctx context.Context, address string) int64 {
	var id int64
	if err := s.db.QueryRowContext(ctx,
		`SELECT h.id FROM hosts h JOIN host_addresses a ON a.host_id=h.id
		 WHERE a.address=? AND h.deleted_at IS NULL LIMIT 1`, address).Scan(&id); err != nil {
		return 0
	}
	return id
}

func portProtocol(port int, guess string) (protocol, name string) {
	name = guess
	protocol = "tcp"
	switch port {
	case 80, 3000, 8006, 8080, 8081, 9090, 32400:
		protocol = "http"
	case 443, 8443:
		protocol = "https"
	}
	if name == "" {
		name = fmt.Sprintf("port-%d", port)
	}
	return protocol, name
}
