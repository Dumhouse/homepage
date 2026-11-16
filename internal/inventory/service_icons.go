package inventory

import "context"

// ServiceIconStats returns how many live services exist and how many already
// have a cached favicon.
func (s *Store) ServiceIconStats(ctx context.Context) (total, withIcon int, err error) {
	err = s.db.QueryRowContext(ctx,
		`SELECT COUNT(*), COALESCE(SUM(CASE WHEN COALESCE(icon,'')<>'' THEN 1 ELSE 0 END),0)
		 FROM services WHERE deleted_at IS NULL`).Scan(&total, &withIcon)
	return total, withIcon, err
}

// ServicesWithDomain lists live services that have a domain, for icon refresh.
func (s *Store) ServicesWithDomain(ctx context.Context) ([]Service, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT s.id,s.host_id,s.name,COALESCE(s.type,''),COALESCE(s.port,0),s.protocol,
		        COALESCE(s.path,''),s.domain,COALESCE(s.version,''),COALESCE(s.notes,''),COALESCE(s.icon,'')
		 FROM services s JOIN hosts h ON h.id=s.host_id AND h.deleted_at IS NULL
		 WHERE s.deleted_at IS NULL AND COALESCE(s.domain,'')<>''
		 ORDER BY s.name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Service{}
	for rows.Next() {
		var v Service
		if err := rows.Scan(&v.ID, &v.HostID, &v.Name, &v.Type, &v.Port, &v.Protocol, &v.Path, &v.Domain,
			&v.Version, &v.Notes, &v.Icon); err != nil {
			return nil, err
		}
		v.Tags = []string{}
		out = append(out, v)
	}
	return out, rows.Err()
}
