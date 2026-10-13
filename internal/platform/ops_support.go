package platform

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"regexp"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
)

type OpsSupportCase struct {
	CaseID          string     `json:"case_id"`
	SubjectID       int64      `json:"subject_newapi_user_id,string"`
	Category        string     `json:"category"`
	State           string     `json:"state"`
	SafeSummary     string     `json:"safe_summary"`
	Version         int64      `json:"-"`
	CreatedBy       int64      `json:"created_by,string"`
	LastOperationID string     `json:"-"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
	ClosedAt        *time.Time `json:"closed_at,omitempty"`
}

type OpsSupportVerificationFact struct {
	FactID        string    `json:"verification_fact_id"`
	CaseID        string    `json:"case_id"`
	FactType      string    `json:"fact_type"`
	Result        string    `json:"result"`
	SafeReference string    `json:"safe_reference,omitempty"`
	ActorUserID   int64     `json:"actor_newapi_user_id,string"`
	OperationID   string    `json:"-"`
	RecordedAt    time.Time `json:"recorded_at"`
}

type OpsSupportCaseDetail struct {
	Case              OpsSupportCase               `json:"case"`
	VerificationFacts []OpsSupportVerificationFact `json:"verification_facts"`
}

type OpsSupportCaseQuery struct {
	State            string
	SubjectID        int64
	Page             int
	Category, Search string
	From, To         time.Time
}

const opsSupportColumns = `case_id::text,subject_newapi_user_id,category,state,safe_summary,version,
 created_by,COALESCE(last_operation_id::text,''),created_at,updated_at,closed_at`

func scanOpsSupportCase(row pgx.Row) (OpsSupportCase, error) {
	var item OpsSupportCase
	err := row.Scan(&item.CaseID, &item.SubjectID, &item.Category, &item.State, &item.SafeSummary,
		&item.Version, &item.CreatedBy, &item.LastOperationID, &item.CreatedAt, &item.UpdatedAt, &item.ClosedAt)
	if err != nil {
		return item, err
	}
	item.CreatedAt, item.UpdatedAt = item.CreatedAt.UTC(), item.UpdatedAt.UTC()
	if item.ClosedAt != nil {
		value := item.ClosedAt.UTC()
		item.ClosedAt = &value
	}
	return item, nil
}

func validSupportState(value string, allowEmpty bool) bool {
	return allowEmpty && value == "" || containsString([]string{
		"OPEN", "VERIFYING", "APPROVED", "EXECUTED", "REJECTED", "CLOSED",
	}, value)
}

func validSupportCategory(value string) bool {
	return containsString([]string{"ACCOUNT_ACCESS", "IDENTITY", "ECONOMY", "GAMEPLAY", "OTHER"}, value)
}

func supportTransitionAllowed(from, to string) bool {
	switch from {
	case "OPEN":
		return to == "VERIFYING"
	case "VERIFYING":
		return to == "APPROVED" || to == "REJECTED"
	case "APPROVED":
		return to == "EXECUTED"
	case "EXECUTED", "REJECTED":
		return to == "CLOSED"
	default:
		return false
	}
}

type OpsSupportCasePage struct {
	Items    []OpsSupportCase `json:"items"`
	Total    int64            `json:"total"`
	Page     int              `json:"page"`
	PageSize int              `json:"page_size"`
}

func (s *Store) OpsSupportCases(ctx context.Context, query OpsSupportCaseQuery) (OpsSupportCasePage, error) {
	return readOpsSupportCases(ctx, s.pool, query)
}
func readOpsSupportCases(ctx context.Context, q announcementQuerier, query OpsSupportCaseQuery) (OpsSupportCasePage, error) {
	page := OpsSupportCasePage{Items: []OpsSupportCase{}, Page: query.Page, PageSize: 20}
	where := ` FROM ops.support_cases WHERE ($1='' OR state=$1) AND ($2::bigint=0 OR subject_newapi_user_id=$2)
 AND ($3='' OR category=$3) AND ($4='' OR safe_summary ILIKE '%'||$4||'%')
 AND ($5::timestamptz IS NULL OR created_at >= $5) AND ($6::timestamptz IS NULL OR created_at < $6)`
	args := []any{query.State, query.SubjectID, query.Category, query.Search, nullableTime(query.From), nullableTime(query.To)}
	if err := q.QueryRow(ctx, `SELECT count(*)`+where, args...).Scan(&page.Total); err != nil {
		return page, err
	}
	args = append(args, int64(query.Page-1)*20)
	rows, err := q.Query(ctx, `SELECT `+opsSupportColumns+where+` ORDER BY created_at DESC,case_id DESC LIMIT 20 OFFSET $7`, args...)
	if err != nil {
		return page, err
	}
	defer rows.Close()
	for rows.Next() {
		item, e := scanOpsSupportCase(rows)
		if e != nil {
			return page, e
		}
		page.Items = append(page.Items, item)
	}
	return page, rows.Err()
}

func (s *Store) OpsSupportCaseDetail(ctx context.Context, caseID string) (OpsSupportCaseDetail, error) {
	return readOpsSupportCase(ctx, s.pool, caseID)
}
func readOpsSupportCase(ctx context.Context, q announcementQuerier, caseID string) (OpsSupportCaseDetail, error) {
	detail := OpsSupportCaseDetail{VerificationFacts: []OpsSupportVerificationFact{}}
	var err error
	detail.Case, err = scanOpsSupportCase(q.QueryRow(ctx, `SELECT `+opsSupportColumns+`
	 FROM ops.support_cases WHERE case_id=$1`, caseID))
	if errors.Is(err, pgx.ErrNoRows) {
		return detail, ErrOpsNotFound
	}
	if err != nil {
		return detail, err
	}
	rows, err := q.Query(ctx, `SELECT verification_fact_id::text,case_id::text,fact_type,result,
	 COALESCE(safe_reference,''),actor_newapi_user_id,COALESCE(operation_id::text,''),recorded_at
	 FROM ops.support_verification_facts WHERE case_id=$1
	 ORDER BY recorded_at,verification_fact_id`, caseID)
	if err != nil {
		return detail, err
	}
	defer rows.Close()
	for rows.Next() {
		var fact OpsSupportVerificationFact
		if err = rows.Scan(&fact.FactID, &fact.CaseID, &fact.FactType, &fact.Result, &fact.SafeReference,
			&fact.ActorUserID, &fact.OperationID, &fact.RecordedAt); err != nil {
			return detail, err
		}
		fact.RecordedAt = fact.RecordedAt.UTC()
		detail.VerificationFacts = append(detail.VerificationFacts, fact)
	}
	return detail, rows.Err()
}

func loadOpsSupportCase(ctx context.Context, tx pgx.Tx, caseID string, lock bool) (OpsSupportCase, error) {
	query := `SELECT ` + opsSupportColumns + ` FROM ops.support_cases WHERE case_id=$1`
	if lock {
		query += " FOR UPDATE"
	}
	item, err := scanOpsSupportCase(tx.QueryRow(ctx, query, caseID))
	if errors.Is(err, pgx.ErrNoRows) {
		return item, ErrOpsNotFound
	}
	return item, err
}

type OpsSupportInput struct {
	SubjectID     int64  `json:"subject_newapi_user_id,string"`
	Category      string `json:"category"`
	SafeSummary   string `json:"safe_summary"`
	NextState     string `json:"next_state"`
	FactType      string `json:"fact_type"`
	Result        string `json:"result"`
	SafeReference string `json:"safe_reference"`
}

var opsSafeReferencePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9:._/-]*$`)

