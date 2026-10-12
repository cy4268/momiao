package platform

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"
)

type OpsAuditEvent struct {
	AuditID        string          `json:"audit_id"`
	ActorUserID    int64           `json:"actor_newapi_user_id,string"`
	ActorName      string          `json:"actor_name"`
	Action         string          `json:"action"`
	TargetType     string          `json:"target_type"`
	TargetID       string          `json:"target_id"`
	BeforeSnapshot json.RawMessage `json:"before_snapshot"`
	AfterSnapshot  json.RawMessage `json:"after_snapshot"`
	OccurredAt     time.Time       `json:"occurred_at"`
}
type OpsAuditFilter struct {
	AuditID                      string
	ActorUserID                  int64
	Action, TargetType, TargetID string
	From, To                     time.Time
	Page, PageSize               int
}
type OpsAuditPage struct {
	Items    []OpsAuditEvent `json:"items"`
	Total    int64           `json:"total"`
	Page     int             `json:"page"`
	PageSize int             `json:"page_size"`
}
type opsAuditReader interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

// Old immutable history stays in place; direct saves use the simpler table.
const opsAuditSet = `WITH events AS (
 SELECT id AS audit_id,actor_user_id,action,target_type,target AS target_id,before_state AS before_snapshot,after_state AS after_snapshot,occurred_at FROM ops.audit_log
 UNION ALL
 SELECT audit_id,actor_newapi_user_id,action,target_type,target_id,before_snapshot,after_snapshot,occurred_at FROM audit.audit_events
), filtered AS (
 SELECT * FROM events WHERE ($1::bigint=0 OR actor_user_id=$1)
 AND ($2='' OR target_type=$2) AND ($3='' OR target_id=$3) AND ($4='' OR action=$4)
 AND ($5::timestamptz IS NULL OR occurred_at >= $5) AND ($6::timestamptz IS NULL OR occurred_at < $6)
 AND ($7::uuid IS NULL OR audit_id=$7)
) `

func (s *Store) ReadOpsAudit(ctx context.Context, filter OpsAuditFilter) (OpsAuditPage, error) {
	return readOpsAudit(ctx, s.pool, filter)
}
func readOpsAudit(ctx context.Context, q opsAuditReader, filter OpsAuditFilter) (OpsAuditPage, error) {
	page := OpsAuditPage{Items: []OpsAuditEvent{}, Page: filter.Page, PageSize: filter.PageSize}
	var id any
	if filter.AuditID != "" {
		id = filter.AuditID
	}
	args := []any{filter.ActorUserID, filter.TargetType, filter.TargetID, filter.Action, nullableTime(filter.From), nullableTime(filter.To), id}
	if err := q.QueryRow(ctx, opsAuditSet+`SELECT count(*) FROM filtered`, args...).Scan(&page.Total); err != nil {
		return page, err
	}
	args = append(args, filter.PageSize, int64(filter.Page-1)*int64(filter.PageSize))
	rows, err := q.Query(ctx, opsAuditSet+`SELECT a.audit_id::text,a.actor_user_id,COALESCE(NULLIF(u.display_name,''),u.username,''),a.action,a.target_type,a.target_id,a.before_snapshot,a.after_snapshot,a.occurred_at
 FROM filtered a LEFT JOIN identity.users u ON u.id=a.actor_user_id
 ORDER BY a.occurred_at DESC,a.audit_id DESC LIMIT $8 OFFSET $9`, args...)
	if err != nil {
		return page, err
	}
	defer rows.Close()
	for rows.Next() {
		var item OpsAuditEvent
		if err = rows.Scan(&item.AuditID, &item.ActorUserID, &item.ActorName, &item.Action, &item.TargetType, &item.TargetID, &item.BeforeSnapshot, &item.AfterSnapshot, &item.OccurredAt); err != nil {
			return page, err
		}
		page.Items = append(page.Items, item)
	}
	return page, rows.Err()
}
