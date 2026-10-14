package platform

import "context"

type PublicMaintenanceNotice struct {
	ID      string   `json:"id"`
	Scopes  []string `json:"scopes"`
	Message string   `json:"message"`
}

func (s *Store) ReadPublicMaintenanceNotices(ctx context.Context, environment string) ([]PublicMaintenanceNotice, error) {
	rows, err := s.pool.Query(ctx, `SELECT w.maintenance_id::text,w.message,
 ARRAY(SELECT scope FROM ops.maintenance_window_scopes WHERE maintenance_id=w.maintenance_id ORDER BY scope)
 FROM ops.maintenance_windows w WHERE w.state='ACTIVE' AND w.environment=$1 ORDER BY w.activated_at,w.maintenance_id`, environment)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []PublicMaintenanceNotice{}
	for rows.Next() {
		var item PublicMaintenanceNotice
		if err = rows.Scan(&item.ID, &item.Message, &item.Scopes); err != nil {
			return nil, err
		}
		result = append(result, item)
	}
	return result, rows.Err()
}
