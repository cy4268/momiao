package platform

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
)

type OpsIncident struct {
	IncidentID      string     `json:"incident_id"`
	Title           string     `json:"title"`
	Severity        string     `json:"severity"`
	State           string     `json:"state"`
	SafeSummary     string     `json:"safe_summary"`
	Version         int64      `json:"version,string"`
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
	Limit    int
}

const opsIncidentColumns = `incident_id::text,title,severity,state,safe_summary,version,created_by,
 last_operation_id::text,created_at,updated_at,resolved_at,closed_at`

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

func (s *Store) OpsIncidents(ctx context.Context, actor, epoch int64, query OpsIncidentQuery) ([]OpsIncident, error) {
	items := []OpsIncident{}
	if s == nil || s.pool == nil {
		return items, ErrOpsUnavailable
	}
	if !validIncidentState(query.State, true) || !validIncidentSeverity(query.Severity, true) ||
		query.Limit < 1 || query.Limit > 100 {
		return items, ErrOpsInvalid
	}
	if _, err := s.RequireOpsPermission(ctx, actor, epoch, "incidents.read"); err != nil {
		return items, err
	}
	rows, err := s.pool.Query(ctx, `SELECT `+opsIncidentColumns+` FROM ops.incidents
	 WHERE ($1='' OR state=$1) AND ($2='' OR severity=$2)
	 ORDER BY updated_at DESC,incident_id DESC LIMIT $3`, query.State, query.Severity, query.Limit)
	if err != nil {
		return items, err
	}
	defer rows.Close()
	for rows.Next() {
		item, scanErr := scanOpsIncident(rows)
		if scanErr != nil {
			return items, scanErr
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) OpsIncidentDetail(ctx context.Context, actor, epoch int64, incidentID string) (OpsIncidentDetail, error) {
	detail := OpsIncidentDetail{Events: []OpsIncidentEvent{}}
	if s == nil || s.pool == nil {
		return detail, ErrOpsUnavailable
	}
	if !opsUUIDPattern.MatchString(incidentID) {
		return detail, ErrOpsInvalid
	}
	if _, err := s.RequireOpsPermission(ctx, actor, epoch, "incidents.read"); err != nil {
		return detail, err
	}
	var err error
	detail.Incident, err = scanOpsIncident(s.pool.QueryRow(ctx, `SELECT `+opsIncidentColumns+`
	 FROM ops.incidents WHERE incident_id=$1`, incidentID))
	if errors.Is(err, pgx.ErrNoRows) {
		return detail, ErrOpsNotFound
	}
	if err != nil {
		return detail, err
	}
	rows, err := s.pool.Query(ctx, `SELECT event_id::text,incident_id::text,event_type,
	 COALESCE(from_state,''),COALESCE(to_state,''),COALESCE(safe_summary,''),
	 COALESCE(related_type,''),COALESCE(related_id,''),assigned_principal_id::text,
	 actor_newapi_user_id,operation_id::text,occurred_at
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

type opsIncidentCreateInput struct {
	Title       string `json:"title"`
	Severity    string `json:"severity"`
	SafeSummary string `json:"safe_summary"`
}

func normalizeIncidentCreate(input opsIncidentCreateInput) (opsIncidentCreateInput, error) {
	if !validOpsText(input.Title, 256, true) || !validIncidentSeverity(input.Severity, false) ||
		!validOpsText(input.SafeSummary, 4096, true) {
		return input, ErrOpsInvalid
	}
	return input, nil
}

type incidentCreateOpsHandler struct{ store *Store }

func (incidentCreateOpsHandler) Canonicalize(raw json.RawMessage) (json.RawMessage, error) {
	var input opsIncidentCreateInput
	if err := decodeOpsInput(raw, &input); err != nil {
		return nil, err
	}
	input, err := normalizeIncidentCreate(input)
	if err != nil {
		return nil, err
	}
	return json.Marshal(input)
}

func (handler incidentCreateOpsHandler) Prepare(ctx context.Context, tx pgx.Tx, actor OpsPrincipal, request OpsPrepareRequest, lock bool) (OpsPreparedMaterial, error) {
	if handler.store == nil || tx == nil || !opsUUIDPattern.MatchString(request.Target.ID) ||
		request.Target.ExpectedVersion != "0" {
		return OpsPreparedMaterial{}, ErrOpsInvalid
	}
	var input opsIncidentCreateInput
	if err := decodeOpsInput(request.Input, &input); err != nil {
		return OpsPreparedMaterial{}, err
	}
	input, err := normalizeIncidentCreate(input)
	if err != nil {
		return OpsPreparedMaterial{}, err
	}
	if lock {
		if err = lockIdentity(ctx, tx, "ops-incident", request.Target.ID); err != nil {
			return OpsPreparedMaterial{}, err
		}
	}
	var exists bool
	if err = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM ops.incidents WHERE incident_id=$1)",
		request.Target.ID).Scan(&exists); err != nil {
		return OpsPreparedMaterial{}, err
	}
	if exists {
		return OpsPreparedMaterial{}, ErrOpsConflict
	}
	proposed := OpsIncident{IncidentID: request.Target.ID, Title: input.Title, Severity: input.Severity,
		State: "OPEN", SafeSummary: input.SafeSummary, Version: 1, CreatedBy: actor.UserID}
	impact := OpsImpact{
		CurrentState: rawOpsValue(struct {
			Exists bool `json:"exists"`
		}{false}),
		ProposedChange: rawOpsValue(proposed), AffectedItems: affectedOne(),
		BlockingFacts: []string{}, ContinuingAcceptedWork: []string{},
		RelatedIDs: []string{request.Target.ID}, UnavailableMeasurements: []string{},
	}
	return OpsPreparedMaterial{Impact: impact, TargetVersion: "0", TargetLocator: request.Target.ID}, nil
}

func (handler incidentCreateOpsHandler) Execute(ctx context.Context, tx pgx.Tx, actor OpsPrincipal, operation OpsOperation, _ OpsPreparedMaterial) (OpsExecutionResult, error) {
	var input opsIncidentCreateInput
	if err := decodeOpsInput(operation.inputPayload, &input); err != nil {
		return OpsExecutionResult{}, err
	}
	after, err := scanOpsIncident(tx.QueryRow(ctx, `INSERT INTO ops.incidents(
	 incident_id,title,severity,safe_summary,created_by,last_operation_id)
	 VALUES($1,$2,$3,$4,$5,$6) RETURNING `+opsIncidentColumns,
		operation.Target.ID, input.Title, input.Severity, input.SafeSummary, actor.UserID, operation.OperationID))
	if err != nil {
		return OpsExecutionResult{}, err
	}
	if _, err = insertIncidentEvent(ctx, tx, operation.Target.ID, "STATE_CHANGED", "", "OPEN",
		"", "", "", nil, actor.UserID, operation.OperationID); err != nil {
		return OpsExecutionResult{}, err
	}
	before := struct {
		Exists bool `json:"exists"`
	}{false}
	result := struct {
		Incident OpsIncident `json:"incident"`
	}{after}
	return OpsExecutionResult{BeforeSnapshot: rawOpsValue(before), AfterSnapshot: rawOpsValue(after),
		Result: rawOpsValue(result), RelatedBusinessID: after.IncidentID}, nil
}

type opsIncidentTransitionInput struct {
	NextState string `json:"next_state"`
}

type incidentTransitionOpsHandler struct{ store *Store }

func (incidentTransitionOpsHandler) Canonicalize(raw json.RawMessage) (json.RawMessage, error) {
	var input opsIncidentTransitionInput
	if err := decodeOpsInput(raw, &input); err != nil || !validIncidentState(input.NextState, false) {
		return nil, ErrOpsInvalid
	}
	return json.Marshal(input)
}

func (handler incidentTransitionOpsHandler) Prepare(ctx context.Context, tx pgx.Tx, _ OpsPrincipal, request OpsPrepareRequest, lock bool) (OpsPreparedMaterial, error) {
	if handler.store == nil || tx == nil || !opsUUIDPattern.MatchString(request.Target.ID) {
		return OpsPreparedMaterial{}, ErrOpsInvalid
	}
	var input opsIncidentTransitionInput
	if err := decodeOpsInput(request.Input, &input); err != nil || !validIncidentState(input.NextState, false) {
		return OpsPreparedMaterial{}, ErrOpsInvalid
	}
	current, err := loadOpsIncident(ctx, tx, request.Target.ID, lock)
	if err != nil {
		return OpsPreparedMaterial{}, err
	}
	if request.Target.ExpectedVersion != strconv.FormatInt(current.Version, 10) {
		return OpsPreparedMaterial{}, ErrOpsPreviewStale
	}
	if !incidentTransitionAllowed(current.State, input.NextState) || current.Version == math.MaxInt64 {
		return OpsPreparedMaterial{}, ErrOpsConflict
	}
	proposed := struct {
		IncidentID string `json:"incident_id"`
		State      string `json:"state"`
		Version    int64  `json:"version,string"`
	}{current.IncidentID, input.NextState, current.Version + 1}
	impact := OpsImpact{CurrentState: rawOpsValue(current), ProposedChange: rawOpsValue(proposed),
		AffectedItems: affectedOne(), BlockingFacts: []string{}, ContinuingAcceptedWork: []string{},
		RelatedIDs: []string{current.IncidentID}, UnavailableMeasurements: []string{}}
	return OpsPreparedMaterial{Impact: impact, TargetVersion: strconv.FormatInt(current.Version, 10),
		TargetLocator: current.IncidentID}, nil
}

func (handler incidentTransitionOpsHandler) Execute(ctx context.Context, tx pgx.Tx, actor OpsPrincipal, operation OpsOperation, _ OpsPreparedMaterial) (OpsExecutionResult, error) {
	var input opsIncidentTransitionInput
	if err := decodeOpsInput(operation.inputPayload, &input); err != nil {
		return OpsExecutionResult{}, err
	}
	before, err := loadOpsIncident(ctx, tx, operation.Target.ID, false)
	if err != nil {
		return OpsExecutionResult{}, err
	}
	after, err := scanOpsIncident(tx.QueryRow(ctx, `UPDATE ops.incidents SET state=$2,version=version+1,
	 last_operation_id=$3,updated_at=clock_timestamp(),
	 resolved_at=CASE WHEN $2='RESOLVED' THEN clock_timestamp() ELSE resolved_at END,
	 closed_at=CASE WHEN $2='CLOSED' THEN clock_timestamp() ELSE NULL END
	 WHERE incident_id=$1 RETURNING `+opsIncidentColumns, operation.Target.ID, input.NextState, operation.OperationID))
	if err != nil {
		return OpsExecutionResult{}, err
	}
	if _, err = insertIncidentEvent(ctx, tx, operation.Target.ID, "STATE_CHANGED", before.State, after.State,
		"", "", "", nil, actor.UserID, operation.OperationID); err != nil {
		return OpsExecutionResult{}, err
	}
	result := struct {
		Incident OpsIncident `json:"incident"`
	}{after}
	return OpsExecutionResult{BeforeSnapshot: rawOpsValue(before), AfterSnapshot: rawOpsValue(after),
		Result: rawOpsValue(result), RelatedBusinessID: after.IncidentID}, nil
}

type opsIncidentEventInput struct {
	EventType   string `json:"event_type"`
	SafeSummary string `json:"safe_summary,omitempty"`
	RelatedType string `json:"related_type,omitempty"`
	RelatedID   string `json:"related_id,omitempty"`
}

func incidentRelatedType(eventType string) string {
	return map[string]string{
		"OPERATION_LINKED":     "OPERATION",
		"JOB_LINKED":           "JOB",
		"AUDIT_LINKED":         "AUDIT",
		"BUSINESS_FACT_LINKED": "BUSINESS_FACT",
	}[eventType]
}

func normalizeIncidentEvent(input opsIncidentEventInput) (opsIncidentEventInput, error) {
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

type incidentEventOpsHandler struct{ store *Store }

func (incidentEventOpsHandler) Canonicalize(raw json.RawMessage) (json.RawMessage, error) {
	var input opsIncidentEventInput
	if err := decodeOpsInput(raw, &input); err != nil {
		return nil, err
	}
	input, err := normalizeIncidentEvent(input)
	if err != nil {
		return nil, err
	}
	return json.Marshal(input)
}

func (handler incidentEventOpsHandler) Prepare(ctx context.Context, tx pgx.Tx, _ OpsPrincipal, request OpsPrepareRequest, lock bool) (OpsPreparedMaterial, error) {
	if handler.store == nil || tx == nil || !opsUUIDPattern.MatchString(request.Target.ID) {
		return OpsPreparedMaterial{}, ErrOpsInvalid
	}
	var input opsIncidentEventInput
	if err := decodeOpsInput(request.Input, &input); err != nil {
		return OpsPreparedMaterial{}, err
	}
	input, err := normalizeIncidentEvent(input)
	if err != nil {
		return OpsPreparedMaterial{}, err
	}
	current, err := loadOpsIncident(ctx, tx, request.Target.ID, lock)
	if err != nil {
		return OpsPreparedMaterial{}, err
	}
	if request.Target.ExpectedVersion != strconv.FormatInt(current.Version, 10) {
		return OpsPreparedMaterial{}, ErrOpsPreviewStale
	}
	if current.State == "CLOSED" || current.Version == math.MaxInt64 {
		return OpsPreparedMaterial{}, ErrOpsConflict
	}
	proposed := struct {
		IncidentID  string `json:"incident_id"`
		EventType   string `json:"event_type"`
		SafeSummary string `json:"safe_summary,omitempty"`
		RelatedType string `json:"related_type,omitempty"`
		RelatedID   string `json:"related_id,omitempty"`
		NextVersion int64  `json:"next_version,string"`
	}{current.IncidentID, input.EventType, input.SafeSummary, input.RelatedType, input.RelatedID, current.Version + 1}
	impact := OpsImpact{CurrentState: rawOpsValue(current), ProposedChange: rawOpsValue(proposed),
		AffectedItems: affectedOne(), BlockingFacts: []string{}, ContinuingAcceptedWork: []string{},
		RelatedIDs: []string{current.IncidentID}, UnavailableMeasurements: []string{}}
	return OpsPreparedMaterial{Impact: impact, TargetVersion: strconv.FormatInt(current.Version, 10),
		TargetLocator: current.IncidentID}, nil
}

func (handler incidentEventOpsHandler) Execute(ctx context.Context, tx pgx.Tx, actor OpsPrincipal, operation OpsOperation, _ OpsPreparedMaterial) (OpsExecutionResult, error) {
	var input opsIncidentEventInput
	if err := decodeOpsInput(operation.inputPayload, &input); err != nil {
		return OpsExecutionResult{}, err
	}
	before, err := loadOpsIncident(ctx, tx, operation.Target.ID, false)
	if err != nil {
		return OpsExecutionResult{}, err
	}
	after, err := scanOpsIncident(tx.QueryRow(ctx, `UPDATE ops.incidents SET version=version+1,
	 last_operation_id=$2,updated_at=clock_timestamp() WHERE incident_id=$1 RETURNING `+opsIncidentColumns,
		operation.Target.ID, operation.OperationID))
	if err != nil {
		return OpsExecutionResult{}, err
	}
	event, err := insertIncidentEvent(ctx, tx, operation.Target.ID, input.EventType, "", "",
		input.SafeSummary, input.RelatedType, input.RelatedID, nil, actor.UserID, operation.OperationID)
	if err != nil {
		return OpsExecutionResult{}, err
	}
	result := struct {
		Incident OpsIncident      `json:"incident"`
		Event    OpsIncidentEvent `json:"event"`
	}{after, event}
	return OpsExecutionResult{BeforeSnapshot: rawOpsValue(before), AfterSnapshot: rawOpsValue(after),
		Result: rawOpsValue(result), RelatedBusinessID: after.IncidentID}, nil
}

func insertIncidentEvent(ctx context.Context, tx pgx.Tx, incidentID, eventType, fromState, toState,
	safeSummary, relatedType, relatedID string, assignedPrincipalID *string, actor int64,
	operationID string) (OpsIncidentEvent, error) {
	eventID, err := uuidV7()
	if err != nil {
		return OpsIncidentEvent{}, err
	}
	var event OpsIncidentEvent
	err = tx.QueryRow(ctx, `INSERT INTO ops.incident_events(
	 event_id,incident_id,event_type,from_state,to_state,safe_summary,related_type,related_id,
	 assigned_principal_id,actor_newapi_user_id,operation_id)
	 VALUES($1,$2,$3,NULLIF($4,''),NULLIF($5,''),NULLIF($6,''),NULLIF($7,''),NULLIF($8,''),$9,$10,$11)
	 RETURNING event_id::text,incident_id::text,event_type,COALESCE(from_state,''),COALESCE(to_state,''),
	  COALESCE(safe_summary,''),COALESCE(related_type,''),COALESCE(related_id,''),
	  assigned_principal_id::text,actor_newapi_user_id,operation_id::text,occurred_at`,
		eventID, incidentID, eventType, fromState, toState, safeSummary, relatedType, relatedID,
		assignedPrincipalID, actor, operationID).Scan(&event.EventID, &event.IncidentID, &event.EventType,
		&event.FromState, &event.ToState, &event.SafeSummary, &event.RelatedType, &event.RelatedID,
		&event.AssignedPrincipalID, &event.ActorUserID, &event.OperationID, &event.OccurredAt)
	if err != nil {
		return OpsIncidentEvent{}, err
	}
	event.OccurredAt = event.OccurredAt.UTC()
	return event, nil
}

// OpsIncidentBindings supplies O04 incident writes; formal History remains read-only.
func OpsIncidentBindings(store *Store) []OpsOperationBinding {
	if store == nil {
		return []OpsOperationBinding{}
	}
	return []OpsOperationBinding{
		{Descriptor: OpsOperationDescriptor{OperationType: "INCIDENT_CREATE", Risk: OpsRiskRoutine,
			RequiredPermission: "records.incident.create", AllowedRoles: []string{"SUPER_ADMIN", "OPERATOR"},
			TargetType: "INCIDENT", InputSchemaVersion: "incident-create.v1",
			ImpactSchemaVersion: "incident-create-impact.v1", RequiresReason: true,
			ConfirmationMode: OpsConfirmNone, ExecutionMode: OpsSameDatabase},
			Handler: incidentCreateOpsHandler{store: store}},
		{Descriptor: OpsOperationDescriptor{OperationType: "INCIDENT_TRANSITION", Risk: OpsRiskImpactful,
			RequiredPermission: "incidents.write", AllowedRoles: []string{"SUPER_ADMIN", "OPERATOR"},
			TargetType: "INCIDENT", InputSchemaVersion: "incident-transition.v1",
			ImpactSchemaVersion: "incident-transition-impact.v1", RequiresReason: true,
			ConfirmationMode: OpsConfirmExplicit, ExecutionMode: OpsSameDatabase},
			Handler: incidentTransitionOpsHandler{store: store}},
		{Descriptor: OpsOperationDescriptor{OperationType: "INCIDENT_EVENT_ADD", Risk: OpsRiskRoutine,
			RequiredPermission: "incidents.write", AllowedRoles: []string{"SUPER_ADMIN", "OPERATOR"},
			TargetType: "INCIDENT", InputSchemaVersion: "incident-event-add.v1",
			ImpactSchemaVersion: "incident-event-add-impact.v1", RequiresReason: true,
			ConfirmationMode: OpsConfirmNone, ExecutionMode: OpsSameDatabase},
			Handler: incidentEventOpsHandler{store: store}},
	}
}
