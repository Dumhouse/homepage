package integrations

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/dawidof/home-services/internal/alerts"
	"sort"
)

// Field describes one config input rendered into the settings form.
type Field struct {
	Key      string
	Label    string
	Help     string
	Kind     string // text | password | url | bool | int
	Required bool
	Secret   bool
	Default  string
}

type Config map[string]string

// Op is a proposed change from a sync. Nothing is written to the inventory by
// Sync itself; the AI changeset pipeline (M9) will review and apply these.
type Op struct {
	Action string            `json:"action"` // create | update | link | delete
	Entity string            `json:"entity"` // host | service | domain | project ...
	Ref    string            `json:"ref"`    // natural key, e.g. domain name
	Data   map[string]string `json:"data,omitempty"`
	Note   string            `json:"note,omitempty"`
}

// Plugin is one integration. Test proves the credentials/endpoint work; Sync
// returns proposed ops only.
type Plugin struct {
	ID          string
	Name        string
	Description string
	Fields      []Field
	Test        func(context.Context, Config) error
	Sync        func(context.Context, Config) ([]Op, error)
	// Kind is KindSync (reads data into the inventory) or KindNotify
	// (delivers alerts through Notify).
	Kind   string
	Notify func(context.Context, Config, alerts.Event) error
}

const (
	KindSync   = ""
	KindNotify = "notify"
)

type Manager struct {
	db      *sql.DB
	secrets SecretStore
	plugins map[string]*Plugin
	order   []string
}

func NewManager(db *sql.DB, secrets SecretStore) *Manager {
	return &Manager{db: db, secrets: secrets, plugins: map[string]*Plugin{}}
}

func (m *Manager) Register(p *Plugin) {
	if _, exists := m.plugins[p.ID]; !exists {
		m.order = append(m.order, p.ID)
	}
	m.plugins[p.ID] = p
}

func (m *Manager) Plugins() []*Plugin {
	out := make([]*Plugin, 0, len(m.order))
	for _, id := range m.order {
		out = append(out, m.plugins[id])
	}
	return out
}

// State is the safe view for the UI/API: non-secret config + secret presence.
type State struct {
	Plugin      *Plugin           `json:"-"`
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Fields      []Field           `json:"fields"`
	Config      map[string]string `json:"config"`
	HasSecret   map[string]bool   `json:"has_secret,omitempty"`
	Enabled     bool              `json:"enabled"`
	LastTestAt  string            `json:"last_test_at,omitempty"`
	LastError   string            `json:"last_error,omitempty"`
	LastSyncAt  string            `json:"last_sync_at,omitempty"`
	Saved       bool              `json:"saved"`
	Ops         []Op              `json:"ops,omitempty"`
}

type row struct {
	exists     bool
	config     string
	enabled    bool
	lastTestAt string
	lastError  string
	lastSyncAt string
	ops        string
}

func (m *Manager) load(ctx context.Context, id string) (row, error) {
	r := row{config: "{}", enabled: true}
	var enabled int
	err := m.db.QueryRowContext(ctx,
		`SELECT config,enabled,COALESCE(last_test_at,''),COALESCE(last_error,''),COALESCE(last_sync_at,''),COALESCE(last_ops,'')
		 FROM integrations WHERE plugin_id=?`, id).
		Scan(&r.config, &enabled, &r.lastTestAt, &r.lastError, &r.lastSyncAt, &r.ops)
	if errors.Is(err, sql.ErrNoRows) {
		return r, nil
	}
	if err != nil {
		return r, err
	}
	r.exists = true
	r.enabled = enabled == 1
	return r, nil
}

