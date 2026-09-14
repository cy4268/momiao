package platform

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

var (
	ErrOpsForbidden          = errors.New("OPS_FORBIDDEN")
	ErrOpsAuthorizationStale = errors.New("AUTHORIZATION_STALE")
	ErrOpsInvalid            = errors.New("OPS_INPUT_INVALID")
	ErrOpsNotFound           = errors.New("OPS_OPERATION_NOT_FOUND")
	ErrOpsConflict           = errors.New("OPS_OPERATION_CONFLICT")
	ErrOpsPreviewStale       = errors.New("PREVIEW_STALE")
	ErrOpsConfirmation       = errors.New("OPS_CONFIRMATION_REQUIRED")
	ErrOpsFreshRequired      = errors.New("FRESH_AUTH_REQUIRED")
	ErrOpsEnvironment        = errors.New("OPERATION_ENVIRONMENT_MISMATCH")
	ErrOpsUnavailable        = errors.New("OPS_CAPABILITY_UNAVAILABLE")
	ErrOpsAuditRejected      = errors.New("AUDIT_SERIALIZATION_REJECTED")
	ErrOpsLastSuperAdmin     = errors.New("LAST_SUPER_ADMIN_REQUIRED")
)

const OpsRegistryVersion = "chaldea-ops-v1"

var opsScopes = []string{
	"ANNOUNCEMENTS", "GAMES", "MODELS", "POKER", "RANKINGS", "RECORDS", "REWARDS", "USERS_IDENTITY",
}

type OpsPermissionDefinition struct {
	Key      string `json:"key"`
	Scope    string `json:"scope,omitempty"`
	ReadOnly bool   `json:"read_only"`
	Operator bool   `json:"operator"`
}

// This is the only permission authority. Database strings can select a fixed
// operator scope, but cannot create a new executable permission.
var opsPermissionCatalog = []OpsPermissionDefinition{
	{Key: "models.read", Scope: "MODELS", ReadOnly: true, Operator: true},
	{Key: "models.metadata.write", Scope: "MODELS", Operator: true},
	{Key: "models.publish", Scope: "MODELS", Operator: true},
	{Key: "users.read", Scope: "USERS_IDENTITY", ReadOnly: true, Operator: true},
	{Key: "users.master.moderate", Scope: "USERS_IDENTITY", Operator: true},
	{Key: "users.support.write", Scope: "USERS_IDENTITY", Operator: true},
	{Key: "games.read", Scope: "GAMES", ReadOnly: true, Operator: true},
	{Key: "games.metadata.write", Scope: "GAMES", Operator: true},
	{Key: "games.runtime.write", Scope: "GAMES", Operator: true},
	{Key: "games.config.draft", Scope: "GAMES", Operator: true},
	{Key: "games.config.validate", Scope: "GAMES", Operator: true},
	{Key: "games.config.activate", Scope: "GAMES", Operator: true},
	{Key: "poker.read", Scope: "POKER", ReadOnly: true, Operator: true},
	{Key: "poker.accepting_players.write", Scope: "POKER", Operator: true},
	{Key: "poker.recovery.request", Scope: "POKER", Operator: true},
	{Key: "poker.chat.moderate", Scope: "POKER", Operator: true},
	{Key: "poker.emergency_pause", Scope: "POKER"},
	{Key: "rewards.read", Scope: "REWARDS", ReadOnly: true, Operator: true},
	{Key: "rewards.claim.retry", Scope: "REWARDS", Operator: true},
	{Key: "rewards.config.draft", Scope: "REWARDS", Operator: true},
	{Key: "rewards.config.activate", Scope: "REWARDS"},
	{Key: "rankings.read", Scope: "RANKINGS", ReadOnly: true, Operator: true},
	{Key: "rankings.rebuild", Scope: "RANKINGS", Operator: true},
	{Key: "rankings.repair.publish", Scope: "RANKINGS"},
	{Key: "records.read", Scope: "RECORDS", ReadOnly: true, Operator: true},
	{Key: "records.incident.create", Scope: "RECORDS", Operator: true},
	{Key: "support-cases.read", Scope: "RECORDS", ReadOnly: true, Operator: true},
	{Key: "support-cases.write", Scope: "RECORDS", Operator: true},
	{Key: "incidents.read", Scope: "RECORDS", ReadOnly: true, Operator: true},
	{Key: "incidents.write", Scope: "RECORDS", Operator: true},
	{Key: "announcements.read", Scope: "ANNOUNCEMENTS", ReadOnly: true, Operator: true},
	{Key: "announcements.write", Scope: "ANNOUNCEMENTS", Operator: true},
	{Key: "announcements.publish", Scope: "ANNOUNCEMENTS", Operator: true},
	{Key: "economy.read", ReadOnly: true},
	{Key: "economy.reconciliation.write"},
	{Key: "economy.adjust"},
	{Key: "access_control.read", ReadOnly: true},
	{Key: "access_control.write"},
	{Key: "audit.read", ReadOnly: true},
	{Key: "operations.read", ReadOnly: true, Operator: true},
	{Key: "search.read", ReadOnly: true, Operator: true},
	{Key: "service-health.read", ReadOnly: true, Operator: true},
	{Key: "jobs.read", ReadOnly: true, Operator: true},
	{Key: "jobs.control"},
	{Key: "attention.read", ReadOnly: true, Operator: true},
	{Key: "maintenance.read", ReadOnly: true, Operator: true},
	{Key: "maintenance.write"},
}

