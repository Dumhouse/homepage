package inventory

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

// Domain is a registered name with its registrar/expiry/cost metadata.
// DNS records live beside it; matched_host_id links an A/CNAME to inventory.
type Domain struct {
	ID           int64          `json:"id"`
	Name         string         `json:"name"`
	Registrar    string         `json:"registrar,omitempty"`
	Account      string         `json:"account,omitempty"`
	ExpiresAt    string         `json:"expires_at,omitempty"` // YYYY-MM-DD
	AutoRenew    bool           `json:"auto_renew"`
	PriceCents   int64          `json:"price_cents"`
	Currency     string         `json:"currency"`
	PaymentLabel string         `json:"payment_label,omitempty"`
	Nameservers  string         `json:"nameservers,omitempty"`
	DNSSEC       bool           `json:"dnssec"`
	TransferLock bool           `json:"transfer_lock"`
	Notes        string         `json:"notes,omitempty"`
	Source       string         `json:"source"`
	Records      []DomainRecord `json:"records,omitempty"`
	DaysLeft     int            `json:"days_left,omitempty"`
	// Paid-day value, like costs: ChargedCents is in ChargedCurrency (usually
	// the default currency) and ChargedNote says where the figure came from.
	PaidAt          string `json:"paid_at,omitempty"`
	ChargedCents    int64  `json:"charged_cents,omitempty"`
	ChargedCurrency string `json:"charged_currency,omitempty"`
	ChargedNote     string `json:"charged_note,omitempty"`
}

// DomainRecord is one DNS record. Records are never applied by Home Services.
type DomainRecord struct {
	ID          int64  `json:"id"`
	DomainID    int64  `json:"domain_id"`
	Name        string `json:"name"`
	Type        string `json:"type"`
	Content     string `json:"content,omitempty"`
	Proxied     bool   `json:"proxied"`
	MatchedHost *int64 `json:"matched_host_id,omitempty"`
	Stale       bool   `json:"stale"`
}

func (d *Domain) normalize() error {
	d.Name = strings.ToLower(strings.TrimSpace(d.Name))
	d.Name = strings.TrimPrefix(d.Name, "http://")
	d.Name = strings.TrimPrefix(d.Name, "https://")
	d.Name = strings.TrimSuffix(d.Name, "/")
	if d.Name == "" {
		return invalid("name is required")
	}
	if strings.ContainsAny(d.Name, " /:@") || !strings.Contains(d.Name, ".") {
		return invalid("name must be a plain domain like example.com")
	}
	var err error
	if d.Source, err = oneOf(d.Source, "manual", "manual", "cloudflare", "ovh"); err != nil {
		return err
	}
	d.Currency = strings.ToUpper(strings.TrimSpace(d.Currency))
	if d.Currency == "" {
		d.Currency = "EUR"
	}
	if len(d.Currency) != 3 {
		return invalid("currency must be a 3-letter code")
	}
	if d.PriceCents < 0 {
		return invalid("price cannot be negative")
	}
	if d.ExpiresAt != "" {
		if _, err := time.Parse("2006-01-02", d.ExpiresAt); err != nil {
			return invalid("expires_at must be YYYY-MM-DD")
		}
		d.DaysLeft = int(time.Until(mustDate(d.ExpiresAt)).Hours() / 24)
	}
	if d.PaidAt != "" {
		if _, err := time.Parse("2006-01-02", d.PaidAt); err != nil {
			return invalid("paid_at must be YYYY-MM-DD")
		}
	}
	d.ChargedCurrency = strings.ToUpper(strings.TrimSpace(d.ChargedCurrency))
	if d.ChargedCents < 0 {
		return invalid("charged amount cannot be negative")
	}
	d.Nameservers = strings.TrimSpace(d.Nameservers)
	return nil
}

func mustDate(s string) time.Time {
	t, _ := time.Parse("2006-01-02", s)
	return t
}

