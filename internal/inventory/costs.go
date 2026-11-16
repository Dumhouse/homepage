package inventory

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

// Cost is a recurring or one-time expense. Domain renewal prices are derived
// from the domain itself and not duplicated here.
type Cost struct {
	ID           int64  `json:"id"`
	Label        string `json:"label"`
	TargetEntity string `json:"target_entity,omitempty"`
	TargetID     int64  `json:"target_id,omitempty"`
	ProjectID    *int64 `json:"project_id,omitempty"`
	AmountCents  int64  `json:"amount_cents"`
	Currency     string `json:"currency"`
	Cycle        string `json:"cycle"` // monthly | yearly | one_time | estimate
	NextRenewal  string `json:"next_renewal,omitempty"`
	AutoRenew    bool   `json:"auto_renew"`
	PaymentLabel string `json:"payment_label,omitempty"`
	Notes        string `json:"notes,omitempty"`
	Source       string `json:"source"`
	DaysLeft     int    `json:"days_left,omitempty"`
	// PaidAt is when it was paid (YYYY-MM-DD). ChargedCents/ChargedCurrency
	// lock its value in your own currency on that day; ChargedNote says where
	// the figure came from ("ECB rate of 2026-03-02" or "as charged").
	PaidAt          string `json:"paid_at,omitempty"`
	ChargedCents    int64  `json:"charged_cents,omitempty"`
	ChargedCurrency string `json:"charged_currency,omitempty"`
	ChargedNote     string `json:"charged_note,omitempty"`
}

func (c *Cost) normalize() error {
	c.Label = strings.TrimSpace(c.Label)
	if c.Label == "" {
		return invalid("label is required")
	}
	var err error
	if c.Cycle, err = oneOf(c.Cycle, "monthly", "monthly", "yearly", "one_time", "estimate"); err != nil {
		return err
	}
	if c.TargetEntity, err = oneOf(c.TargetEntity, "", "host", "service", "project", "domain", "provider", "other"); err != nil {
		return err
	}
	c.Currency = strings.ToUpper(strings.TrimSpace(c.Currency))
	if c.Currency == "" {
		c.Currency = "EUR"
	}
	if len(c.Currency) != 3 {
		return invalid("currency must be a 3-letter code")
	}
	if c.AmountCents < 0 {
		return invalid("amount cannot be negative")
	}
	if c.NextRenewal != "" {
		if _, err := time.Parse("2006-01-02", c.NextRenewal); err != nil {
			return invalid("next_renewal must be YYYY-MM-DD")
		}
		c.DaysLeft = int(time.Until(mustDate(c.NextRenewal)).Hours() / 24)
	}
	if c.Source == "" {
		c.Source = "manual"
	}
	if c.PaidAt != "" {
		if _, err := time.Parse("2006-01-02", c.PaidAt); err != nil {
			return invalid("paid_at must be YYYY-MM-DD")
		}
	}
	c.ChargedCurrency = strings.ToUpper(strings.TrimSpace(c.ChargedCurrency))
	if c.ChargedCents < 0 {
		return invalid("charged amount cannot be negative")
	}
	return nil
}

const costCols = `id,label,COALESCE(target_entity,''),target_id,project_id,amount_cents,currency,cycle,
	COALESCE(next_renewal,''),auto_renew,COALESCE(payment_label,''),COALESCE(notes,''),source,
	COALESCE(paid_at,''),charged_cents,charged_currency,charged_note`

func scanCost(sc interface{ Scan(...any) error }) (Cost, error) {
	var c Cost
	var project sql.NullInt64
	err := sc.Scan(&c.ID, &c.Label, &c.TargetEntity, &c.TargetID, &project, &c.AmountCents, &c.Currency,
		&c.Cycle, &c.NextRenewal, &c.AutoRenew, &c.PaymentLabel, &c.Notes, &c.Source,
		&c.PaidAt, &c.ChargedCents, &c.ChargedCurrency, &c.ChargedNote)
	if project.Valid {
		c.ProjectID = &project.Int64
	}
	if c.NextRenewal != "" {
		c.DaysLeft = int(time.Until(mustDate(c.NextRenewal)).Hours() / 24)
	}
	return c, err
}

func (s *Store) CreateCost(ctx context.Context, actor string, c Cost) (*Cost, error) {
	if err := c.normalize(); err != nil {
		return nil, err
	}
	if c.ProjectID != nil {
		if _, err := s.GetProject(ctx, *c.ProjectID); err != nil {
			return nil, invalid("project %d does not exist", *c.ProjectID)
		}
	}
	err := s.tx(ctx, func(tx *sql.Tx) error {
		r, err := tx.ExecContext(ctx,
			`INSERT INTO costs(label,target_entity,target_id,project_id,amount_cents,currency,cycle,
			 next_renewal,auto_renew,payment_label,notes,source,paid_at,charged_cents,charged_currency,charged_note)
			 VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			c.Label, c.TargetEntity, c.TargetID, c.ProjectID, c.AmountCents, c.Currency, c.Cycle,
			nullStr(c.NextRenewal), c.AutoRenew, c.PaymentLabel, c.Notes, c.Source,
			nullStr(c.PaidAt), c.ChargedCents, c.ChargedCurrency, c.ChargedNote)
		if err != nil {
			return err
		}
		c.ID, _ = r.LastInsertId()
		return s.audit(ctx, tx, actor, "create", "cost", c.ID, c)
	})
	if err != nil {
		return nil, err
	}
	return s.GetCost(ctx, c.ID)
}

func (s *Store) GetCost(ctx context.Context, id int64) (*Cost, error) {
	c, err := scanCost(s.db.QueryRowContext(ctx, `SELECT `+costCols+` FROM costs WHERE id=? AND deleted_at IS NULL`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &c, err
}

func (s *Store) ListCosts(ctx context.Context) ([]Cost, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+costCols+` FROM costs WHERE deleted_at IS NULL ORDER BY label COLLATE NOCASE`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Cost{}
	for rows.Next() {
		c, err := scanCost(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) UpdateCost(ctx context.Context, actor string, id int64, c Cost) (*Cost, error) {
	if _, err := s.GetCost(ctx, id); err != nil {
		return nil, err
	}
	if err := c.normalize(); err != nil {
		return nil, err
	}
	c.ID = id
	err := s.tx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx,
			`UPDATE costs SET label=?,target_entity=?,target_id=?,project_id=?,amount_cents=?,currency=?,cycle=?,
			 next_renewal=?,auto_renew=?,payment_label=?,notes=?,source=?,
			 paid_at=?,charged_cents=?,charged_currency=?,charged_note=? WHERE id=?`,
			c.Label, c.TargetEntity, c.TargetID, c.ProjectID, c.AmountCents, c.Currency, c.Cycle,
			nullStr(c.NextRenewal), c.AutoRenew, c.PaymentLabel, c.Notes, c.Source,
			nullStr(c.PaidAt), c.ChargedCents, c.ChargedCurrency, c.ChargedNote, id)
		if err != nil {
			return err
		}
		return s.audit(ctx, tx, actor, "update", "cost", id, c)
	})
	if err != nil {
		return nil, err
	}
	return s.GetCost(ctx, id)
}

func (s *Store) DeleteCost(ctx context.Context, actor string, id int64) error {
	return s.softDelete(ctx, actor, "costs", "cost", id, true)
}
func (s *Store) RestoreCost(ctx context.Context, actor string, id int64) error {
	return s.softDelete(ctx, actor, "costs", "cost", id, false)
}