var opsPermissionAliases = map[string]string{
	"models.write":                "models.metadata.write",
	"operations.health.read":      "service-health.read",
	"operations.jobs.read":        "jobs.read",
	"operations.attention.read":   "attention.read",
	"operations.maintenance.read": "maintenance.read",
}

type OpsPrincipal struct {
	PrincipalID string   `json:"principal_id"`
	UserID      int64    `json:"newapi_user_id,string"`
	Role        string   `json:"base_role"`
	Status      string   `json:"status"`
	Epoch       int64    `json:"authz_epoch,string"`
	Version     int64    `json:"version,string"`
	Scopes      []string `json:"scopes"`
	Permissions []string `json:"permissions"`
}

type OpsBootstrap struct {
	Principal      OpsPrincipal             `json:"principal"`
	RegistryVersion string                  `json:"registry_version"`
	Operations     []OpsOperationDescriptor `json:"operations"`
}

func OpsScopes() []string { return append([]string{}, opsScopes...) }

func OpsPermissions() []OpsPermissionDefinition {
	return append([]OpsPermissionDefinition{}, opsPermissionCatalog...)
}

func canonicalOpsPermission(permission string) (string, bool) {
	if canonical, ok := opsPermissionAliases[permission]; ok {
		permission = canonical
	}
	for _, definition := range opsPermissionCatalog {
		if definition.Key == permission {
			return permission, true
		}
	}
	return "", false
}

func validOpsScope(scope string) bool { return slices.Contains(opsScopes, scope) }

