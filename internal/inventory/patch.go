package inventory

import (
	"context"
)

// HostPatch changes only the fields that are present. Unlike UpdateHost/PUT, an
// omitted field is left alone, so an agent can rename a host without sending
// (and accidentally dropping) its addresses. `location_id: 0` clears it.
type HostPatch struct {
	Name        *string        `json:"name,omitempty"`
	Hostname    *string        `json:"hostname,omitempty"`
	Kind        *string        `json:"kind,omitempty"`
	OS          *string        `json:"os,omitempty"`
	Description *string        `json:"description,omitempty"`
	Icon        *string        `json:"icon,omitempty"`
	Lifecycle   *string        `json:"lifecycle,omitempty"`
	LocationID  *int64         `json:"location_id,omitempty"`
	Tags        *[]string      `json:"tags,omitempty"`
	Addresses   *[]AddressOp   `json:"addresses,omitempty"`
	SSH         *[]SSHAccessOp `json:"ssh,omitempty"`
}

// AddressOp updates an existing address by id, adds a new one (id 0), or removes it.
type AddressOp struct {
	ID        int64  `json:"id,omitempty"`
	Kind      string `json:"kind,omitempty"`
	Address   string `json:"address,omitempty"`
	IsPrimary bool   `json:"is_primary,omitempty"`
	Remove    bool   `json:"remove,omitempty"`
}

// SSHAccessOp updates an existing login by id, adds a new one (id 0), or removes it.
type SSHAccessOp struct {
	ID     int64  `json:"id,omitempty"`
	User   string `json:"user,omitempty"`
	Port   int    `json:"port,omitempty"`
	Status string `json:"status,omitempty"`
	Vault  string `json:"vault_url,omitempty"`
	Remove bool   `json:"remove,omitempty"`
}

// PatchHost applies a partial update and returns the fresh host with warnings.
func (s *Store) PatchHost(ctx context.Context, actor string, id int64, p HostPatch) (*Host, error) {
	cur, err := s.GetHost(ctx, id)
	if err != nil {
		return nil, err
	}
	if p.Name != nil {
		cur.Name = *p.Name
	}
	if p.Hostname != nil {
		cur.Hostname = *p.Hostname
	}
	if p.Kind != nil {
		cur.Kind = *p.Kind
	}
	if p.OS != nil {
		cur.OS = *p.OS
	}
	if p.Description != nil {
		cur.Description = *p.Description
	}
	if p.Icon != nil {
		cur.Icon = *p.Icon
	}
	if p.Lifecycle != nil {
		cur.Lifecycle = *p.Lifecycle
	}
	if p.LocationID != nil {
		if *p.LocationID == 0 {
			cur.LocationID = nil
		} else {
			v := *p.LocationID
			cur.LocationID = &v
		}
	}
	if p.Tags != nil {
		cur.Tags = *p.Tags
	}
	if p.Addresses != nil {
		cur.Addresses = applyAddressOps(cur.Addresses, *p.Addresses)
	}
	if p.SSH != nil {
		cur.SSH = applySSHOps(cur.SSH, *p.SSH)
	}
	return s.UpdateHost(ctx, actor, id, *cur)
}

func applyAddressOps(cur []Address, ops []AddressOp) []Address {
	removed := map[int64]bool{}
	out := append([]Address{}, cur...)
	for _, op := range ops {
		if op.Remove {
			if op.ID > 0 {
				removed[op.ID] = true
			}
			continue
		}
		if op.ID == 0 {
			out = append(out, Address{Kind: op.Kind, Address: op.Address, IsPrimary: op.IsPrimary})
			continue
		}
		for i := range out {
			if out[i].ID != op.ID {
				continue
			}
			if op.Kind != "" {
				out[i].Kind = op.Kind
			}
			if op.Address != "" {
				out[i].Address = op.Address
			}
			out[i].IsPrimary = op.IsPrimary
		}
	}
	filtered := out[:0]
	for _, a := range out {
		if !removed[a.ID] {
			filtered = append(filtered, a)
		}
	}
	return filtered
}

func applySSHOps(cur []SSHAccess, ops []SSHAccessOp) []SSHAccess {
	removed := map[int64]bool{}
	out := append([]SSHAccess{}, cur...)
	for _, op := range ops {
		if op.Remove {
			if op.ID > 0 {
				removed[op.ID] = true
			}
			continue
		}
		if op.ID == 0 {
			out = append(out, SSHAccess{User: op.User, Port: op.Port, Status: op.Status, Vault: op.Vault})
			continue
		}
		for i := range out {
			if out[i].ID != op.ID {
				continue
			}
			if op.User != "" {
				out[i].User = op.User
			}
			if op.Port != 0 {
				out[i].Port = op.Port
			}
			if op.Status != "" {
				out[i].Status = op.Status
			}
			if op.Vault != "" {
				out[i].Vault = op.Vault
			}
		}
	}
	filtered := out[:0]
	for _, x := range out {
		if !removed[x.ID] {
			filtered = append(filtered, x)
		}
	}
	return filtered
}
