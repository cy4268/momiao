package platform

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

type normalizedOpsAudit struct {
	Before json.RawMessage
	After  json.RawMessage
	Result json.RawMessage
}

var forbiddenAuditKeys = map[string]bool{
	"password": true, "password_hash": true, "secret": true, "key_secret": true,
	"api_key_secret": true, "access_token": true, "refresh_token": true, "oauth_secret": true,
	"authorization": true, "cookie": true, "sid": true, "native_sid": true, "proof": true,
	"prompt": true, "response": true, "hole_card": true, "hole_cards": true,
	"server_seed": true, "unrevealed_seed": true, "future_deck": true,
}

func auditValueSafe(value any, depth int) bool {
	if depth > 16 {
		return false
	}
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			normalized := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(key, "-", "_"), " ", "_"))
			if forbiddenAuditKeys[normalized] || strings.HasSuffix(normalized, "_password") ||
				strings.HasSuffix(normalized, "_secret") || strings.HasSuffix(normalized, "_token") {
				return false
			}
			if !auditValueSafe(child, depth+1) {
				return false
			}
		}
	case []any:
		for _, child := range typed {
			if !auditValueSafe(child, depth+1) {
				return false
			}
		}
	}
	return true
}

func canonicalOpsAuditObject(raw json.RawMessage, maximum int) (json.RawMessage, error) {
	canonical, err := canonicalOpsObject(raw, maximum)
	if err != nil {
		return nil, ErrOpsAuditRejected
	}
	var value any
	if err = json.Unmarshal(canonical, &value); err != nil || !auditValueSafe(value, 0) {
		return nil, ErrOpsAuditRejected
	}
	return canonical, nil
}

func normalizeOpsAudit(result OpsExecutionResult) (normalizedOpsAudit, error) {
	before, err := canonicalOpsAuditObject(result.BeforeSnapshot, 32768)
	if err != nil {
		return normalizedOpsAudit{}, err
	}
	after, err := canonicalOpsAuditObject(result.AfterSnapshot, 32768)
	if err != nil {
		return normalizedOpsAudit{}, err
	}
	resultJSON, err := canonicalOpsAuditObject(result.Result, 8192)
	if err != nil {
		return normalizedOpsAudit{}, err
	}
	if !validOpsText(result.RelatedBusinessID, 512, false) {
		return normalizedOpsAudit{}, ErrOpsAuditRejected
	}
	return normalizedOpsAudit{Before: before, After: after, Result: resultJSON}, nil
}

func insertOpsAudit(ctx context.Context, tx pgx.Tx, actor OpsPrincipal, operation OpsOperation,
	audit normalizedOpsAudit, at time.Time) (string, error) {
	auditID, err := uuidV7()
	if err != nil {
		return "", err
	}
	reason := operation.Reason
	if reason == "" {
		reason = "NOT_REQUIRED"
	}
	_, err = tx.Exec(ctx, `INSERT INTO audit.audit_events(
	 audit_id,actor_newapi_user_id,actor_role,actor_scopes_snapshot,action,target_type,target_id,
	 before_snapshot,after_snapshot,reason,operation_id,result,related_business_id,request_id,environment,occurred_at)
	 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,NULLIF($13,''),NULLIF($14,''),$15,$16)`,
		auditID, actor.UserID, actor.Role, actor.Scopes, operation.OperationType, operation.Target.Type,
		operation.Target.ID, audit.Before, audit.After, reason, operation.OperationID, audit.Result,
		operation.RelatedBusinessID, operation.RequestID, operation.Environment, at)
	if err != nil {
		return "", err
	}
	return auditID, nil
}

type OpsAuditEvent struct {
	AuditID             string          `json:"audit_id"`
	ActorUserID          int64           `json:"actor_newapi_user_id,string"`
	ActorRole            string          `json:"actor_role"`
	ActorScopes          []string        `json:"actor_scopes_snapshot"`
	Action               string          `json:"action"`
	TargetType           string          `json:"target_type"`
	TargetID             string          `json:"target_id"`
	BeforeSnapshot       json.RawMessage `json:"before_snapshot"`
	AfterSnapshot        json.RawMessage `json:"after_snapshot"`
	Reason               string          `json:"reason"`
	OperationID          string          `json:"operation_id"`
	Result               json.RawMessage `json:"result"`
	RelatedBusinessID    string          `json:"related_business_id,omitempty"`
	RequestID            string          `json:"request_id,omitempty"`
	Environment          string          `json:"environment"`
	OccurredAt           time.Time       `json:"occurred_at"`
}

type OpsAuditQuery struct {
	OperationID string
	ActorUserID int64
	Action      string
	TargetType  string
	TargetID    string
	Before      time.Time
	Limit       int
}

func scanOpsAudit(row pgx.Row) (OpsAuditEvent, error) {
	var event OpsAuditEvent
	var before, after, result []byte
	err := row.Scan(&event.AuditID, &event.ActorUserID, &event.ActorRole, &event.ActorScopes,
		&event.Action, &event.TargetType, &event.TargetID, &before, &after, &event.Reason,
		&event.OperationID, &result, &event.RelatedBusinessID, &event.RequestID, &event.Environment, &event.OccurredAt)
	event.BeforeSnapshot = append(json.RawMessage{}, before...)
	event.AfterSnapshot = append(json.RawMessage{}, after...)
	event.Result = append(json.RawMessage{}, result...)
	return event, err
}

