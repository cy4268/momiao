package platform

import (
	"context"
	"encoding/json"
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