func loadOpsPrincipal(ctx context.Context, q announcementQuerier, userID int64, lock bool) (OpsPrincipal, error) {
	principal := OpsPrincipal{UserID: userID, Scopes: []string{}, Permissions: []string{}}
	if ctx == nil || q == nil || userID <= 0 {
		return principal, ErrOpsForbidden
	}
	query := `SELECT admin_principal_id::text,newapi_user_id,base_role,status,authz_epoch,version
	 FROM ops.admin_principals WHERE newapi_user_id=$1`
	if lock {
		query += " FOR UPDATE"
	}
	err := q.QueryRow(ctx, query, userID).Scan(&principal.PrincipalID, &principal.UserID, &principal.Role,
		&principal.Status, &principal.Epoch, &principal.Version)
	if errors.Is(err, pgx.ErrNoRows) || err == nil && principal.Status != "ACTIVE" {
		return principal, ErrOpsForbidden
	}
	if err != nil {
		return principal, err
	}
	if principal.Role != "SUPER_ADMIN" && principal.Role != "OPERATOR" && principal.Role != "AUDITOR" {
		return principal, ErrOpsForbidden
	}
	rows, err := q.Query(ctx, `SELECT scope FROM ops.admin_principal_scopes
	 WHERE admin_principal_id=$1 ORDER BY scope`, principal.PrincipalID)
	if err != nil {
		return principal, err
	}
	defer rows.Close()
	for rows.Next() {
		var scope string
		if err = rows.Scan(&scope); err != nil {
			return principal, err
		}
		if !validOpsScope(scope) {
			return principal, ErrOpsForbidden
		}
		principal.Scopes = append(principal.Scopes, scope)
	}
	if err = rows.Err(); err != nil {
		return principal, err
	}
	for _, definition := range opsPermissionCatalog {
		allowed := principal.Role == "SUPER_ADMIN" ||
			principal.Role == "AUDITOR" && definition.ReadOnly ||
			principal.Role == "OPERATOR" && definition.Operator &&
				(definition.Scope == "" || slices.Contains(principal.Scopes, definition.Scope))
		if allowed {
			principal.Permissions = append(principal.Permissions, definition.Key)
		}
	}
	return principal, nil
}

func requireOpsPermission(ctx context.Context, q announcementQuerier, userID, expectedEpoch int64, permission string, lock bool) (OpsPrincipal, error) {
	canonical, known := canonicalOpsPermission(permission)
	if !known {
		return OpsPrincipal{}, ErrOpsForbidden
	}
	principal, err := loadOpsPrincipal(ctx, q, userID, lock)
	if err != nil {
		return principal, err
	}
	if expectedEpoch > 0 && principal.Epoch != expectedEpoch {
		return principal, ErrOpsAuthorizationStale
	}
	if !slices.Contains(principal.Permissions, canonical) {
		return principal, ErrOpsForbidden
	}
	return principal, nil
}

func (s *Store) RequireOpsPermission(ctx context.Context, userID, expectedEpoch int64, permission string) (OpsPrincipal, error) {
	if s == nil || s.pool == nil {
		return OpsPrincipal{}, ErrOpsUnavailable
	}
	return requireOpsPermission(ctx, s.pool, userID, expectedEpoch, permission, false)
}

func legacyOpsDomainAuthority(ctx context.Context, q announcementQuerier, userID int64, lock bool, domainScope, permissionPrefix string) (AnnouncementPrincipal, error) {
	if !validOpsScope(domainScope) || strings.ToLower(domainScope) == domainScope {
		return AnnouncementPrincipal{UserID: userID, Permissions: []string{}}, ErrAnnouncementForbidden
	}
	principal, err := requireOpsPermission(ctx, q, userID, 0, permissionPrefix+".read", lock)
	legacy := AnnouncementPrincipal{UserID: userID, Role: principal.Role, Epoch: principal.Epoch, Permissions: []string{}}
	if errors.Is(err, ErrOpsForbidden) {
		return legacy, ErrAnnouncementForbidden
	}
	if err != nil {
		return legacy, err
	}
	for _, permission := range principal.Permissions {
		if strings.HasPrefix(permission, permissionPrefix+".") {
			if permission == "models.metadata.write" {
				permission = "models.write"
			}
			legacy.Permissions = append(legacy.Permissions, permission)
		}
	}
	if len(legacy.Permissions) == 0 {
		return legacy, ErrAnnouncementForbidden
	}
	return legacy, nil
}

type OpsAccessControlInput struct {
	Action       string   `json:"action"`
	NewAPIUserID int64    `json:"newapi_user_id,string,omitempty"`
	BaseRole     string   `json:"base_role,omitempty"`
	Scopes       []string `json:"scopes,omitempty"`
}

