package inventory

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// HostFacts is the read-only snapshot collected over SSH.
type HostFacts struct {
	HostID          int64  `json:"host_id"`
	CollectedAt     string `json:"collected_at"`
	OS              string `json:"os,omitempty"`
	Kernel          string `json:"kernel,omitempty"`
	Hostname        string `json:"hostname,omitempty"`
	Uptime          string `json:"uptime,omitempty"`
	DiskUsedPct     int    `json:"disk_used_pct"`
	FailedUnits     int    `json:"failed_units"`
	RebootRequired  string `json:"reboot_required,omitempty"` // yes | no
	PasswordAuth    string `json:"password_auth,omitempty"`   // yes | no | unknown
	RootLogin       string `json:"root_login,omitempty"`      // e.g. prohibit-password
	KeyFingerprints string `json:"key_fingerprints,omitempty"`
	Error           string `json:"error,omitempty"`
}

// SaveHostFacts upserts the snapshot and audits it.
func (s *Store) SaveHostFacts(ctx context.Context, actor string, f HostFacts) error {
	return s.tx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx,
			`INSERT INTO host_facts(host_id,collected_at,os,kernel,hostname,uptime,disk_used_pct,failed_units,
			  reboot_required,password_auth,root_login,key_fingerprints,error)
			 VALUES(?,datetime('now'),?,?,?,?,?,?,?,?,?,?,?)
			 ON CONFLICT(host_id) DO UPDATE SET
			  collected_at=datetime('now'), os=excluded.os, kernel=excluded.kernel, hostname=excluded.hostname,
			  uptime=excluded.uptime, disk_used_pct=excluded.disk_used_pct, failed_units=excluded.failed_units,
			  reboot_required=excluded.reboot_required, password_auth=excluded.password_auth,
			  root_login=excluded.root_login, key_fingerprints=excluded.key_fingerprints, error=excluded.error`,
			f.HostID, f.OS, f.Kernel, f.Hostname, f.Uptime, f.DiskUsedPct, f.FailedUnits,
			f.RebootRequired, f.PasswordAuth, f.RootLogin, f.KeyFingerprints, f.Error)
		if err != nil {
			return err
		}
		return s.audit(ctx, tx, actor, "facts", "host", f.HostID, f)
	})
}

func (s *Store) GetHostFacts(ctx context.Context, hostID int64) (*HostFacts, error) {
	f := &HostFacts{}
	err := s.db.QueryRowContext(ctx,
		`SELECT host_id,collected_at,COALESCE(os,''),COALESCE(kernel,''),COALESCE(hostname,''),COALESCE(uptime,''),
		        disk_used_pct,failed_units,COALESCE(reboot_required,''),COALESCE(password_auth,''),
		        COALESCE(root_login,''),COALESCE(key_fingerprints,''),COALESCE(error,'')
		 FROM host_facts WHERE host_id=?`, hostID).
		Scan(&f.HostID, &f.CollectedAt, &f.OS, &f.Kernel, &f.Hostname, &f.Uptime, &f.DiskUsedPct,
			&f.FailedUnits, &f.RebootRequired, &f.PasswordAuth, &f.RootLogin, &f.KeyFingerprints, &f.Error)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return f, err
}

// FactsAge is a helper for templates/API consumers.
func (f *HostFacts) Age() time.Duration {
	t, err := time.Parse("2006-01-02 15:04:05", f.CollectedAt)
	if err != nil {
		return 0
	}
	return time.Since(t)
}
