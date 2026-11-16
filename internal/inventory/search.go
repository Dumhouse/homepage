package inventory

import (
	"context"
	"sort"
	"strings"
)

// SearchResult is a typed hit for ⌘K and for agents: what matched, and where to go.
type SearchResult struct {
	Kind     string `json:"kind"` // host | service | project | app_link | location
	ID       int64  `json:"id"`
	Title    string `json:"title"`
	Subtitle string `json:"subtitle,omitempty"`
	URL      string `json:"url,omitempty"`
}

type scored struct {
	res  SearchResult
	rank int
}

// Search matches every word of the query across names, hostnames, addresses,
// domains, project slugs, app-link labels and locations. Ranking: exact name,
// then prefix, then any word match. Max 20 results.
func (s *Store) Search(ctx context.Context, query string, limit int) ([]SearchResult, error) {
	q := strings.ToLower(strings.TrimSpace(query))
	if q == "" {
		return []SearchResult{}, nil
	}
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	toks := strings.Fields(q)
	var hits []scored
	add := func(r SearchResult, fields ...string) {
		if rank, ok := matchRank(q, toks, fields...); ok {
			hits = append(hits, scored{res: r, rank: rank})
		}
	}

	// host addresses, keyed by host, for matching and subtitles
	addresses := map[int64][]string{}
	rows, err := s.db.QueryContext(ctx, `SELECT host_id,address FROM host_addresses ORDER BY is_primary DESC,id`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var id int64
		var a string
		if err := rows.Scan(&id, &a); err != nil {
			rows.Close()
			return nil, err
		}
		addresses[id] = append(addresses[id], a)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	rows, err = s.db.QueryContext(ctx,
		`SELECT h.id,h.name,COALESCE(h.hostname,''),COALESCE(h.description,''),COALESCE(l.name,'')
		 FROM hosts h LEFT JOIN locations l ON l.id=h.location_id AND l.deleted_at IS NULL
		 WHERE h.deleted_at IS NULL`)
	if err != nil {
		return nil, err
	}
	type hostRow struct {
		id                             int64
		name, hostname, desc, location string
	}
	var hosts []hostRow
	for rows.Next() {
		var h hostRow
		if err := rows.Scan(&h.id, &h.name, &h.hostname, &h.desc, &h.location); err != nil {
			rows.Close()
			return nil, err
		}
		hosts = append(hosts, h)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for _, h := range hosts {
		fields := append([]string{h.name, h.hostname, h.desc, h.location}, addresses[h.id]...)
		sub := h.hostname
		if sub == "" && len(addresses[h.id]) > 0 {
			sub = addresses[h.id][0]
		}
		add(SearchResult{Kind: "host", ID: h.id, Title: h.name, Subtitle: sub, URL: "/hosts/" + itoa(h.id)}, fields...)
	}

	rows, err = s.db.QueryContext(ctx,
		`SELECT s.id,s.name,COALESCE(s.type,''),COALESCE(s.domain,''),COALESCE(s.port,0),h.id,h.name
		 FROM services s JOIN hosts h ON h.id=s.host_id AND h.deleted_at IS NULL
		 WHERE s.deleted_at IS NULL`)
	if err != nil {
		return nil, err
	}
	type svcRow struct {
		id, hostID              int64
		name, typ, domain, host string
		port                    int
	}
	var svcs []svcRow
	for rows.Next() {
		var v svcRow
		if err := rows.Scan(&v.id, &v.name, &v.typ, &v.domain, &v.port, &v.hostID, &v.host); err != nil {
			rows.Close()
			return nil, err
		}
		svcs = append(svcs, v)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for _, v := range svcs {
		sub := v.host
		if v.domain != "" {
			sub += " · " + v.domain
		} else if v.port > 0 {
			sub += " · :" + itoa(int64(v.port))
		}
		add(SearchResult{Kind: "service", ID: v.id, Title: v.name, Subtitle: sub, URL: "/hosts/" + itoa(v.hostID)},
			v.name, v.typ, v.domain, v.host)
	}

	rows, err = s.db.QueryContext(ctx,
		`SELECT p.id,p.name,p.slug,COALESCE(p.description,''),COALESCE(g.name,'')
		 FROM projects p LEFT JOIN groups g ON g.id=p.group_id AND g.deleted_at IS NULL
		 WHERE p.deleted_at IS NULL`)
	if err != nil {
		return nil, err
	}
	type projRow struct {
		id          int64
		name, slug  string
		desc, group string
	}
	var projs []projRow
	for rows.Next() {
		var p projRow
		if err := rows.Scan(&p.id, &p.name, &p.slug, &p.desc, &p.group); err != nil {
			rows.Close()
			return nil, err
		}
		projs = append(projs, p)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for _, p := range projs {
		sub := "project"
		if p.group != "" {
			sub = p.group
		}
		add(SearchResult{Kind: "project", ID: p.id, Title: p.name, Subtitle: sub, URL: "/projects/" + p.slug},
			p.name, p.slug, p.desc, p.group)
	}

	// environments, for resolving app-link URLs
	envs := map[int64][]ProjectEnv{}
	rows, err = s.db.QueryContext(ctx,
		`SELECT id,project_id,name,COALESCE(base_url,'') FROM project_envs
		 WHERE deleted_at IS NULL ORDER BY sort,id`)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var e ProjectEnv
		if err := rows.Scan(&e.ID, &e.ProjectID, &e.Name, &e.BaseURL); err != nil {
			rows.Close()
			return nil, err
		}
		envs[e.ProjectID] = append(envs[e.ProjectID], e)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	rows, err = s.db.QueryContext(ctx,
		`SELECT al.id,al.label,COALESCE(al.path,''),COALESCE(al.url,''),COALESCE(al.only_env,''),
		        p.id,p.name,p.slug
		 FROM app_links al JOIN projects p ON p.id=al.project_id AND p.deleted_at IS NULL
		 WHERE al.deleted_at IS NULL`)
	if err != nil {
		return nil, err
	}
	type linkRow struct {
		id, projectID                                  int64
		label, path, override, onlyEnv, projName, slug string
	}
	var links []linkRow
	for rows.Next() {
		var l linkRow
		if err := rows.Scan(&l.id, &l.label, &l.path, &l.override, &l.onlyEnv, &l.projectID, &l.projName, &l.slug); err != nil {
			rows.Close()
			return nil, err
		}
		links = append(links, l)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for _, l := range links {
		link := AppLink{Path: l.path, OverrideURL: l.override, OnlyEnv: l.onlyEnv}
		url := "/projects/" + l.slug
		for _, e := range envs[l.projectID] {
			if l.onlyEnv != "" && e.Name != l.onlyEnv {
				continue
			}
			if resolved := link.resolve(e.BaseURL); resolved != "" {
				url = resolved
				break
			}
		}
		add(SearchResult{Kind: "app_link", ID: l.id, Title: l.label, Subtitle: l.projName, URL: url},
			l.label, l.path, l.projName, l.slug)
	}

	rows, err = s.db.QueryContext(ctx,
		`SELECT id,name,COALESCE(notes,'') FROM locations WHERE deleted_at IS NULL`)
	if err != nil {
		return nil, err
	}
	type locRow struct {
		id         int64
		name, note string
	}
	var locs []locRow
	for rows.Next() {
		var l locRow
		if err := rows.Scan(&l.id, &l.name, &l.note); err != nil {
			rows.Close()
			return nil, err
		}
		locs = append(locs, l)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for _, l := range locs {
		add(SearchResult{Kind: "location", ID: l.id, Title: l.name, Subtitle: "location", URL: "/locations/" + itoa(l.id)},
			l.name, l.note)
	}

	sort.SliceStable(hits, func(i, j int) bool {
		if hits[i].rank != hits[j].rank {
			return hits[i].rank < hits[j].rank
		}
		return strings.ToLower(hits[i].res.Title) < strings.ToLower(hits[j].res.Title)
	})
	out := []SearchResult{}
	for _, h := range hits {
		if len(out) == limit {
			break
		}
		out = append(out, h.res)
	}
	return out, nil
}

// matchRank scores a row against the query and its words. Lower is better.
func matchRank(q string, toks []string, fields ...string) (int, bool) {
	hay := strings.ToLower(strings.Join(fields, "\x00"))
	for _, t := range toks {
		if !strings.Contains(hay, t) {
			return 0, false
		}
	}
	primary := ""
	if len(fields) > 0 {
		primary = strings.ToLower(strings.TrimSpace(fields[0]))
	}
	switch {
	case primary == q:
		return 0, true
	case strings.HasPrefix(primary, q):
		return 1, true
	default:
		return 2, true
	}
}
