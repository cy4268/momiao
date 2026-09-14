package platform

import (
 "context"
 "time"
)

// PublicMaintenanceNotice deliberately omits operator reasons, audit payloads,
// account counts and internal health failures. Its lifetime follows the window.
type PublicMaintenanceNotice struct {
 ID string `json:"id"`
 Scopes []string `json:"scopes"`
 ScheduledEnd *time.Time `json:"scheduled_end_at"`
 EstimatedEnd *time.Time `json:"estimated_end_at"`
 AnnouncementID *string `json:"announcement_id"`
}
func(s *Store) ReadPublicMaintenanceNotices(ctx context.Context,environment string)([]PublicMaintenanceNotice,error){
 rows,err:=s.pool.Query(ctx,`SELECT n.critical_notice_id::text,
 ARRAY(SELECT scope FROM ops.maintenance_window_scopes WHERE maintenance_id=w.maintenance_id ORDER BY scope),
 w.scheduled_end_at,w.estimated_end_at,
 (SELECT a.announcement_id::text FROM content.announcements a JOIN content.announcement_revisions r ON r.announcement_id=a.announcement_id AND r.content_version=a.current_content_version
  WHERE a.announcement_id=w.announcement_id AND a.state='PUBLISHED' AND r.visibility='PUBLIC'
   AND a.visible_from<=statement_timestamp() AND (a.visible_until IS NULL OR a.visible_until>statement_timestamp()))
 FROM ops.critical_notice_state n JOIN ops.maintenance_windows w USING(maintenance_id)
 WHERE w.state='ACTIVE' AND w.environment=$1 ORDER BY w.activated_at,n.critical_notice_id LIMIT 8`,environment)
 if err!=nil{return nil,err};defer rows.Close();result:=[]PublicMaintenanceNotice{}
 for rows.Next(){var item PublicMaintenanceNotice;if err=rows.Scan(&item.ID,&item.Scopes,&item.ScheduledEnd,&item.EstimatedEnd,&item.AnnouncementID);err!=nil{return nil,err};result=append(result,item)}
 return result,rows.Err()
}
