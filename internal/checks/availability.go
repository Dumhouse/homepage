package checks

import (
	"context"
	"strconv"
	"time"
)

// Bar is one thin availability slot: Class is ok | warn | bad | empty.
type Bar struct {
	Class string  `json:"class"`
	Ratio float64 `json:"ratio"`
}

// Availability buckets the last `hours` of raw results for a check into
// `buckets` slots (oldest first). Empty slots mean no data for that period.
func (s *Store) Availability(ctx context.Context, checkID int64, buckets, hours int) ([]Bar, error) {
	if buckets <= 0 {
		buckets = 12
	}
	if hours <= 0 {
		hours = 24
	}
	out := make([]Bar, buckets)
	for i := range out {
		out[i].Class = "empty"
	}
	if checkID == 0 {
		return out, nil
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT at,ok FROM check_results WHERE check_id=? AND at >= datetime('now', ?) ORDER BY at`,
		checkID, "-"+strconv.Itoa(hours)+" hours")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	now := time.Now().UTC()
	width := time.Duration(hours) * time.Hour / time.Duration(buckets)
	okCount := make([]int, buckets)
	total := make([]int, buckets)
	for rows.Next() {
		var at string
		var ok int
		if err := rows.Scan(&at, &ok); err != nil {
			return nil, err
		}
		t, err := time.Parse("2006-01-02 15:04:05", at)
		if err != nil {
			continue
		}
		age := now.Sub(t)
		if age < 0 || age >= time.Duration(hours)*time.Hour {
			continue
		}
		idx := buckets - 1 - int(age/width)
		if idx < 0 || idx >= buckets {
			continue
		}
		total[idx]++
		if ok == 1 {
			okCount[idx]++
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range out {
		if total[i] == 0 {
			continue
		}
		out[i].Ratio = float64(okCount[i]) / float64(total[i])
		switch {
		case okCount[i] == total[i]:
			out[i].Class = "ok"
		case out[i].Ratio >= 0.8:
			out[i].Class = "warn"
		default:
			out[i].Class = "bad"
		}
	}
	return out, nil
}

func (s *Store) AvailabilityFor(ctx context.Context, entity string, id int64) ([]Bar, error) {
	return s.AvailabilityN(ctx, entity, id, 12)
}

// AvailabilityN merges every enabled check of a target (LAN and public)
// into n slots over 24 h; each slot shows the worst result in it. Leading
// slots from before the first check ran are dropped, so a new check shows a
// short full-width bar instead of a row of grey gaps.
func (s *Store) AvailabilityN(ctx context.Context, entity string, id int64, n int) ([]Bar, error) {
	list, err := s.List(ctx, entity, id)
	if err != nil || len(list) == 0 {
		return nil, err
	}
	rank := map[string]int{"empty": 0, "ok": 1, "warn": 2, "bad": 3}
	var out []Bar
	for _, c := range list {
		if !c.Enabled {
			continue
		}
		bars, err := s.Availability(ctx, c.ID, n, 24)
		if err != nil {
			return nil, err
		}
		if out == nil {
			out = bars
			continue
		}
		for i := range bars {
			if rank[bars[i].Class] > rank[out[i].Class] {
				out[i] = bars[i]
			}
		}
	}
	for len(out) > 0 && out[0].Class == "empty" {
		out = out[1:]
	}
	return out, nil
}