func (s *Store) SaveOpsSupportCase(ctx context.Context, actor int64, id, action string, input OpsSupportInput) (OpsSupportCase, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return OpsSupportCase{}, err
	}
	defer tx.Rollback(ctx)
	result, err := saveOpsSupportCase(ctx, tx, actor, id, action, input)
	if err != nil {
		return result, err
	}
	return result, tx.Commit(ctx)
}
func saveOpsSupportCase(ctx context.Context, tx pgx.Tx, actor int64, id, action string, input OpsSupportInput) (OpsSupportCase, error) {
	var before, after OpsSupportCase
	switch action {
	case "create":
		if input.SubjectID <= 0 || !validSupportCategory(input.Category) || !validOpsText(input.SafeSummary, 2048, true) {
			return after, ErrOpsInvalid
		}
	case "state":
		if !validSupportState(input.NextState, false) {
			return after, ErrOpsInvalid
		}
	case "facts":
		if !containsString([]string{"IDENTITY_MATCH", "OWNERSHIP_VERIFIED", "DISCORD_BINDING_VERIFIED", "CONTACT_VERIFIED", "OTHER_SAFE_METADATA"}, input.FactType) || !containsString([]string{"VERIFIED", "NOT_VERIFIED", "INCONCLUSIVE"}, input.Result) || !validOpsText(input.SafeReference, 256, false) || input.SafeReference != "" && !opsSafeReferencePattern.MatchString(input.SafeReference) {
			return after, ErrOpsInvalid
		}
	default:
		return after, ErrOpsInvalid
	}
	if err := RequireLocalOpsAdmin(ctx, tx, actor); err != nil {
		return after, err
	}
	var err error
	if action == "create" {
		var exists bool
		if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM identity.users WHERE id=$1 AND deleted_at IS NULL)`, input.SubjectID).Scan(&exists); err != nil {
			return after, err
		}
		if !exists {
			return after, ErrOpsNotFound
		}
		id, err = uuidV7()
		if err != nil {
			return after, err
		}
		after, err = scanOpsSupportCase(tx.QueryRow(ctx, `INSERT INTO ops.support_cases(case_id,subject_newapi_user_id,category,safe_summary,created_by) VALUES($1,$2,$3,$4,$5) RETURNING `+opsSupportColumns, id, input.SubjectID, input.Category, input.SafeSummary, actor))
	} else {
		before, err = loadOpsSupportCase(ctx, tx, id, true)
		if err != nil {
			return after, err
		}
		if before.State == "CLOSED" {
			return after, ErrOpsConflict
		}
		next := before.State
		if action == "state" {
			if input.NextState != before.State && !supportTransitionAllowed(before.State, input.NextState) {
				return after, ErrOpsConflict
			}
			next = input.NextState
		}
		after, err = scanOpsSupportCase(tx.QueryRow(ctx, `UPDATE ops.support_cases SET state=$2,version=version+1,last_operation_id=NULL,updated_at=clock_timestamp(),closed_at=CASE WHEN $2='CLOSED' THEN clock_timestamp() ELSE NULL END WHERE case_id=$1 RETURNING `+opsSupportColumns, id, next))
	}
	if err != nil {
		return after, err
	}
	var auditAfter any = after
	if action == "facts" {
		factID, e := uuidV7()
		if e != nil {
			return after, e
		}
		var fact OpsSupportVerificationFact
		err = tx.QueryRow(ctx, `INSERT INTO ops.support_verification_facts(verification_fact_id,case_id,fact_type,result,safe_reference,actor_newapi_user_id) VALUES($1,$2,$3,$4,NULLIF($5,''),$6) RETURNING verification_fact_id::text,case_id::text,fact_type,result,COALESCE(safe_reference,''),actor_newapi_user_id,recorded_at`, factID, id, input.FactType, input.Result, input.SafeReference, actor).Scan(&fact.FactID, &fact.CaseID, &fact.FactType, &fact.Result, &fact.SafeReference, &fact.ActorUserID, &fact.RecordedAt)
		if err != nil {
			return after, err
		}
		auditAfter = map[string]any{"case": after, "verification_fact": fact}
	}
	actions := map[string]string{"create": "SUPPORT_CASE_CREATE", "state": "SUPPORT_CASE_TRANSITION", "facts": "SUPPORT_VERIFICATION_ADD"}
	err = RecordOpsChange(ctx, tx, actor, actions[action], "SUPPORT_CASE", id, before, auditAfter)
	return after, err
}

type opsMasterSnapshot struct {
	UserID            int64      `json:"newapi_user_id,string"`
	DisplayName       string     `json:"display_name"`
	ProfileVersion    int64      `json:"profile_version,string"`
	RenameRequired    bool       `json:"rename_required"`
	NicknameChangedAt *time.Time `json:"nickname_changed_at"`
}

func opsMasterProfileSnapshot(profile Profile) opsMasterSnapshot {
	return opsMasterSnapshot{
		UserID: profile.UserID, DisplayName: profile.DisplayName, ProfileVersion: profile.ProfileVersion,
		RenameRequired: profile.RenameRequired, NicknameChangedAt: profile.NicknameChangedAt,
	}
}

func loadOpsMasterProfile(ctx context.Context, tx pgx.Tx, userID int64, lock bool) (Profile, error) {
	query := "SELECT " + profileColumns + " FROM identity.master_profiles WHERE newapi_user_id=$1"
	if lock {
		query += " FOR UPDATE"
	}
	profile, err := scanProfile(tx.QueryRow(ctx, query, userID))
	if errors.Is(err, pgx.ErrNoRows) || err == nil && profile.ProfileVersion == 0 {
		return profile, ErrOpsNotFound
	}
	return profile, err
}

func validateOpsMasterTarget(target OpsTarget) (int64, int64, error) {
	userID, err := canonicalTargetUser(target)
	if err != nil {
		return 0, 0, err
	}
	version, err := strconv.ParseInt(target.ExpectedVersion, 10, 64)
	if err != nil || version < 1 || strconv.FormatInt(version, 10) != target.ExpectedVersion {
		return 0, 0, ErrOpsInvalid
	}
	return userID, version, nil
}

type opsMasterRenameRequiredInput struct {
	Required *bool `json:"required"`
}

type masterRenameRequiredOpsHandler struct{ store *Store }

func (masterRenameRequiredOpsHandler) Canonicalize(raw json.RawMessage) (json.RawMessage, error) {
	var input opsMasterRenameRequiredInput
	if err := decodeOpsInput(raw, &input); err != nil || input.Required == nil {
		return nil, ErrOpsInvalid
	}
	return json.Marshal(input)
}

func (handler masterRenameRequiredOpsHandler) Prepare(ctx context.Context, tx pgx.Tx, _ OpsPrincipal, request OpsPrepareRequest, lock bool) (OpsPreparedMaterial, error) {
	if handler.store == nil || tx == nil {
		return OpsPreparedMaterial{}, ErrOpsUnavailable
	}
	userID, expected, err := validateOpsMasterTarget(request.Target)
	if err != nil {
		return OpsPreparedMaterial{}, err
	}
	var input opsMasterRenameRequiredInput
	if err = decodeOpsInput(request.Input, &input); err != nil || input.Required == nil {
		return OpsPreparedMaterial{}, ErrOpsInvalid
	}
	current, err := loadOpsMasterProfile(ctx, tx, userID, lock)
	if err != nil {
		return OpsPreparedMaterial{}, err
	}
	if current.ProfileVersion != expected {
		return OpsPreparedMaterial{}, ErrOpsPreviewStale
	}
	if current.ProfileVersion == math.MaxInt64 || current.RenameRequired == *input.Required {
		return OpsPreparedMaterial{}, ErrOpsConflict
	}
	proposed := opsMasterProfileSnapshot(current)
	proposed.ProfileVersion++
	proposed.RenameRequired = *input.Required
	impact := OpsImpact{CurrentState: rawOpsValue(opsMasterProfileSnapshot(current)),
		ProposedChange: rawOpsValue(proposed), AffectedUsers: affectedOne(), AffectedItems: affectedOne(),
		BlockingFacts: []string{}, ContinuingAcceptedWork: []string{},
		RelatedIDs: []string{request.Target.ID}, UnavailableMeasurements: []string{}}
	return OpsPreparedMaterial{Impact: impact, TargetVersion: strconv.FormatInt(current.ProfileVersion, 10),
		TargetLocator: request.Target.ID}, nil
}

func (handler masterRenameRequiredOpsHandler) Execute(ctx context.Context, tx pgx.Tx, _ OpsPrincipal, operation OpsOperation, _ OpsPreparedMaterial) (OpsExecutionResult, error) {
	userID, _, err := validateOpsMasterTarget(operation.Target)
	if err != nil {
		return OpsExecutionResult{}, err
	}
	var input opsMasterRenameRequiredInput
	if err = decodeOpsInput(operation.inputPayload, &input); err != nil || input.Required == nil {
		return OpsExecutionResult{}, ErrOpsInvalid
	}
	before, err := loadOpsMasterProfile(ctx, tx, userID, false)
	if err != nil {
		return OpsExecutionResult{}, err
	}
	after, err := scanProfile(tx.QueryRow(ctx, `UPDATE identity.master_profiles
	 SET rename_required=$2,profile_version=profile_version+1,updated_at=clock_timestamp()
	 WHERE newapi_user_id=$1 RETURNING `+profileColumns, userID, *input.Required))
	if err != nil {
		return OpsExecutionResult{}, err
	}
	result := struct {
		Profile opsMasterSnapshot `json:"profile"`
	}{opsMasterProfileSnapshot(after)}
	return OpsExecutionResult{BeforeSnapshot: rawOpsValue(opsMasterProfileSnapshot(before)),
		AfterSnapshot: rawOpsValue(opsMasterProfileSnapshot(after)), Result: rawOpsValue(result),
		RelatedBusinessID: operation.Target.ID}, nil
}

type opsMasterForceRenameInput struct {
	DisplayName string `json:"display_name"`
}

type masterForceRenameOpsHandler struct{ store *Store }

func normalizeMasterForceRename(input opsMasterForceRenameInput) (opsMasterForceRenameInput, string, error) {
	display, normalized, err := ValidateNickname(input.DisplayName)
	if err != nil {
		return input, "", ErrOpsInvalid
	}
	input.DisplayName = display
	return input, normalized, nil
}

func (masterForceRenameOpsHandler) Canonicalize(raw json.RawMessage) (json.RawMessage, error) {
	var input opsMasterForceRenameInput
	if err := decodeOpsInput(raw, &input); err != nil {
		return nil, err
	}
	input, _, err := normalizeMasterForceRename(input)
	if err != nil {
		return nil, err
	}
	return json.Marshal(input)
}

func (handler masterForceRenameOpsHandler) Prepare(ctx context.Context, tx pgx.Tx, _ OpsPrincipal, request OpsPrepareRequest, lock bool) (OpsPreparedMaterial, error) {
	if handler.store == nil || tx == nil {
		return OpsPreparedMaterial{}, ErrOpsUnavailable
	}
	userID, expected, err := validateOpsMasterTarget(request.Target)
	if err != nil {
		return OpsPreparedMaterial{}, err
	}
	var input opsMasterForceRenameInput
	if err = decodeOpsInput(request.Input, &input); err != nil {
		return OpsPreparedMaterial{}, err
	}
	input, _, err = normalizeMasterForceRename(input)
	if err != nil {
		return OpsPreparedMaterial{}, err
	}
	current, err := loadOpsMasterProfile(ctx, tx, userID, lock)
	if err != nil {
		return OpsPreparedMaterial{}, err
	}
	if current.ProfileVersion != expected {
		return OpsPreparedMaterial{}, ErrOpsPreviewStale
	}
	if current.ProfileVersion == math.MaxInt64 || current.DisplayName == input.DisplayName {
		return OpsPreparedMaterial{}, ErrOpsConflict
	}
	proposed := opsMasterProfileSnapshot(current)
	proposed.DisplayName, proposed.RenameRequired = input.DisplayName, false
	proposed.ProfileVersion++
	impact := OpsImpact{CurrentState: rawOpsValue(opsMasterProfileSnapshot(current)),
		ProposedChange: rawOpsValue(proposed), AffectedUsers: affectedOne(), AffectedItems: affectedOne(),
		BlockingFacts: []string{}, ContinuingAcceptedWork: []string{},
		RelatedIDs: []string{request.Target.ID}, UnavailableMeasurements: []string{}}
	return OpsPreparedMaterial{Impact: impact, TargetVersion: strconv.FormatInt(current.ProfileVersion, 10),
		TargetLocator: request.Target.ID}, nil
}

func (handler masterForceRenameOpsHandler) Execute(ctx context.Context, tx pgx.Tx, _ OpsPrincipal, operation OpsOperation, _ OpsPreparedMaterial) (OpsExecutionResult, error) {
	userID, _, err := validateOpsMasterTarget(operation.Target)
	if err != nil {
		return OpsExecutionResult{}, err
	}
	var input opsMasterForceRenameInput
	if err = decodeOpsInput(operation.inputPayload, &input); err != nil {
		return OpsExecutionResult{}, err
	}
	input, normalized, err := normalizeMasterForceRename(input)
	if err != nil {
		return OpsExecutionResult{}, err
	}
	before, err := loadOpsMasterProfile(ctx, tx, userID, false)
	if err != nil {
		return OpsExecutionResult{}, err
	}
	after, err := scanProfile(tx.QueryRow(ctx, `UPDATE identity.master_profiles SET
	 display_name=$2,normalized_name=$3,profile_version=profile_version+1,rename_required=false,
	 updated_at=clock_timestamp() WHERE newapi_user_id=$1 RETURNING `+profileColumns,
		userID, input.DisplayName, normalized))
	if err != nil {
		return OpsExecutionResult{}, profileDBError(err)
	}
	if err = appendProfileName(ctx, tx, after, normalized); err != nil {
		return OpsExecutionResult{}, profileDBError(err)
	}
	result := struct {
		Profile opsMasterSnapshot `json:"profile"`
	}{opsMasterProfileSnapshot(after)}
	return OpsExecutionResult{BeforeSnapshot: rawOpsValue(opsMasterProfileSnapshot(before)),
		AfterSnapshot: rawOpsValue(opsMasterProfileSnapshot(after)), Result: rawOpsValue(result),
		RelatedBusinessID: operation.Target.ID}, nil
}

// OpsMasterBindings remains until Master moderation moves to the local user page.
func OpsMasterBindings(store *Store) []OpsOperationBinding {
	if store == nil {
		return []OpsOperationBinding{}
	}
	return []OpsOperationBinding{
		{Descriptor: OpsOperationDescriptor{OperationType: "MASTER_RENAME_REQUIRED", Risk: OpsRiskRoutine,
			RequiredPermission: "users.master.moderate", AllowedRoles: []string{"SUPER_ADMIN", "OPERATOR"},
			TargetType: "MASTER_PROFILE", InputSchemaVersion: "master-rename-required.v1",
			ImpactSchemaVersion: "master-rename-required-impact.v1", RequiresReason: true,
			ConfirmationMode: OpsConfirmNone, ExecutionMode: OpsSameDatabase},
			Handler: masterRenameRequiredOpsHandler{store: store}},
		{Descriptor: OpsOperationDescriptor{OperationType: "MASTER_FORCE_RENAME", Risk: OpsRiskImpactful,
			RequiredPermission: "users.master.moderate", AllowedRoles: []string{"SUPER_ADMIN", "OPERATOR"},
			TargetType: "MASTER_PROFILE", InputSchemaVersion: "master-force-rename.v1",
			ImpactSchemaVersion: "master-force-rename-impact.v1", RequiresReason: true,
			ConfirmationMode: OpsConfirmExplicit, ExecutionMode: OpsSameDatabase},
			Handler: masterForceRenameOpsHandler{store: store}},
	}
}