type opsAccessPrincipal struct {
	PrincipalID string   `json:"principal_id"`
	UserID      int64    `json:"newapi_user_id,string"`
	Role        string   `json:"base_role"`
	Status      string   `json:"status"`
	Epoch       int64    `json:"authz_epoch,string"`
	Version     int64    `json:"version,string"`
	Scopes      []string `json:"scopes"`
}

type accessControlOpsHandler struct{}

func accessControlOpsBindings() []OpsOperationBinding {
	return []OpsOperationBinding{{
		Descriptor: OpsOperationDescriptor{
			OperationType: "ACCESS_CONTROL_CHANGE", Risk: OpsRiskCritical,
			RequiredPermission: "access_control.write", AllowedRoles: []string{"SUPER_ADMIN"},
			TargetType: "ADMIN_PRINCIPAL", InputSchemaVersion: "access-control.v1",
			ImpactSchemaVersion: "access-control-impact.v1", RequiresReason: true,
			RequiresFreshAuth: true, ConfirmationMode: OpsConfirmTyped, ExecutionMode: OpsSameDatabase,
		},
		Handler: accessControlOpsHandler{},
	}}
}

func validOpsRole(role string) bool {
	return role == "SUPER_ADMIN" || role == "OPERATOR" || role == "AUDITOR"
}

func normalizeAccessInput(input OpsAccessControlInput) (OpsAccessControlInput, error) {
	if input.Scopes == nil {
		input.Scopes = []string{}
	}
	slices.Sort(input.Scopes)
	input.Scopes = slices.Compact(input.Scopes)
	for _, scope := range input.Scopes {
		if !validOpsScope(scope) {
			return input, ErrOpsInvalid
		}
	}
	switch input.Action {
	case "CREATE":
		if input.NewAPIUserID <= 0 || !validOpsRole(input.BaseRole) || input.BaseRole != "OPERATOR" && len(input.Scopes) != 0 {
			return input, ErrOpsInvalid
		}
	case "CHANGE_ROLE":
		if input.NewAPIUserID != 0 || !validOpsRole(input.BaseRole) || len(input.Scopes) != 0 {
			return input, ErrOpsInvalid
		}
	case "CHANGE_SCOPES":
		if input.NewAPIUserID != 0 || input.BaseRole != "" {
			return input, ErrOpsInvalid
		}
	case "DISABLE", "ENABLE":
		if input.NewAPIUserID != 0 || input.BaseRole != "" || len(input.Scopes) != 0 {
			return input, ErrOpsInvalid
		}
	default:
		return input, ErrOpsInvalid
	}
	return input, nil
}

func (accessControlOpsHandler) Canonicalize(raw json.RawMessage) (json.RawMessage, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var input OpsAccessControlInput
	if err := decoder.Decode(&input); err != nil {
		return nil, ErrOpsInvalid
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return nil, ErrOpsInvalid
	}
	input, err := normalizeAccessInput(input)
	if err != nil {
		return nil, err
	}
	canonical, err := json.Marshal(input)
	if err != nil {
		return nil, err
	}
	return canonical, nil
}

func lockAccessControlGuard(ctx context.Context, tx pgx.Tx) error {
	var guard string
	err := tx.QueryRow(ctx, `SELECT guard_key FROM ops.access_control_guards
	 WHERE guard_key='ADMIN_PRINCIPAL_SET' FOR UPDATE`).Scan(&guard)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrOpsUnavailable
	}
	return err
}

