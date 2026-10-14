package platform

import (
	"context"
	"time"
)

type OpsJobSummary struct {
	Kind        string     `json:"kind"`
	Pending     int64      `json:"pending_count,string"`
	Attention   int64      `json:"attention_count,string"`
	Due         int64      `json:"due_count,string"`
	OldestDueAt *time.Time `json:"oldest_due_at"`
}
type OpsJob struct {
	Kind      string     `json:"kind"`
	ID        string     `json:"job_id"`
	TargetID  string     `json:"target_id"`
	State     string     `json:"state"`
	Pending   bool       `json:"pending"`
	Attention bool       `json:"attention"`
	RunAt     *time.Time `json:"run_at"`
	UpdatedAt time.Time  `json:"updated_at"`
	Legacy    bool       `json:"legacy"`
}
type OpsJobsFilter struct {
	Kind, State   string
	Page          int
	From, To      time.Time
	AttentionOnly bool
}
type OpsJobsPage struct {
	Summary  []OpsJobSummary `json:"summary"`
	Items    []OpsJob        `json:"items"`
	Total    int64           `json:"total"`
	Page     int             `json:"page"`
	PageSize int             `json:"page_size"`
}

func (s *Store) ReadOpsJobs(ctx context.Context, filter OpsJobsFilter) (OpsJobsPage, error) {
	return readOpsJobs(ctx, s.pool, filter)
}
func readOpsJobs(ctx context.Context, q announcementQuerier, filter OpsJobsFilter) (OpsJobsPage, error) {
	page := OpsJobsPage{Summary: []OpsJobSummary{}, Items: []OpsJob{}, Page: filter.Page, PageSize: 20}
	rows, err := q.Query(ctx, `SELECT kind,pending_count,attention_count,due_count,oldest_due_at FROM ops.runtime_jobs_read() ORDER BY kind`)
	if err != nil {
		return page, err
	}
	for rows.Next() {
		var item OpsJobSummary
		if err = rows.Scan(&item.Kind, &item.Pending, &item.Attention, &item.Due, &item.OldestDueAt); err != nil {
			rows.Close()
			return page, err
		}
		page.Summary = append(page.Summary, item)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return page, err
	}
	where := ` FROM ops.runtime_job_items() WHERE ($1='' OR kind=$1) AND ($2='' OR state=$2)
 AND ($3::timestamptz IS NULL OR updated_at >= $3) AND ($4::timestamptz IS NULL OR updated_at < $4)
 AND (NOT $5::boolean OR attention)`
	args := []any{filter.Kind, filter.State, nullableTime(filter.From), nullableTime(filter.To), filter.AttentionOnly}
	if err = q.QueryRow(ctx, `SELECT count(*)`+where, args...).Scan(&page.Total); err != nil {
		return page, err
	}
	args = append(args, int64(filter.Page-1)*20)
	rows, err = q.Query(ctx, `SELECT kind,job_id,target_id,state,pending,attention,run_at,updated_at,legacy`+where+` ORDER BY updated_at DESC,kind,job_id LIMIT 20 OFFSET $6`, args...)
	if err != nil {
		return page, err
	}
	defer rows.Close()
	for rows.Next() {
		var item OpsJob
		if err = rows.Scan(&item.Kind, &item.ID, &item.TargetID, &item.State, &item.Pending, &item.Attention, &item.RunAt, &item.UpdatedAt, &item.Legacy); err != nil {
			return page, err
		}
		page.Items = append(page.Items, item)
	}
	return page, rows.Err()
}
