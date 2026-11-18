package inventory

import (
	"context"
	"net/url"
	"strconv"
)

// GraphNode is one box on the dependency map.
type GraphNode struct {
	Key   string `json:"key"`  // "p:1", "s:4", "h:2", "l:3"
	Kind  string `json:"kind"` // project | service | host | location
	ID    int64  `json:"id"`
	Label string `json:"label"`
	Sub   string `json:"sub,omitempty"`
	URL   string `json:"url"`
}

// GraphEdge says From depends on To (a project uses a service, a service runs
// on a host, a host sits in a location).
type GraphEdge struct {
	From string `json:"from"`
	To   string `json:"to"`
	Role string `json:"role,omitempty"`
}

// Graph is the whole dependency map.
type Graph struct {
	Nodes []GraphNode `json:"nodes"`
	Edges []GraphEdge `json:"edges"`
}

func key(prefix string, id int64) string { return prefix + ":" + strconv.FormatInt(id, 10) }

// DependencyGraph builds the map of what depends on what. Decommissioned
// hosts and deleted things are left out.
func (s *Store) DependencyGraph(ctx context.Context) (*Graph, error) {
	g := &Graph{Nodes: []GraphNode{}, Edges: []GraphEdge{}}
	have := map[string]bool{}
	add := func(n GraphNode) {
		if !have[n.Key] {
			have[n.Key] = true
			g.Nodes = append(g.Nodes, n)
		}
	}
	seen := map[[2]string]bool{}
	edge := func(from, to, role string) {
		if !have[from] || !have[to] || from == to || seen[[2]string{from, to}] {
			return
		}
		seen[[2]string{from, to}] = true
		g.Edges = append(g.Edges, GraphEdge{From: from, To: to, Role: role})
	}
	type nodeQuery struct {
		q    string
		make func(id int64, a, b string) GraphNode
	}
	queries := []nodeQuery{
		{`SELECT id,name,COALESCE(kind,'') FROM locations WHERE deleted_at IS NULL ORDER BY name COLLATE NOCASE`,
			func(id int64, a, b string) GraphNode {
				return GraphNode{Key: key("l", id), Kind: "location", ID: id, Label: a, Sub: b, URL: "/hosts?loc=" + url.QueryEscape(a)}
			}},
		{`SELECT id,name,kind FROM hosts WHERE deleted_at IS NULL AND lifecycle<>'decommissioned' ORDER BY name COLLATE NOCASE`,
			func(id int64, a, b string) GraphNode {
				return GraphNode{Key: key("h", id), Kind: "host", ID: id, Label: a, Sub: b, URL: "/hosts/" + strconv.FormatInt(id, 10)}
			}},
		{`SELECT s.id,s.name,COALESCE(NULLIF(s.domain,''),CASE WHEN s.port>0 THEN ':'||s.port ELSE '' END)
		  FROM services s JOIN hosts h ON h.id=s.host_id AND h.deleted_at IS NULL AND h.lifecycle<>'decommissioned'
		  WHERE s.deleted_at IS NULL ORDER BY s.name COLLATE NOCASE`,
			func(id int64, a, b string) GraphNode {
				return GraphNode{Key: key("s", id), Kind: "service", ID: id, Label: a, Sub: b, URL: "/services/" + strconv.FormatInt(id, 10) + "/edit"}
			}},
		{`SELECT id,name,slug FROM projects WHERE deleted_at IS NULL ORDER BY name COLLATE NOCASE`,
			func(id int64, a, b string) GraphNode {
				return GraphNode{Key: key("p", id), Kind: "project", ID: id, Label: a, URL: "/projects/" + b}
			}},
	}
	for _, nq := range queries {
		rows, err := s.db.QueryContext(ctx, nq.q)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var id int64
			var a, b string
			if err := rows.Scan(&id, &a, &b); err != nil {
				rows.Close()
				return nil, err
			}
			add(nq.make(id, a, b))
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}
	}
	type edgeQuery struct{ q, from, to string }
	edges := []edgeQuery{
		{`SELECT id,location_id,'' FROM hosts WHERE location_id IS NOT NULL`, "h", "l"},
		{`SELECT id,host_id,'' FROM services WHERE deleted_at IS NULL`, "s", "h"},
		{`SELECT project_id,service_id,'' FROM project_services`, "p", "s"},
		{`SELECT project_id,host_id,'' FROM project_hosts`, "p", "h"},
		{`SELECT project_id,target_id,role FROM project_connections WHERE deleted_at IS NULL AND target_kind='service' AND target_id IS NOT NULL`, "p", "s"},
		{`SELECT project_id,target_id,role FROM project_connections WHERE deleted_at IS NULL AND target_kind='host' AND target_id IS NOT NULL`, "p", "h"},
	}
	for _, eq := range edges {
		rows, err := s.db.QueryContext(ctx, eq.q)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var a, b int64
			var role string
			if err := rows.Scan(&a, &b, &role); err != nil {
				rows.Close()
				return nil, err
			}
			edge(key(eq.from, a), key(eq.to, b), role)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return nil, err
		}
	}
	return g, nil
}

// Impact returns every node that depends on key, directly or not: what
// breaks if it goes down.
func (g *Graph) Impact(k string) []string {
	up := map[string][]string{}
	for _, e := range g.Edges {
		up[e.To] = append(up[e.To], e.From)
	}
	out, seen, queue := []string{}, map[string]bool{k: true}, []string{k}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for _, n := range up[cur] {
			if !seen[n] {
				seen[n] = true
				out = append(out, n)
				queue = append(queue, n)
			}
		}
	}
	return out
}