func loadAccessPrincipal(ctx context.Context, tx pgx.Tx, principalID string, lock bool) (opsAccessPrincipal, error) {
	var principal opsAccessPrincipal
	query := `SELECT admin_principal_id::text,newapi_user_id,base_role,status,authz_epoch,version
	 FROM ops.admin_principals WHERE admin_principal_id=$1`
	if lock {
		query += " FOR UPDATE"
	}
	err := tx.QueryRow(ctx, query, principalID).Scan(&principal.PrincipalID, &principal.UserID, &principal.Role,
		&principal.Status, &principal.Epoch, &principal.Version)
	if errors.Is(err, pgx.ErrNoRows) {
		return principal, ErrOpsNotFound
	}
	if err != nil {
		return principal, err
	}
	rows, err := tx.Query(ctx, `SELECT scope FROM ops.admin_principal_scopes
	 WHERE admin_principal_id=$1 ORDER BY scope`, principalID)
	if err != nil {
		return principal, err
	}
	defer rows.Close()
	principal.Scopes = []string{}
	for rows.Next() {
		var scope string
		if err = rows.Scan(&scope); err != nil {
			return principal, err
		}
		principal.Scopes = append(principal.Scopes, scope)
	}
	return principal, rows.Err()
}

func accessSnapshot(principal *opsAccessPrincipal) json.RawMessage {
	if principal == nil {
		return json.RawMessage(`{"exists":false}`)
	}
	raw, _ := json.Marshal(struct {
		Exists bool `json:"exists"`
		opsAccessPrincipal
	}{true, *principal})
	return raw
}

func proposedAccessPrincipal(current *opsAccessPrincipal, targetID string, input OpsAccessControlInput) (opsAccessPrincipal, error) {
	if input.Action == "CREATE" {
		version := int64(1 + len(input.Scopes))
		return opsAccessPrincipal{PrincipalID: targetID, UserID: input.NewAPIUserID, Role: input.BaseRole,
			Status: "ACTIVE", Epoch: version, Version: version, Scopes: append([]string{}, input.Scopes...)}, nil
	}
	if current == nil {
		return opsAccessPrincipal{}, ErrOpsNotFound
	}
	next := *current
	next.Scopes = append([]string{}, current.Scopes...)
	switch input.Action {
	case "CHANGE_ROLE":
		if current.Role == input.BaseRole {
			return next, ErrOpsConflict
		}
		next.Role = input.BaseRole
		next.Epoch++
		next.Version++
		if input.BaseRole != "OPERATOR" {
			next.Epoch += int64(len(next.Scopes))
			next.Version += int64(len(next.Scopes))
			next.Scopes = []string{}
		}
	case "CHANGE_SCOPES":
		if current.Role != "OPERATOR" {
			return next, ErrOpsInvalid
		}
		next.Scopes = append([]string{}, input.Scopes...)
		next.Epoch += int64(1 + len(current.Scopes) + len(input.Scopes))
		next.Version += int64(1 + len(current.Scopes) + len(input.Scopes))
	case "DISABLE":
		if current.Status != "ACTIVE" {
			return next, ErrOpsConflict
		}
		next.Status = "DISABLED"
		next.Epoch++
		next.Version++
	case "ENABLE":
		if current.Status != "DISABLED" {
			return next, ErrOpsConflict
		}
		next.Status = "ACTIVE"
		next.Epoch++
		next.Version++
	}
	return next, nil
}

func decodeAccessInput(raw json.RawMessage) (OpsAccessControlInput, error) {
	var input OpsAccessControlInput
	if err := json.Unmarshal(raw, &input); err != nil {
		return input, ErrOpsInvalid
	}
	return normalizeAccessInput(input)
}

