package platform

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

type OpsIncident struct {
	IncidentID      string     `json:"incident_id"`
	Title           string     `json:"title"`
	Severity        string     `json:"severity"`
	State           string     `json:"state"`
	SafeSummary     string     `json:"safe_summary"`
	Version         int64      `json:"-"`
	CreatedBy       int64      `json:"created_by,string"`
	LastOperationID string     `json:"-"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
	ResolvedAt      *time.Time `json:"resolved_at,omitempty"`
	ClosedAt        *time.Time `json:"closed_at,omitempty"`
}

type OpsIncidentEvent struct {
	EventID             string    `json:"event_id"`
	IncidentID          string    `json:"incident_id"`
	EventType           string    `json:"event_type"`
	FromState           string    `json:"from_state,omitempty"`
	ToState             string    `json:"to_state,omitempty"`
	SafeSummary         string    `json:"safe_summary,omitempty"`
	RelatedType         string    `json:"related_type,omitempty"`
	RelatedID           string    `json:"related_id,omitempty"`
	AssignedPrincipalID *string   `json:"assigned_principal_id,omitempty"`
	ActorUserID         int64     `json:"actor_newapi_user_id,string"`
	OperationID         string    `json:"-"`
	OccurredAt          time.Time `json:"occurred_at"`
}

type OpsIncidentDetail struct {
	Incident OpsIncident        `json:"incident"`
	Events   []OpsIncidentEvent `json:"events"`
}

type OpsIncidentQuery struct {
	State    string
	Severity string
	Page     int
	Search   string
	From, To time.Time
}

const opsIncidentColumns = `incident_id::text,title,severity,state,safe_summary,version,created_by,
 COALESCE(last_operation_id::text,''),created_at,updated_at,resolved_at,closed_at`

func scanOpsIncident(row pgx.Row) (OpsIncident, error) {
	var item OpsIncident
	err := row.Scan(&item.IncidentID, &item.Title, &item.Severity, &item.State, &item.SafeSummary,
		&item.Version, &item.CreatedBy, &item.LastOperationID, &item.CreatedAt, &item.UpdatedAt,
		&item.ResolvedAt, &item.ClosedAt)
	if err != nil {
		return item, err
	}
	item.CreatedAt, item.UpdatedAt = item.CreatedAt.UTC(), item.UpdatedAt.UTC()
	if item.ResolvedAt != nil {
		value := item.ResolvedAt.UTC()
		item.ResolvedAt = &value
	}
	if item.ClosedAt != nil {
		value := item.ClosedAt.UTC()
		item.ClosedAt = &value
	}
	return item, nil
}

func validIncidentState(value string, allowEmpty bool) bool {
	return allowEmpty && value == "" || containsString([]string{
		"OPEN", "TRIAGED", "IN_PROGRESS", "MONITORING", "RESOLVED", "CLOSED",
	}, value)
}

func validIncidentSeverity(value string, allowEmpty bool) bool {
	return allowEmpty && value == "" || containsString([]string{"CRITICAL", "WARNING", "INFO"}, value)
}

func incidentTransitionAllowed(from, to string) bool {
	if from == "CLOSED" || from == to {
		return false
	}
	if to == "CLOSED" {
		return true
	}
	switch from {
	case "OPEN":
		return to == "TRIAGED"
	case "TRIAGED":
		return to == "IN_PROGRESS"
	case "IN_PROGRESS":
		return to == "MONITORING"
	case "MONITORING":
		return to == "RESOLVED"
	case "RESOLVED":
		return to == "CLOSED"
	default:
		return false
	}
}

type OpsIncidentPage struct {
	Items    []OpsIncident `json:"items"`
	Total    int64         `json:"total"`
	Page     int           `json:"page"`
	PageSize int           `json:"page_size"`
}

func (s *Store) OpsIncidents(ctx context.Context, query OpsIncidentQuery) (OpsIncidentPage, error) {
	return readOpsIncidents(ctx, s.pool, query)
}
func readOpsIncidents(ctx context.Context, q announcementQuerier, query OpsIncidentQuery) (OpsIncidentPage, error) {
	page := OpsIncidentPage{Items: []OpsIncident{}, Page: query.Page, PageSize: 20}
	where := ` FROM ops.incidents WHERE ($1='' OR state=$1) AND ($2='' OR severity=$2)
 AND ($3='' OR title ILIKE '%'||$3||'%' OR safe_summary ILIKE '%'||$3||'%')
 AND ($4::timestamptz IS NULL OR created_at >= $4) AND ($5::timestamptz IS NULL OR created_at < $5)`
	args := []any{query.State, query.Severity, query.Search, nullableTime(query.From), nullableTime(query.To)}
	if err := q.QueryRow(ctx, `SELECT count(*)`+where, args...).Scan(&page.Total); err != nil {
		return page, err
	}
	args = append(args, int64(query.Page-1)*20)
	rows, err := q.Query(ctx, `SELECT `+opsIncidentColumns+where+` ORDER BY created_at DESC,incident_id DESC LIMIT 20 OFFSET $6`, args...)
	if err != nil {
		return page, err
	}
	defer rows.Close()
	for rows.Next() {
		item, e := scanOpsIncident(rows)
		if e != nil {
			return page, e
		}
		page.Items = append(page.Items, item)
	}
	return page, rows.Err()
}

func (s *Store) OpsIncidentDetail(ctx context.Context, incidentID string) (OpsIncidentDetail, error) {
	return readOpsIncident(ctx, s.pool, incidentID)
}
func readOpsIncident(ctx context.Context, q announcementQuerier, incidentID string) (OpsIncidentDetail, error) {
	detail := OpsIncidentDetail{Events: []OpsIncidentEvent{}}
	var err error
	detail.Incident, err = scanOpsIncident(q.QueryRow(ctx, `SELECT `+opsIncidentColumns+`
	 FROM ops.incidents WHERE incident_id=$1`, incidentID))
	if errors.Is(err, pgx.ErrNoRows) {
		return detail, ErrOpsNotFound
	}
	if err != nil {
		return detail, err
	}
	rows, err := q.Query(ctx, `SELECT event_id::text,incident_id::text,event_type,
	 COALESCE(from_state,''),COALESCE(to_state,''),COALESCE(safe_summary,''),
	 COALESCE(related_type,''),COALESCE(related_id,''),assigned_principal_id::text,
	 actor_newapi_user_id,COALESCE(operation_id::text,''),occurred_at
	 FROM ops.incident_events WHERE incident_id=$1 ORDER BY occurred_at,event_id`, incidentID)
	if err != nil {
		return detail, err
	}
	defer rows.Close()
	for rows.Next() {
		var event OpsIncidentEvent
		if err = rows.Scan(&event.EventID, &event.IncidentID, &event.EventType, &event.FromState,
			&event.ToState, &event.SafeSummary, &event.RelatedType, &event.RelatedID,
			&event.AssignedPrincipalID, &event.ActorUserID, &event.OperationID, &event.OccurredAt); err != nil {
			return detail, err
		}
		event.OccurredAt = event.OccurredAt.UTC()
		detail.Events = append(detail.Events, event)
	}
	return detail, rows.Err()
}

func loadOpsIncident(ctx context.Context, tx pgx.Tx, incidentID string, lock bool) (OpsIncident, error) {
	query := `SELECT ` + opsIncidentColumns + ` FROM ops.incidents WHERE incident_id=$1`
	if lock {
		query += " FOR UPDATE"
	}
	item, err := scanOpsIncident(tx.QueryRow(ctx, query, incidentID))
	if errors.Is(err, pgx.ErrNoRows) {
		return item, ErrOpsNotFound
	}
	return item, err
}

type OpsIncidentInput struct {
	Title       string `json:"title"`
	Severity    string `json:"severity"`
	SafeSummary string `json:"safe_summary"`
	NextState   string `json:"next_state"`
	EventType   string `json:"event_type"`
	RelatedType string `json:"related_type"`
	RelatedID   string `json:"related_id"`
}

func (s *Store) SaveOpsIncident(ctx context.Context, actor int64, id, action string, input OpsIncidentInput) (OpsIncident, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return OpsIncident{}, err
	}
	defer tx.Rollback(ctx)
	result, err := saveOpsIncident(ctx, tx, actor, id, action, input)
	if err != nil {
		return result, err
	}
	return result, tx.Commit(ctx)
}
func saveOpsIncident(ctx context.Context, tx pgx.Tx, actor int64, id, action string, input OpsIncidentInput) (OpsIncident, error) {
	var before, after OpsIncident
	switch action {
	case "create":
		if !validOpsText(input.Title, 256, true) || !validIncidentSeverity(input.Severity, false) || !validOpsText(input.SafeSummary, 4096, true) {
			return after, ErrOpsInvalid
		}
	case "state":
		if !validIncidentState(input.NextState, false) {
			return after, ErrOpsInvalid
		}
	case "events":
		if _, err := normalizeIncidentEvent(input); err != nil {
			return after, err
		}
	default:
		return after, ErrOpsInvalid
	}
	if err := RequireLocalOpsAdmin(ctx, tx, actor); err != nil {
		return after, err
	}
	var err error
	if action == "create" {
		id, err = uuidV7()
		if err != nil {
			return after, err
		}
		after, err = scanOpsIncident(tx.QueryRow(ctx, `INSERT INTO ops.incidents(incident_id,title,severity,safe_summary,created_by) VALUES($1,$2,$3,$4,$5) RETURNING `+opsIncidentColumns, id, input.Title, input.Severity, input.SafeSummary, actor))
	} else {
		before, err = loadOpsIncident(ctx, tx, id, true)
		if err != nil {
			return after, err
		}
		if before.State == "CLOSED" {
			return after, ErrOpsConflict
		}
		next := before.State
		if action == "state" {
			if input.NextState != before.State && !incidentTransitionAllowed(before.State, input.NextState) {
				return after, ErrOpsConflict
			}
			next = input.NextState
		}
		after, err = scanOpsIncident(tx.QueryRow(ctx, `UPDATE ops.incidents SET state=$2,version=version+1,last_operation_id=NULL,updated_at=clock_timestamp(),resolved_at=CASE WHEN $2='RESOLVED' THEN COALESCE(resolved_at,clock_timestamp()) ELSE resolved_at END,closed_at=CASE WHEN $2='CLOSED' THEN clock_timestamp() ELSE NULL END WHERE incident_id=$1 RETURNING `+opsIncidentColumns, id, next))
	}
	if err != nil {
		return after, err
	}
	var event OpsIncidentEvent
	if action == "events" {
		event, err = insertIncidentEvent(ctx, tx, id, input.EventType, "", "", input.SafeSummary, input.RelatedType, input.RelatedID, nil, actor)
	} else if before.State != after.State {
		event, err = insertIncidentEvent(ctx, tx, id, "STATE_CHANGED", before.State, after.State, "", "", "", nil, actor)
	}
	if err != nil {
		return after, err
	}
	actions := map[string]string{"create": "INCIDENT_CREATE", "state": "INCIDENT_TRANSITION", "events": "INCIDENT_EVENT_ADD"}
	err = RecordOpsChange(ctx, tx, actor, actions[action], "INCIDENT", id, before, map[string]any{"incident": after, "event": event})
	return after, err
}

func incidentRelatedType(eventType string) string {
	return map[string]string{
		"OPERATION_LINKED":     "OPERATION",
		"JOB_LINKED":           "JOB",
		"AUDIT_LINKED":         "AUDIT",
		"BUSINESS_FACT_LINKED": "BUSINESS_FACT",
	}[eventType]
}

func normalizeIncidentEvent(input OpsIncidentInput) (OpsIncidentInput, error) {
	if !containsString([]string{
		"COMMENT", "OPERATION_LINKED", "JOB_LINKED", "AUDIT_LINKED", "BUSINESS_FACT_LINKED",
	}, input.EventType) || !validOpsText(input.SafeSummary, 2048, false) {
		return input, ErrOpsInvalid
	}
	expected := incidentRelatedType(input.EventType)
	if input.EventType == "COMMENT" {
		if input.SafeSummary == "" || input.RelatedType != "" || input.RelatedID != "" {
			return input, ErrOpsInvalid
		}
		return input, nil
	}
	if input.RelatedType != expected || !validOpsText(input.RelatedID, 512, true) {
		return input, ErrOpsInvalid
	}
	return input, nil
}

func insertIncidentEvent(ctx context.Context, tx pgx.Tx, incidentID, eventType, fromState, toState,
	safeSummary, relatedType, relatedID string, assignedPrincipalID *string, actor int64) (OpsIncidentEvent, error) {
	eventID, err := uuidV7()
	if err != nil {
		return OpsIncidentEvent{}, err
	}
	var event OpsIncidentEvent
	err = tx.QueryRow(ctx, `INSERT INTO ops.incident_events(
	 event_id,incident_id,event_type,from_state,to_state,safe_summary,related_type,related_id,
	 assigned_principal_id,actor_newapi_user_id)
	 VALUES($1,$2,$3,NULLIF($4,''),NULLIF($5,''),NULLIF($6,''),NULLIF($7,''),NULLIF($8,''),$9,$10)
	 RETURNING event_id::text,incident_id::text,event_type,COALESCE(from_state,''),COALESCE(to_state,''),
	  COALESCE(safe_summary,''),COALESCE(related_type,''),COALESCE(related_id,''),
	  assigned_principal_id::text,actor_newapi_user_id,COALESCE(operation_id::text,''),occurred_at`,
		eventID, incidentID, eventType, fromState, toState, safeSummary, relatedType, relatedID,
		assignedPrincipalID, actor).Scan(&event.EventID, &event.IncidentID, &event.EventType,
		&event.FromState, &event.ToState, &event.SafeSummary, &event.RelatedType, &event.RelatedID,
		&event.AssignedPrincipalID, &event.ActorUserID, &event.OperationID, &event.OccurredAt)
	if err != nil {
		return OpsIncidentEvent{}, err
	}
	event.OccurredAt = event.OccurredAt.UTC()
	return event, nil
}