func (s *Store) CreateDomain(ctx context.Context, actor string, d Domain) (*Domain, error) {
	if err := d.normalize(); err != nil {
		return nil, err
	}
	err := s.tx(ctx, func(tx *sql.Tx) error {
		r, err := tx.ExecContext(ctx,
			`INSERT INTO domains(name,registrar,account,expires_at,auto_renew,price_cents,currency,payment_label,
			 nameservers,dnssec,transfer_lock,notes,source,paid_at,charged_cents,charged_currency,charged_note)
			 VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			d.Name, d.Registrar, d.Account, nullStr(d.ExpiresAt), d.AutoRenew, d.PriceCents, d.Currency,
			d.PaymentLabel, d.Nameservers, d.DNSSEC, d.TransferLock, d.Notes, d.Source,
			nullStr(d.PaidAt), d.ChargedCents, d.ChargedCurrency, d.ChargedNote)
		if err != nil {
			if isUniqueErr(err) {
				return invalid("domain %q already exists", d.Name)
			}
			return err
		}
		d.ID, _ = r.LastInsertId()
		return s.audit(ctx, tx, actor, "create", "domain", d.ID, d)
	})
	if err != nil {
		return nil, err
	}
	return s.GetDomain(ctx, d.ID)
}

func nullStr(v string) any {
	if v == "" {
		return nil
	}
	return v
}

const domainCols = `id,name,COALESCE(registrar,''),COALESCE(account,''),COALESCE(expires_at,''),auto_renew,
	price_cents,currency,COALESCE(payment_label,''),COALESCE(nameservers,''),dnssec,transfer_lock,
	COALESCE(notes,''),source,COALESCE(paid_at,''),charged_cents,charged_currency,charged_note`

func scanDomain(sc interface{ Scan(...any) error }) (Domain, error) {
	var d Domain
	err := sc.Scan(&d.ID, &d.Name, &d.Registrar, &d.Account, &d.ExpiresAt, &d.AutoRenew, &d.PriceCents,
		&d.Currency, &d.PaymentLabel, &d.Nameservers, &d.DNSSEC, &d.TransferLock, &d.Notes, &d.Source,
		&d.PaidAt, &d.ChargedCents, &d.ChargedCurrency, &d.ChargedNote)
	if d.ExpiresAt != "" {
		d.DaysLeft = int(time.Until(mustDate(d.ExpiresAt)).Hours() / 24)
	}
	return d, err
}

func (s *Store) GetDomain(ctx context.Context, id int64) (*Domain, error) {
	d, err := scanDomain(s.db.QueryRowContext(ctx, `SELECT `+domainCols+` FROM domains WHERE id=? AND deleted_at IS NULL`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if d.Records, err = s.domainRecords(ctx, id); err != nil {
		return nil, err
	}
	return &d, nil
}

func (s *Store) ListDomains(ctx context.Context) ([]Domain, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+domainCols+` FROM domains WHERE deleted_at IS NULL ORDER BY name COLLATE NOCASE`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Domain{}
	for rows.Next() {
		d, err := scanDomain(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (s *Store) UpdateDomain(ctx context.Context, actor string, id int64, d Domain) (*Domain, error) {
	if _, err := s.GetDomain(ctx, id); err != nil {
		return nil, err
	}
	if err := d.normalize(); err != nil {
		return nil, err
	}
	d.ID = id
	err := s.tx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx,
			`UPDATE domains SET name=?,registrar=?,account=?,expires_at=?,auto_renew=?,price_cents=?,currency=?,
			 payment_label=?,nameservers=?,dnssec=?,transfer_lock=?,notes=?,source=?,
			 paid_at=?,charged_cents=?,charged_currency=?,charged_note=? WHERE id=?`,
			d.Name, d.Registrar, d.Account, nullStr(d.ExpiresAt), d.AutoRenew, d.PriceCents, d.Currency,
			d.PaymentLabel, d.Nameservers, d.DNSSEC, d.TransferLock, d.Notes, d.Source,
			nullStr(d.PaidAt), d.ChargedCents, d.ChargedCurrency, d.ChargedNote, id)
		if err != nil {
			if isUniqueErr(err) {
				return invalid("domain %q already exists", d.Name)
			}
			return err
		}
		return s.audit(ctx, tx, actor, "update", "domain", id, d)
	})
	if err != nil {
		return nil, err
	}
	return s.GetDomain(ctx, id)
}

func (s *Store) DeleteDomain(ctx context.Context, actor string, id int64) error {
	return s.softDelete(ctx, actor, "domains", "domain", id, true)
}
func (s *Store) RestoreDomain(ctx context.Context, actor string, id int64) error {
	return s.softDelete(ctx, actor, "domains", "domain", id, false)
}

func (s *Store) domainRecords(ctx context.Context, domainID int64) ([]DomainRecord, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id,domain_id,name,type,COALESCE(content,''),proxied,matched_host_id,stale
		 FROM dns_records WHERE domain_id=? AND deleted_at IS NULL ORDER BY name,type`, domainID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []DomainRecord{}
	for rows.Next() {
		var r DomainRecord
		var host sql.NullInt64
		if err := rows.Scan(&r.ID, &r.DomainID, &r.Name, &r.Type, &r.Content, &r.Proxied, &host, &r.Stale); err != nil {
			return nil, err
		}
		if host.Valid {
			r.MatchedHost = &host.Int64
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// AddDomainRecord stores one DNS record (manual entry for now).
func (s *Store) AddDomainRecord(ctx context.Context, actor string, domainID int64, r DomainRecord) (*DomainRecord, error) {
	if _, err := s.GetDomain(ctx, domainID); err != nil {
		return nil, err
	}
	r.Name = strings.TrimSpace(r.Name)
	if r.Name == "" {
		return nil, invalid("record name is required")
	}
	r.Type = strings.ToUpper(strings.TrimSpace(r.Type))
	switch r.Type {
	case "A", "AAAA", "CNAME", "TXT", "MX", "NS", "SRV", "CAA":
	default:
		return nil, invalid("unsupported record type %q", r.Type)
	}
	if r.MatchedHost != nil {
		if _, err := s.GetHost(ctx, *r.MatchedHost); err != nil {
			return nil, invalid("matched host %d does not exist", *r.MatchedHost)
		}
	}
	r.DomainID = domainID
	err := s.tx(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx,
			`INSERT INTO dns_records(domain_id,name,type,content,proxied,matched_host_id,stale)
			 VALUES(?,?,?,?,?,?,?)`,
			domainID, r.Name, r.Type, r.Content, r.Proxied, r.MatchedHost, r.Stale)
		if err != nil {
			return err
		}
		r.ID, _ = res.LastInsertId()
		return s.audit(ctx, tx, actor, "create", "dns_record", r.ID, r)
	})
	if err != nil {
		return nil, err
	}
	return &r, nil
}

func (s *Store) DeleteDomainRecord(ctx context.Context, actor string, id int64) error {
	return s.softDelete(ctx, actor, "dns_records", "dns_record", id, true)
}

// ExpiringDomains returns live domains expiring within days, soonest first.
func (s *Store) ExpiringDomains(ctx context.Context, days int) ([]Domain, error) {
	all, err := s.ListDomains(ctx)
	if err != nil {
		return nil, err
	}
	out := []Domain{}
	for _, d := range all {
		if d.ExpiresAt == "" || d.DaysLeft < 0 || d.DaysLeft > days {
			continue
		}
		out = append(out, d)
	}
	return out, nil
}