func ensureLastSuperAdmin(ctx context.Context, tx pgx.Tx, current opsAccessPrincipal, next opsAccessPrincipal) error {
	removing := current.Role == "SUPER_ADMIN" && current.Status == "ACTIVE" &&
		(next.Role != "SUPER_ADMIN" || next.Status != "ACTIVE")
	if !removing {
		return nil
	}
	var count int64
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM ops.admin_principals
	 WHERE base_role='SUPER_ADMIN' AND status='ACTIVE'`).Scan(&count); err != nil {
		return err
	}
	if count <= 1 {
		return ErrOpsLastSuperAdmin
	}
	return nil
}

func (accessControlOpsHandler) Prepare(ctx context.Context, tx pgx.Tx, _ OpsPrincipal, request OpsPrepareRequest, lock bool) (OpsPreparedMaterial, error) {
	if err := lockAccessControlGuard(ctx, tx); err != nil {
		return OpsPreparedMaterial{}, err
	}
	input, err := decodeAccessInput(request.Input)
	if err != nil {
		return OpsPreparedMaterial{}, err
	}
	var current *opsAccessPrincipal
	version := "1"
	if input.Action == "CREATE" {
		if !opsUUIDPattern.MatchString(request.Target.ID) || request.Target.ExpectedVersion != "1" {
			return OpsPreparedMaterial{}, ErrOpsInvalid
		}
		var exists bool
		if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM ops.admin_principals
		 WHERE admin_principal_id=$1 OR newapi_user_id=$2)`, request.Target.ID, input.NewAPIUserID).Scan(&exists); err != nil {
			return OpsPreparedMaterial{}, err
		}
		if exists {
			return OpsPreparedMaterial{}, ErrOpsConflict
		}
		if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM identity.account_refs WHERE newapi_user_id=$1)`, input.NewAPIUserID).Scan(&exists); err != nil {
			return OpsPreparedMaterial{}, err
		}
		if !exists {
			return OpsPreparedMaterial{}, ErrOpsNotFound
		}
	} else {
		loaded, loadErr := loadAccessPrincipal(ctx, tx, request.Target.ID, lock)
		if loadErr != nil {
			return OpsPreparedMaterial{}, loadErr
		}
		current = &loaded
		version = strconv.FormatInt(loaded.Version, 10)
		if request.Target.ExpectedVersion != version {
			return OpsPreparedMaterial{}, ErrOpsPreviewStale
		}
	}
	next, err := proposedAccessPrincipal(current, request.Target.ID, input)
	if err != nil {
		return OpsPreparedMaterial{}, err
	}
	if current != nil {
		if err = ensureLastSuperAdmin(ctx, tx, *current, next); err != nil {
			return OpsPreparedMaterial{}, err
		}
	}
	impact := OpsImpact{CurrentState: accessSnapshot(current), ProposedChange: accessSnapshot(&next),
		BlockingFacts: []string{}, ContinuingAcceptedWork: []string{}, RelatedIDs: []string{request.Target.ID},
		UnavailableMeasurements: []string{}}
	return OpsPreparedMaterial{Impact: impact, TargetVersion: version, TargetLocator: request.Target.ID}, nil
}

func insertAccessHistory(ctx context.Context, tx pgx.Tx, operation OpsOperation, actor OpsPrincipal,
	action string, before *opsAccessPrincipal, after opsAccessPrincipal) error {
	historyID, err := uuidV7()
	if err != nil {
		return err
	}
	var previousRole, previousStatus any
	if before != nil {
		previousRole, previousStatus = before.Role, before.Status
	}
	_, err = tx.Exec(ctx, `INSERT INTO ops.admin_role_history(
	 role_history_id,admin_principal_id,newapi_user_id,action,previous_role,next_role,
	 previous_status,next_status,authz_epoch,actor_kind,actor_newapi_user_id,operation_id,created_at)
	 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,'ADMIN',$10,$11,$12)`, historyID, after.PrincipalID, after.UserID,
		action, previousRole, after.Role, previousStatus, after.Status, after.Epoch, actor.UserID, operation.OperationID, time.Now().UTC())
	return err
}

func (accessControlOpsHandler) Execute(ctx context.Context, tx pgx.Tx, actor OpsPrincipal, operation OpsOperation, _ OpsPreparedMaterial) (OpsExecutionResult, error) {
	input, err := decodeAccessInput(operation.inputPayload)
	if err != nil {
		return OpsExecutionResult{}, err
	}
	var before *opsAccessPrincipal
	if input.Action == "CREATE" {
		_, err = tx.Exec(ctx, `INSERT INTO ops.admin_principals(
		 admin_principal_id,newapi_user_id,base_role,status,created_by,updated_by)
		 VALUES($1,$2,$3,'ACTIVE',$4,$4)`, operation.Target.ID, input.NewAPIUserID, input.BaseRole, actor.UserID)
		if err != nil {
			return OpsExecutionResult{}, announcementDBError(err)
		}
		for _, scope := range input.Scopes {
			if _, err = tx.Exec(ctx, `INSERT INTO ops.admin_principal_scopes(admin_principal_id,scope) VALUES($1,$2)`, operation.Target.ID, scope); err != nil {
				return OpsExecutionResult{}, err
			}
		}
	} else {
		loaded, loadErr := loadAccessPrincipal(ctx, tx, operation.Target.ID, false)
		if loadErr != nil {
			return OpsExecutionResult{}, loadErr
		}
		before = &loaded
		next, nextErr := proposedAccessPrincipal(before, operation.Target.ID, input)
		if nextErr != nil {
			return OpsExecutionResult{}, nextErr
		}
		if err = ensureLastSuperAdmin(ctx, tx, loaded, next); err != nil {
			return OpsExecutionResult{}, err
		}
		switch input.Action {
		case "CHANGE_ROLE":
			_, err = tx.Exec(ctx, `UPDATE ops.admin_principals SET base_role=$2,updated_by=$3
			 WHERE admin_principal_id=$1`, operation.Target.ID, input.BaseRole, actor.UserID)
			if err == nil && input.BaseRole != "OPERATOR" {
				_, err = tx.Exec(ctx, `DELETE FROM ops.admin_principal_scopes WHERE admin_principal_id=$1`, operation.Target.ID)
			}
		case "CHANGE_SCOPES":
			if slices.Equal(loaded.Scopes, input.Scopes) {
				return OpsExecutionResult{}, ErrOpsConflict
			}
			if _, err = tx.Exec(ctx, `UPDATE ops.admin_principals SET updated_by=$2 WHERE admin_principal_id=$1`, operation.Target.ID, actor.UserID); err == nil {
				_, err = tx.Exec(ctx, `DELETE FROM ops.admin_principal_scopes WHERE admin_principal_id=$1`, operation.Target.ID)
			}
			for _, scope := range input.Scopes {
				if err != nil { break }
				_, err = tx.Exec(ctx, `INSERT INTO ops.admin_principal_scopes(admin_principal_id,scope) VALUES($1,$2)`, operation.Target.ID, scope)
			}
		case "DISABLE":
			_, err = tx.Exec(ctx, `UPDATE ops.admin_principals SET status='DISABLED',disabled_at=clock_timestamp(),
			 disabled_by=$2,updated_by=$2 WHERE admin_principal_id=$1`, operation.Target.ID, actor.UserID)
		case "ENABLE":
			_, err = tx.Exec(ctx, `UPDATE ops.admin_principals SET status='ACTIVE',disabled_at=NULL,
			 disabled_by=NULL,updated_by=$2 WHERE admin_principal_id=$1`, operation.Target.ID, actor.UserID)
		}
		if err != nil {
			return OpsExecutionResult{}, err
		}
	}
	after, err := loadAccessPrincipal(ctx, tx, operation.Target.ID, false)
	if err != nil {
		return OpsExecutionResult{}, err
	}
	if err = insertAccessHistory(ctx, tx, operation, actor, input.Action, before, after); err != nil {
		return OpsExecutionResult{}, err
	}
	beforeRaw, afterRaw := accessSnapshot(before), accessSnapshot(&after)
	result, _ := json.Marshal(map[string]any{"principal": after})
	return OpsExecutionResult{BeforeSnapshot: beforeRaw, AfterSnapshot: afterRaw, Result: result,
		RelatedBusinessID: after.PrincipalID}, nil
}
