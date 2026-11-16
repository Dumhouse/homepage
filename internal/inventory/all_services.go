package inventory

import "context"

// ServiceWithHost is a service plus the host/location it runs on, for the
// cross-host services page.
type ServiceWithHost struct {
	Service
	HostName string `json:"host_name"`
	Address  string `json:"address,omitempty"`
	Location string `json:"location,omitempty"`
}

// AllServices lists every live service with its host and primary address.
func (s *Store) AllServices(ctx context.Context) ([]ServiceWithHost, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT s.id,s.host_id,s.name,COALESCE(s.type,''),COALESCE(s.port,0),s.protocol,
		        COALESCE(s.path,''),COALESCE(s.domain,''),COALESCE(s.version,''),COALESCE(s.notes,''),COALESCE(s.icon,''),
		        h.name, COALESCE((SELECT a.address FROM host_addresses a WHERE a.host_id=h.id ORDER BY a.is_primary DESC,a.id LIMIT 1),''),
		        COALESCE(l.name,'')
		 FROM services s
		 JOIN hosts h ON h.id=s.host_id AND h.deleted_at IS NULL
		 LEFT JOIN locations l ON l.id=h.location_id AND l.deleted_at IS NULL
		 WHERE s.deleted_at IS NULL
		 ORDER BY h.name COLLATE NOCASE, COALESCE(s.port,0), s.name COLLATE NOCASE`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ServiceWithHost{}
	for rows.Next() {
		var v ServiceWithHost
		if err := rows.Scan(&v.ID, &v.HostID, &v.Name, &v.Type, &v.Port, &v.Protocol, &v.Path, &v.Domain,
			&v.Version, &v.Notes, &v.Icon, &v.HostName, &v.Address, &v.Location); err != nil {
			return nil, err
		}
		v.Tags = []string{}
		out = append(out, v)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()
	for i := range out {
		if out[i].Tags, err = s.tagsFor(ctx, "service", out[i].ID); err != nil {
			return nil, err
		}
	}
	return out, nil
}