const opsAuditColumns = `audit_id::text,actor_newapi_user_id,actor_role,actor_scopes_snapshot,
 action,target_type,target_id,before_snapshot,after_snapshot,reason,operation_id::text,result,
 COALESCE(related_business_id,''),COALESCE(request_id,''),environment,occurred_at`

func (s *OpsService) AuditEvent(ctx context.Context, userID, epoch int64, auditID string) (OpsAuditEvent, error) {
	if s == nil || !opsUUIDPattern.MatchString(auditID) {
		return OpsAuditEvent{}, ErrOpsInvalid
	}
	if _, err := requireOpsPermission(ctx, s.store.pool, userID, epoch, "audit.read", false); err != nil {
		return OpsAuditEvent{}, err
	}
	event, err := scanOpsAudit(s.store.pool.QueryRow(ctx, "SELECT "+opsAuditColumns+" FROM audit.audit_events WHERE audit_id=$1", auditID))
	if errors.Is(err, pgx.ErrNoRows) {
		return event, ErrOpsNotFound
	}
	return event, err
}

func (s *OpsService) AuditEvents(ctx context.Context, userID, epoch int64, query OpsAuditQuery) ([]OpsAuditEvent, error) {
	if s == nil || query.Limit < 1 || query.Limit > 100 ||
		query.OperationID != "" && !opsUUIDPattern.MatchString(query.OperationID) ||
		!validOpsText(query.Action, 128, false) || !validOpsText(query.TargetType, 128, false) ||
		!validOpsText(query.TargetID, 512, false) || query.ActorUserID < 0 {
		return nil, ErrOpsInvalid
	}
	if _, err := requireOpsPermission(ctx, s.store.pool, userID, epoch, "audit.read", false); err != nil {
		return nil, err
	}
	rows, err := s.store.pool.Query(ctx, `SELECT `+opsAuditColumns+` FROM audit.audit_events
	 WHERE ($1='' OR operation_id=NULLIF($1,'')::uuid) AND ($2=0 OR actor_newapi_user_id=$2)
	  AND ($3='' OR action=$3) AND ($4='' OR target_type=$4) AND ($5='' OR target_id=$5)
	  AND ($6::timestamptz IS NULL OR occurred_at<$6)
	 ORDER BY occurred_at DESC,audit_id DESC LIMIT $7`, query.OperationID, query.ActorUserID,
		query.Action, query.TargetType, query.TargetID, nullableTime(query.Before), query.Limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	events := []OpsAuditEvent{}
	for rows.Next() {
		event, scanErr := scanOpsAudit(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		events = append(events, event)
	}
	return events, rows.Err()
}

func nullableTime(value time.Time) any {
	if value.IsZero() {
		return nil
	}
	return value
}

type OpsAdminPrincipal struct {
	PrincipalID string     `json:"principal_id"`
	UserID      int64      `json:"newapi_user_id,string"`
	Role        string     `json:"base_role"`
	Status      string     `json:"status"`
	Epoch       int64      `json:"authz_epoch,string"`
	Version     int64      `json:"version,string"`
	Scopes      []string   `json:"scopes"`
	CreatedAt   time.Time  `json:"created_at"`
	CreatedBy   *int64     `json:"created_by,omitempty"`
	UpdatedAt   time.Time  `json:"updated_at"`
	UpdatedBy   *int64     `json:"updated_by,omitempty"`
	DisabledAt  *time.Time `json:"disabled_at,omitempty"`
	DisabledBy  *int64     `json:"disabled_by,omitempty"`
}

func (s *OpsService) AdminPrincipals(ctx context.Context, userID, epoch int64) ([]OpsAdminPrincipal, error) {
	if s == nil {
		return nil, ErrOpsUnavailable
	}
	if _, err := requireOpsPermission(ctx, s.store.pool, userID, epoch, "access_control.read", false); err != nil {
		return nil, err
	}
	rows, err := s.store.pool.Query(ctx, `SELECT p.admin_principal_id::text,p.newapi_user_id,p.base_role,p.status,
	 p.authz_epoch,p.version,p.created_at,p.created_by,p.updated_at,p.updated_by,p.disabled_at,p.disabled_by,
	 COALESCE(array_agg(s.scope ORDER BY s.scope) FILTER (WHERE s.scope IS NOT NULL),ARRAY[]::text[])
	 FROM ops.admin_principals p LEFT JOIN ops.admin_principal_scopes s USING(admin_principal_id)
	 GROUP BY p.admin_principal_id ORDER BY p.status,p.base_role,p.created_at,p.admin_principal_id LIMIT 500`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	principals := []OpsAdminPrincipal{}
	for rows.Next() {
		var principal OpsAdminPrincipal
		if err = rows.Scan(&principal.PrincipalID, &principal.UserID, &principal.Role, &principal.Status,
			&principal.Epoch, &principal.Version, &principal.CreatedAt, &principal.CreatedBy,
			&principal.UpdatedAt, &principal.UpdatedBy, &principal.DisabledAt, &principal.DisabledBy,
			&principal.Scopes); err != nil {
			return nil, err
		}
		principals = append(principals, principal)
	}
	return principals, rows.Err()
}
