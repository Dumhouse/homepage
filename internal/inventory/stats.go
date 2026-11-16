package inventory

import "context"

// Stats are the counts the dashboard and sidebar show.
type Stats struct {
	Hosts     int `json:"hosts"`
	Services  int `json:"services"`
	Projects  int `json:"projects"`
	Locations int `json:"locations"`
	Domains   int `json:"domains"`
	Expiring  int `json:"expiring_domains"` // within 30 days
}

func (s *Store) Stats(ctx context.Context) (Stats, error) {
	var st Stats
	counts := []struct {
		q   string
		ptr *int
	}{
		{`SELECT COUNT(*) FROM hosts WHERE deleted_at IS NULL`, &st.Hosts},
		{`SELECT COUNT(*) FROM services WHERE deleted_at IS NULL`, &st.Services},
		{`SELECT COUNT(*) FROM projects WHERE deleted_at IS NULL`, &st.Projects},
		{`SELECT COUNT(*) FROM locations WHERE deleted_at IS NULL`, &st.Locations},
		{`SELECT COUNT(*) FROM domains WHERE deleted_at IS NULL`, &st.Domains},
		{`SELECT COUNT(*) FROM domains WHERE deleted_at IS NULL AND expires_at IS NOT NULL
		  AND date(expires_at) <= date('now','+30 days') AND date(expires_at) >= date('now')`, &st.Expiring},
	}
	for _, c := range counts {
		if err := s.db.QueryRowContext(ctx, c.q).Scan(c.ptr); err != nil {
			return st, err
		}
	}
	return st, nil
}
