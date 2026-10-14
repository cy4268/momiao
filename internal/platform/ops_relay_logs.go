package platform

import (
	"context"
	"time"
)

type OpsLogFilter struct {
	Page, PageSize, Start, End, User, Key, Channel int64
	Model, Group, Dimension                        string
	ErrorsOnly                                     bool
}
type OpsRelayLog struct {
	ID          int64     `json:"id"`
	UserID      int64     `json:"user_id"`
	Username    string    `json:"username"`
	KeyID       int64     `json:"key_id"`
	KeyName     string    `json:"key_name"`
	Model       string    `json:"model"`
	ChannelID   int64     `json:"channel_id"`
	ChannelName string    `json:"channel_name"`
	Group       string    `json:"group"`
	CreatedAt   time.Time `json:"created_at"`
	Tokens      string    `json:"tokens"`
	Cost        string    `json:"cost"`
	Duration    int64     `json:"duration_ms"`
	Status      int       `json:"status_code"`
	Error       string    `json:"error"`
	Kind        int       `json:"kind"`
	RequestID   string    `json:"request_id"`
	Attempt     int       `json:"attempt"`
}
type OpsLogSummary struct {
	Requests  int64   `json:"requests"`
	Attempts  int64   `json:"attempts"`
	Errors    int64   `json:"errors"`
	Tokens    string  `json:"tokens"`
	Cost      string  `json:"cost"`
	ErrorRate float64 `json:"error_rate"`
}
type OpsLogTop struct {
	Name string `json:"name"`
	OpsLogSummary
}
type OpsLogPage struct {
	Items    []OpsRelayLog `json:"items"`
	Total    int64         `json:"total"`
	Page     int64         `json:"page"`
	PageSize int64         `json:"page_size"`
	Summary  OpsLogSummary `json:"summary"`
	Top      []OpsLogTop   `json:"top"`
}

func (s *Store) LocalOpsLogs(ctx context.Context, f OpsLogFilter) (OpsLogPage, error) {
	return localOpsLogs(ctx, s.pool, f)
}
func localOpsLogs(ctx context.Context, q announcementQuerier, f OpsLogFilter) (OpsLogPage, error) {
	out := OpsLogPage{Items: []OpsRelayLog{}, Top: []OpsLogTop{}, Page: f.Page, PageSize: f.PageSize}
	where := ` WHERE l.kind IN(2,5) AND ($1::bigint=0 OR l.created_at>=to_timestamp($1::bigint)) AND ($2::bigint=0 OR l.created_at<=to_timestamp($2::bigint))
 AND ($3::bigint=0 OR l.user_id=$3) AND ($4::bigint=0 OR l.key_id=$4) AND ($5::text='' OR l.model=$5)
 AND ($6::bigint=0 OR l.channel_id=$6) AND ($7::text='' OR l.group_name=$7) AND (NOT $8::boolean OR l.kind=5 OR l.status_code>=400)`
	args := []any{f.Start, f.End, f.User, f.Key, f.Model, f.Channel, f.Group, f.ErrorsOnly}
	// Failed attempts followed by success count as one successful logical request.
	filtered := `WITH filtered AS (SELECT l.*,coalesce(l.request_id,'native:'||l.id::text) request_key FROM relay.logs l` + where + `),
 requests AS (SELECT request_key,bool_and(kind=5 OR status_code>=400) failed,count(*) attempts,sum(prompt_tokens::numeric+completion_tokens) tokens,sum(cost_units) cost FROM filtered GROUP BY request_key) `
	summarySQL := `count(*),coalesce(sum(attempts),0)::bigint,count(*) FILTER(WHERE failed),coalesce(sum(tokens),0)::text,trim_scale(coalesce(sum(cost),0)::numeric/500000)::text`
	if err := q.QueryRow(ctx, filtered+`SELECT `+summarySQL+` FROM requests`, args...).Scan(&out.Summary.Requests, &out.Summary.Attempts, &out.Summary.Errors, &out.Summary.Tokens, &out.Summary.Cost); err != nil {
		return out, err
	}
	out.Total = out.Summary.Attempts
	if out.Summary.Requests > 0 {
		out.Summary.ErrorRate = float64(out.Summary.Errors) / float64(out.Summary.Requests)
	}
	rows, err := q.Query(ctx, `SELECT l.id,l.user_id,coalesce(u.username,''),coalesce(l.key_id,0),l.key_name,l.model,coalesce(l.channel_id,0),coalesce(c.name,''),l.group_name,l.created_at,(l.prompt_tokens::numeric+l.completion_tokens)::text,trim_scale(l.cost_units::numeric/500000)::text,l.duration_ms,l.status_code,l.error,l.kind,coalesce(l.request_id,''),l.attempt
 FROM relay.logs l LEFT JOIN identity.users u ON u.id=l.user_id LEFT JOIN relay.channels c ON c.id=l.channel_id`+where+` ORDER BY l.created_at DESC,l.id DESC LIMIT $9 OFFSET $10`, append(args, f.PageSize, (f.Page-1)*f.PageSize)...)
	if err != nil {
		return out, err
	}
	for rows.Next() {
		var v OpsRelayLog
		if err = rows.Scan(&v.ID, &v.UserID, &v.Username, &v.KeyID, &v.KeyName, &v.Model, &v.ChannelID, &v.ChannelName, &v.Group, &v.CreatedAt, &v.Tokens, &v.Cost, &v.Duration, &v.Status, &v.Error, &v.Kind, &v.RequestID, &v.Attempt); err != nil {
			rows.Close()
			return out, err
		}
		out.Items = append(out.Items, v)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return out, err
	}
	dimension := map[string]string{"model": "model", "channel": "coalesce(channel_id,0)::text", "user": "user_id::text"}[f.Dimension]
	if dimension == "" {
		return out, ErrOpsInvalid
	}
	topSQL := `WITH filtered AS (SELECT l.*,coalesce(l.request_id,'native:'||l.id::text) request_key FROM relay.logs l` + where + `),
 requests AS (SELECT ` + dimension + ` name,request_key,bool_and(kind=5 OR status_code>=400) failed,count(*) attempts,sum(prompt_tokens::numeric+completion_tokens) tokens,sum(cost_units) cost FROM filtered GROUP BY name,request_key)
 SELECT name,` + summarySQL + ` FROM requests GROUP BY name ORDER BY count(*) DESC,name LIMIT 20`
	rows, err = q.Query(ctx, topSQL, args...)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var v OpsLogTop
		if err = rows.Scan(&v.Name, &v.Requests, &v.Attempts, &v.Errors, &v.Tokens, &v.Cost); err != nil {
			return out, err
		}
		if v.Requests > 0 {
			v.ErrorRate = float64(v.Errors) / float64(v.Requests)
		}
		out.Top = append(out.Top, v)
	}
	return out, rows.Err()
}