func (m *Manager) List(ctx context.Context) ([]State, error) {
	out := []State{}
	for _, id := range m.order {
		p := m.plugins[id]
		r, err := m.load(ctx, id)
		if err != nil {
			return nil, err
		}
		cfg := map[string]string{}
		_ = json.Unmarshal([]byte(r.config), &cfg)
		state := State{
			Plugin: p, ID: p.ID, Name: p.Name, Description: p.Description, Fields: p.Fields,
			Config: cfg, HasSecret: map[string]bool{}, Enabled: r.enabled, Saved: r.exists,
			LastTestAt: r.lastTestAt, LastError: r.lastError, LastSyncAt: r.lastSyncAt,
		}
		for _, f := range p.Fields {
			if f.Secret {
				v, _ := m.secrets.Get(ctx, secretKey(id, f.Key))
				state.HasSecret[f.Key] = v != ""
			} else if _, ok := cfg[f.Key]; !ok && f.Default != "" {
				state.Config[f.Key] = f.Default
			}
		}
		if r.ops != "" {
			_ = json.Unmarshal([]byte(r.ops), &state.Ops)
		}
		out = append(out, state)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func (m *Manager) plugin(id string) (*Plugin, error) {
	p, ok := m.plugins[id]
	if !ok {
		return nil, fmt.Errorf("%w: unknown integration %q", ErrInvalid, id)
	}
	return p, nil
}

var ErrInvalid = errors.New("invalid integration")

// Save stores non-secret fields in SQLite and secret fields in the SecretStore.
// A blank secret field means "keep the existing value".
func (m *Manager) Save(ctx context.Context, actor, id string, values map[string]string) error {
	p, err := m.plugin(id)
	if err != nil {
		return err
	}
	r, err := m.load(ctx, id)
	if err != nil {
		return err
	}
	cfg := map[string]string{}
	_ = json.Unmarshal([]byte(r.config), &cfg)
	for _, f := range p.Fields {
		v := values[f.Key]
		if f.Secret {
			if v != "" {
				if err := m.secrets.Set(ctx, secretKey(id, f.Key), v); err != nil {
					return err
				}
			}
			continue
		}
		if f.Required && v == "" {
			return fmt.Errorf("%w: %s is required", ErrInvalid, f.Label)
		}
		if f.Kind == "bool" { // keep "off" distinct from "never set" (which means Default)
			v = map[bool]string{true: "1", false: "0"}[v != "" && v != "0"]
		}
		cfg[f.Key] = v
	}
	b, _ := json.Marshal(cfg)
	return m.tx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO integrations(plugin_id,config) VALUES(?,?)
			 ON CONFLICT(plugin_id) DO UPDATE SET config=excluded.config`, id, string(b)); err != nil {
			return err
		}
		return audit(ctx, tx, actor, "update", "integration", id, map[string]any{"config": cfg})
	})
}

func (m *Manager) mergedConfig(ctx context.Context, id string) (Config, error) {
	p, err := m.plugin(id)
	if err != nil {
		return nil, err
	}
	r, err := m.load(ctx, id)
	if err != nil {
		return nil, err
	}
	cfg := Config{}
	_ = json.Unmarshal([]byte(r.config), &cfg)
	for _, f := range p.Fields {
		if f.Secret {
			cfg[f.Key], _ = m.secrets.Get(ctx, secretKey(id, f.Key))
		} else if cfg[f.Key] == "" {
			cfg[f.Key] = f.Default
		}
	}
	return cfg, nil
}

func (m *Manager) Test(ctx context.Context, id string) error {
	p, err := m.plugin(id)
	if err != nil {
		return err
	}
	cfg, err := m.mergedConfig(ctx, id)
	if err != nil {
		return err
	}
	var testErr error
	switch {
	case p.Test != nil:
		testErr = p.Test(ctx, cfg)
	case p.Notify != nil:
		testErr = p.Notify(ctx, cfg, testEvent())
	}
	msg := ""
	if testErr != nil {
		msg = testErr.Error()
	}
	if err := m.record(ctx, id, "last_test_at", msg); err != nil {
		return err
	}
	return testErr
}

func (m *Manager) Sync(ctx context.Context, id string) ([]Op, error) {
	p, err := m.plugin(id)
	if err != nil {
		return nil, err
	}
	if p.Sync == nil {
		return nil, fmt.Errorf("%w: %s has nothing to sync", ErrInvalid, p.Name)
	}
	cfg, err := m.mergedConfig(ctx, id)
	if err != nil {
		return nil, err
	}
	ops, syncErr := p.Sync(ctx, cfg)
	msg := ""
	if syncErr != nil {
		msg = syncErr.Error()
	}
	raw, _ := json.Marshal(ops)
	err = m.tx(ctx, func(tx *sql.Tx) error {
		_, err := tx.ExecContext(ctx,
			`INSERT INTO integrations(plugin_id,config,last_sync_at,last_ops,last_error)
			 VALUES(?, '{}', datetime('now'), ?, ?)
			 ON CONFLICT(plugin_id) DO UPDATE SET last_sync_at=datetime('now'), last_ops=excluded.last_ops, last_error=excluded.last_error`,
			id, string(raw), msg)
		return err
	})
	if err != nil {
		return nil, err
	}
	return ops, syncErr
}

func (m *Manager) record(ctx context.Context, id, column, errMsg string) error {
	q := `INSERT INTO integrations(plugin_id,config,` + column + `,last_error) VALUES(?, '{}', datetime('now'), ?)
	      ON CONFLICT(plugin_id) DO UPDATE SET ` + column + `=datetime('now'), last_error=excluded.last_error`
	_, err := m.db.ExecContext(ctx, q, id, errMsg)
	return err
}

func secretKey(plugin, field string) string { return plugin + "." + field }

func (m *Manager) tx(ctx context.Context, fn func(*sql.Tx) error) error {
	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		tx.Rollback()
		return err
	}
	return tx.Commit()
}

func audit(ctx context.Context, tx *sql.Tx, actor, action, entity string, id any, v any) error {
	b, _ := json.Marshal(v)
	_, err := tx.ExecContext(ctx,
		`INSERT INTO audit_log(actor,action,entity,entity_id,diff) VALUES(?,?,?,?,?)`,
		actor, action, entity, 0, string(b))
	return err
}

// Reset forgets an integration: its config, secrets, test/sync state.
func (m *Manager) Reset(ctx context.Context, actor, id string) error {
	p, err := m.plugin(id)
	if err != nil {
		return err
	}
	for _, f := range p.Fields {
		if f.Secret {
			_ = m.secrets.Delete(ctx, secretKey(id, f.Key))
		}
	}
	return m.tx(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `DELETE FROM integrations WHERE plugin_id=?`, id); err != nil {
			return err
		}
		return audit(ctx, tx, actor, "reset", "integration", id, nil)
	})
}

// Configured reports whether the integration has been saved at least once.
func (s State) Configured() bool {
	return s.Saved
}

// Working reports a configured integration whose last test or sync succeeded.
func (s State) Working() bool {
	return s.Configured() && s.LastError == "" && (s.LastTestAt != "" || s.LastSyncAt != "")
}
