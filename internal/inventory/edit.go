package inventory

import (
	"context"
	"database/sql"
	"errors"
)

// GetAccess returns one login entry.
func (s *Store) GetAccess(ctx context.Context, id int64) (*Access, error) {
	a := &Access{}
	err := s.db.QueryRowContext(ctx,
		`SELECT id,target_entity,target_id,label,COALESCE(url,''),method,COALESCE(account,''),
		        COALESCE(twofa,'none'),COALESCE(vault_item_url,''),COALESCE(notes,'')
		 FROM access WHERE id=? AND deleted_at IS NULL`, id).
		Scan(&a.ID, &a.TargetEntity, &a.TargetID, &a.Label, &a.URL, &a.Method, &a.Account,
			&a.TwoFA, &a.VaultItemURL, &a.Notes)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return a, err
}

// UpdateAccess edits the fields of an existing login. Target cannot change.
func (s *Store) UpdateAccess(ctx context.Context, actor string, id int64, a Access) (*Access, error) {
	cur, err := s.GetAccess(ctx, id)
	if err != nil {
		return nil, err
	}
	a.ID, a.TargetEntity, a.TargetID = id, cur.TargetEntity, cur.TargetID
	if err := a.normalize(); err != nil {
		return nil, err
	}
	err = s.tx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx,
			`UPDATE access SET label=?,url=?,method=?,account=?,twofa=?,vault_item_url=?,notes=?
			 WHERE id=? AND deleted_at IS NULL`,
			a.Label, a.URL, a.Method, a.Account, a.TwoFA, a.VaultItemURL, a.Notes, id)
		if err != nil {
			return err
		}
		return s.audit(ctx, tx, actor, "update", "access", id, a)
	})
	if err != nil {
		return nil, err
	}
	return s.GetAccess(ctx, id)
}

// GetAppLink returns one app link.
func (s *Store) GetAppLink(ctx context.Context, id int64) (*AppLink, error) {
	l := &AppLink{}
	var access sql.NullInt64
	err := s.db.QueryRowContext(ctx,
		`SELECT id,project_id,label,COALESCE(path,''),COALESCE(url,''),COALESCE(only_env,''),kind,
		        COALESCE(icon,''),access_id,health_check,sort
		 FROM app_links WHERE id=? AND deleted_at IS NULL`, id).
		Scan(&l.ID, &l.ProjectID, &l.Label, &l.Path, &l.OverrideURL, &l.OnlyEnv, &l.Kind,
			&l.Icon, &access, &l.HealthCheck, &l.Sort)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if access.Valid {
		l.AccessID = &access.Int64
	}
	return l, nil
}

// UpdateAppLink edits an existing app link (project stays the same).
func (s *Store) UpdateAppLink(ctx context.Context, actor string, id int64, l AppLink) (*AppLink, error) {
	cur, err := s.GetAppLink(ctx, id)
	if err != nil {
		return nil, err
	}
	l.ID, l.ProjectID = id, cur.ProjectID
	if err := l.normalize(); err != nil {
		return nil, err
	}
	if l.AccessID != nil {
		if err := s.checkAccess(ctx, l.AccessID); err != nil {
			return nil, err
		}
	}
	err = s.tx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx,
			`UPDATE app_links SET label=?,path=?,url=?,only_env=?,kind=?,icon=?,access_id=?,health_check=?,sort=?
			 WHERE id=? AND deleted_at IS NULL`,
			l.Label, l.Path, l.OverrideURL, l.OnlyEnv, l.Kind, l.Icon, l.AccessID, l.HealthCheck, l.Sort, id)
		if err != nil {
			return err
		}
		return s.audit(ctx, tx, actor, "update", "app_link", id, l)
	})
	if err != nil {
		return nil, err
	}
	return s.GetAppLink(ctx, id)
}
