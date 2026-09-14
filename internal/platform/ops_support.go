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

type OpsUser struct {
	UserID         int64     `json:"newapi_user_id,string"`
	ShortAccountID string    `json:"short_account_id"`
	ProfileStatus  string    `json:"profile_status"`
	DisplayName    string    `json:"display_name"`
	ProfileVersion int64     `json:"profile_version,string"`
	RenameRequired bool      `json:"rename_required"`
	CreatedAt      time.Time `json:"created_at"`
}

const opsUserColumns = `a.newapi_user_id,COALESCE(p.display_name,''),COALESCE(p.profile_version,0),
 COALESCE(p.rename_required,false),a.created_at`

func scanOpsUser(row pgx.Row) (OpsUser, error) {
	var user OpsUser
	err := row.Scan(&user.UserID, &user.DisplayName, &user.ProfileVersion, &user.RenameRequired, &user.CreatedAt)
	if err != nil {
		return user, err
	}
	user.ShortAccountID = ShortAccountID(user.UserID)
	user.ProfileStatus = "INCOMPLETE"
	if user.ProfileVersion > 0 {
		user.ProfileStatus = "COMPLETE"
	}
	user.CreatedAt = user.CreatedAt.UTC()
	return user, nil
}

func (s *Store) OpsUsers(ctx context.Context, actor, epoch int64, query string, limit int) ([]OpsUser, error) {
	users := []OpsUser{}
	if s == nil || s.pool == nil {
		return users, ErrOpsUnavailable
	}
	if !validOpsText(query, 100, false) || limit < 1 || limit > 100 {
		return users, ErrOpsInvalid
	}
	if _, err := s.RequireOpsPermission(ctx, actor, epoch, "users.read"); err != nil {
		return users, err
	}
	rows, err := s.pool.Query(ctx, `SELECT `+opsUserColumns+`
	 FROM identity.account_refs a LEFT JOIN identity.master_profiles p USING(newapi_user_id)
	 WHERE $1='' OR a.newapi_user_id::text=$1 OR
	  'CA-'||upper(substr(encode(sha256(convert_to('chaldea-short-account-id-v1','UTF8')||decode('00','hex')||convert_to(a.newapi_user_id::text,'UTF8')),'hex'),1,12))=upper($1) OR
	  strpos(lower(COALESCE(p.display_name,'')),lower($1))>0
	 ORDER BY a.created_at DESC,a.newapi_user_id DESC LIMIT $2`, query, limit)
	if err != nil {
		return users, err
	}
	defer rows.Close()
	for rows.Next() {
		user, scanErr := scanOpsUser(rows)
		if scanErr != nil {
			return users, scanErr
		}
		users = append(users, user)
	}
	return users, rows.Err()
}

func (s *Store) OpsUser(ctx context.Context, actor, epoch, subject int64) (OpsUser, error) {
	if s == nil || s.pool == nil {
		return OpsUser{}, ErrOpsUnavailable
	}
	if subject <= 0 {
		return OpsUser{}, ErrOpsInvalid
	}
	if _, err := s.RequireOpsPermission(ctx, actor, epoch, "users.read"); err != nil {
		return OpsUser{}, err
	}
	user, err := scanOpsUser(s.pool.QueryRow(ctx, `SELECT `+opsUserColumns+`
	 FROM identity.account_refs a LEFT JOIN identity.master_profiles p USING(newapi_user_id)
	 WHERE a.newapi_user_id=$1`, subject))
	if errors.Is(err, pgx.ErrNoRows) {
		return user, ErrOpsNotFound
	}
	return user, err
}

type OpsSupportCase struct {
	CaseID          string     `json:"case_id"`
	SubjectID       int64      `json:"subject_newapi_user_id,string"`
	Category        string     `json:"category"`
	State           string     `json:"state"`
	SafeSummary     string     `json:"safe_summary"`
	Version         int64      `json:"version,string"`
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
	OperationID   string    `json:"operation_id"`
	RecordedAt    time.Time `json:"recorded_at"`
}

type OpsSupportCaseDetail struct {
	Case              OpsSupportCase               `json:"case"`
	VerificationFacts []OpsSupportVerificationFact `json:"verification_facts"`
}

type OpsSupportCaseQuery struct {
	State     string
	SubjectID int64
	Limit     int
}

const opsSupportColumns = `case_id::text,subject_newapi_user_id,category,state,safe_summary,version,
 created_by,last_operation_id::text,created_at,updated_at,closed_at`

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

func (s *Store) OpsSupportCases(ctx context.Context, actor, epoch int64, query OpsSupportCaseQuery) ([]OpsSupportCase, error) {
	items := []OpsSupportCase{}
	if s == nil || s.pool == nil {
		return items, ErrOpsUnavailable
	}
	if !validSupportState(query.State, true) || query.SubjectID < 0 || query.Limit < 1 || query.Limit > 100 {
		return items, ErrOpsInvalid
	}
	if _, err := s.RequireOpsPermission(ctx, actor, epoch, "support-cases.read"); err != nil {
		return items, err
	}
	rows, err := s.pool.Query(ctx, `SELECT `+opsSupportColumns+` FROM ops.support_cases
	 WHERE ($1='' OR state=$1) AND ($2=0 OR subject_newapi_user_id=$2)
	 ORDER BY updated_at DESC,case_id DESC LIMIT $3`, query.State, query.SubjectID, query.Limit)
	if err != nil {
		return items, err
	}
	defer rows.Close()
	for rows.Next() {
		item, scanErr := scanOpsSupportCase(rows)
		if scanErr != nil {
			return items, scanErr
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) OpsSupportCaseDetail(ctx context.Context, actor, epoch int64, caseID string) (OpsSupportCaseDetail, error) {
	detail := OpsSupportCaseDetail{VerificationFacts: []OpsSupportVerificationFact{}}
	if s == nil || s.pool == nil {
		return detail, ErrOpsUnavailable
	}
	if !opsUUIDPattern.MatchString(caseID) {
		return detail, ErrOpsInvalid
	}
	if _, err := s.RequireOpsPermission(ctx, actor, epoch, "support-cases.read"); err != nil {
		return detail, err
	}
	var err error
	detail.Case, err = scanOpsSupportCase(s.pool.QueryRow(ctx, `SELECT `+opsSupportColumns+`
	 FROM ops.support_cases WHERE case_id=$1`, caseID))
	if errors.Is(err, pgx.ErrNoRows) {
		return detail, ErrOpsNotFound
	}
	if err != nil {
		return detail, err
	}
	rows, err := s.pool.Query(ctx, `SELECT verification_fact_id::text,case_id::text,fact_type,result,
	 COALESCE(safe_reference,''),actor_newapi_user_id,operation_id::text,recorded_at
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

type opsSupportCaseCreateInput struct {
	SubjectID   int64  `json:"subject_newapi_user_id,string"`
	Category    string `json:"category"`
	SafeSummary string `json:"safe_summary"`
}

func normalizeSupportCaseCreate(input opsSupportCaseCreateInput) (opsSupportCaseCreateInput, error) {
	if input.SubjectID <= 0 || !validSupportCategory(input.Category) || !validOpsText(input.SafeSummary, 2048, true) {
		return input, ErrOpsInvalid
	}
	return input, nil
}

type supportCaseCreateOpsHandler struct{ store *Store }

func (supportCaseCreateOpsHandler) Canonicalize(raw json.RawMessage) (json.RawMessage, error) {
	var input opsSupportCaseCreateInput
	if err := decodeOpsInput(raw, &input); err != nil {
		return nil, err
	}
	input, err := normalizeSupportCaseCreate(input)
	if err != nil {
		return nil, err
	}
	return json.Marshal(input)
}

func (handler supportCaseCreateOpsHandler) Prepare(ctx context.Context, tx pgx.Tx, actor OpsPrincipal, request OpsPrepareRequest, lock bool) (OpsPreparedMaterial, error) {
	if handler.store == nil || tx == nil || !opsUUIDPattern.MatchString(request.Target.ID) ||
		request.Target.ExpectedVersion != "0" {
		return OpsPreparedMaterial{}, ErrOpsInvalid
	}
	var input opsSupportCaseCreateInput
	if err := decodeOpsInput(request.Input, &input); err != nil {
		return OpsPreparedMaterial{}, err
	}
	input, err := normalizeSupportCaseCreate(input)
	if err != nil {
		return OpsPreparedMaterial{}, err
	}
	if lock {
		if err = lockIdentity(ctx, tx, "ops-support-case", request.Target.ID); err != nil {
			return OpsPreparedMaterial{}, err
		}
	}
	var accountExists, caseExists bool
	if err = tx.QueryRow(ctx, `SELECT
	 EXISTS(SELECT 1 FROM identity.account_refs WHERE newapi_user_id=$1),
	 EXISTS(SELECT 1 FROM ops.support_cases WHERE case_id=$2)`,
		input.SubjectID, request.Target.ID).Scan(&accountExists, &caseExists); err != nil {
		return OpsPreparedMaterial{}, err
	}
	if !accountExists {
		return OpsPreparedMaterial{}, ErrOpsNotFound
	}
	if caseExists {
		return OpsPreparedMaterial{}, ErrOpsConflict
	}
	proposed := OpsSupportCase{CaseID: request.Target.ID, SubjectID: input.SubjectID, Category: input.Category,
		State: "OPEN", SafeSummary: input.SafeSummary, Version: 1, CreatedBy: actor.UserID}
	impact := OpsImpact{
		CurrentState: rawOpsValue(struct {
			Exists bool `json:"exists"`
		}{false}),
		ProposedChange:          rawOpsValue(proposed),
		AffectedUsers:           affectedOne(),
		AffectedItems:           affectedOne(),
		BlockingFacts:           []string{},
		ContinuingAcceptedWork:  []string{},
		RelatedIDs:              []string{request.Target.ID, strconv.FormatInt(input.SubjectID, 10)},
		UnavailableMeasurements: []string{},
	}
	return OpsPreparedMaterial{Impact: impact, TargetVersion: "0", TargetLocator: request.Target.ID}, nil
}

func (handler supportCaseCreateOpsHandler) Execute(ctx context.Context, tx pgx.Tx, actor OpsPrincipal, operation OpsOperation, _ OpsPreparedMaterial) (OpsExecutionResult, error) {
	var input opsSupportCaseCreateInput
	if err := decodeOpsInput(operation.inputPayload, &input); err != nil {
		return OpsExecutionResult{}, err
	}
	after, err := scanOpsSupportCase(tx.QueryRow(ctx, `INSERT INTO ops.support_cases(
	 case_id,subject_newapi_user_id,category,safe_summary,created_by,last_operation_id)
	 VALUES($1,$2,$3,$4,$5,$6) RETURNING `+opsSupportColumns,
		operation.Target.ID, input.SubjectID, input.Category, input.SafeSummary, actor.UserID, operation.OperationID))
	if err != nil {
		return OpsExecutionResult{}, err
	}
	before := struct {
		Exists bool `json:"exists"`
	}{false}
	result := struct {
		Case OpsSupportCase `json:"case"`
	}{after}
	return OpsExecutionResult{BeforeSnapshot: rawOpsValue(before), AfterSnapshot: rawOpsValue(after),
		Result: rawOpsValue(result), RelatedBusinessID: after.CaseID}, nil
}

type opsSupportTransitionInput struct {
	NextState string `json:"next_state"`
}

type supportCaseTransitionOpsHandler struct{ store *Store }

func (supportCaseTransitionOpsHandler) Canonicalize(raw json.RawMessage) (json.RawMessage, error) {
	var input opsSupportTransitionInput
	if err := decodeOpsInput(raw, &input); err != nil || !validSupportState(input.NextState, false) {
		return nil, ErrOpsInvalid
	}
	return json.Marshal(input)
}

func (handler supportCaseTransitionOpsHandler) Prepare(ctx context.Context, tx pgx.Tx, _ OpsPrincipal, request OpsPrepareRequest, lock bool) (OpsPreparedMaterial, error) {
	if handler.store == nil || tx == nil || !opsUUIDPattern.MatchString(request.Target.ID) {
		return OpsPreparedMaterial{}, ErrOpsInvalid
	}
	var input opsSupportTransitionInput
	if err := decodeOpsInput(request.Input, &input); err != nil || !validSupportState(input.NextState, false) {
		return OpsPreparedMaterial{}, ErrOpsInvalid
	}
	current, err := loadOpsSupportCase(ctx, tx, request.Target.ID, lock)
	if err != nil {
		return OpsPreparedMaterial{}, err
	}
	if request.Target.ExpectedVersion != strconv.FormatInt(current.Version, 10) {
		return OpsPreparedMaterial{}, ErrOpsPreviewStale
	}
	if !supportTransitionAllowed(current.State, input.NextState) || current.Version == math.MaxInt64 {
		return OpsPreparedMaterial{}, ErrOpsConflict
	}
	proposed := struct {
		CaseID  string `json:"case_id"`
		State   string `json:"state"`
		Version int64  `json:"version,string"`
	}{current.CaseID, input.NextState, current.Version + 1}
	impact := OpsImpact{CurrentState: rawOpsValue(current), ProposedChange: rawOpsValue(proposed),
		AffectedUsers: affectedOne(), AffectedItems: affectedOne(), BlockingFacts: []string{},
		ContinuingAcceptedWork: []string{}, RelatedIDs: []string{current.CaseID}, UnavailableMeasurements: []string{}}
	return OpsPreparedMaterial{Impact: impact, TargetVersion: strconv.FormatInt(current.Version, 10),
		TargetLocator: current.CaseID}, nil
}

func (handler supportCaseTransitionOpsHandler) Execute(ctx context.Context, tx pgx.Tx, _ OpsPrincipal, operation OpsOperation, _ OpsPreparedMaterial) (OpsExecutionResult, error) {
	var input opsSupportTransitionInput
	if err := decodeOpsInput(operation.inputPayload, &input); err != nil {
		return OpsExecutionResult{}, err
	}
	before, err := loadOpsSupportCase(ctx, tx, operation.Target.ID, false)
	if err != nil {
		return OpsExecutionResult{}, err
	}
	after, err := scanOpsSupportCase(tx.QueryRow(ctx, `UPDATE ops.support_cases SET state=$2,version=version+1,
	 last_operation_id=$3,updated_at=clock_timestamp(),
	 closed_at=CASE WHEN $2='CLOSED' THEN clock_timestamp() ELSE NULL END
	 WHERE case_id=$1 RETURNING `+opsSupportColumns, operation.Target.ID, input.NextState, operation.OperationID))
	if err != nil {
		return OpsExecutionResult{}, err
	}
	result := struct {
		Case OpsSupportCase `json:"case"`
	}{after}
	return OpsExecutionResult{BeforeSnapshot: rawOpsValue(before), AfterSnapshot: rawOpsValue(after),
		Result: rawOpsValue(result), RelatedBusinessID: after.CaseID}, nil
}

var opsSafeReferencePattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9:._/-]*$`)

type opsSupportVerificationInput struct {
	FactType      string `json:"fact_type"`
	Result        string `json:"result"`
	SafeReference string `json:"safe_reference,omitempty"`
}

func normalizeSupportVerification(input opsSupportVerificationInput) (opsSupportVerificationInput, error) {
	if !containsString([]string{
		"IDENTITY_MATCH", "OWNERSHIP_VERIFIED", "DISCORD_BINDING_VERIFIED", "CONTACT_VERIFIED", "OTHER_SAFE_METADATA",
	}, input.FactType) ||
		!containsString([]string{"VERIFIED", "NOT_VERIFIED", "INCONCLUSIVE"}, input.Result) ||
		!validOpsText(input.SafeReference, 256, false) ||
		input.SafeReference != "" && !opsSafeReferencePattern.MatchString(input.SafeReference) {
		return input, ErrOpsInvalid
	}
	return input, nil
}

type supportVerificationOpsHandler struct{ store *Store }

func (supportVerificationOpsHandler) Canonicalize(raw json.RawMessage) (json.RawMessage, error) {
	var input opsSupportVerificationInput
	if err := decodeOpsInput(raw, &input); err != nil {
		return nil, err
	}
	input, err := normalizeSupportVerification(input)
	if err != nil {
		return nil, err
	}
	return json.Marshal(input)
}

func (handler supportVerificationOpsHandler) Prepare(ctx context.Context, tx pgx.Tx, _ OpsPrincipal, request OpsPrepareRequest, lock bool) (OpsPreparedMaterial, error) {
	if handler.store == nil || tx == nil || !opsUUIDPattern.MatchString(request.Target.ID) {
		return OpsPreparedMaterial{}, ErrOpsInvalid
	}
	var input opsSupportVerificationInput
	if err := decodeOpsInput(request.Input, &input); err != nil {
		return OpsPreparedMaterial{}, err
	}
	input, err := normalizeSupportVerification(input)
	if err != nil {
		return OpsPreparedMaterial{}, err
	}
	current, err := loadOpsSupportCase(ctx, tx, request.Target.ID, lock)
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
		CaseID        string `json:"case_id"`
		FactType      string `json:"fact_type"`
		Result        string `json:"result"`
		SafeReference string `json:"safe_reference,omitempty"`
		NextVersion   int64  `json:"next_version,string"`
	}{current.CaseID, input.FactType, input.Result, input.SafeReference, current.Version + 1}
	impact := OpsImpact{CurrentState: rawOpsValue(current), ProposedChange: rawOpsValue(proposed),
		AffectedUsers: affectedOne(), AffectedItems: affectedOne(), BlockingFacts: []string{},
		ContinuingAcceptedWork: []string{}, RelatedIDs: []string{current.CaseID}, UnavailableMeasurements: []string{}}
	return OpsPreparedMaterial{Impact: impact, TargetVersion: strconv.FormatInt(current.Version, 10),
		TargetLocator: current.CaseID}, nil
}

func (handler supportVerificationOpsHandler) Execute(ctx context.Context, tx pgx.Tx, actor OpsPrincipal, operation OpsOperation, _ OpsPreparedMaterial) (OpsExecutionResult, error) {
	var input opsSupportVerificationInput
	if err := decodeOpsInput(operation.inputPayload, &input); err != nil {
		return OpsExecutionResult{}, err
	}
	before, err := loadOpsSupportCase(ctx, tx, operation.Target.ID, false)
	if err != nil {
		return OpsExecutionResult{}, err
	}
	after, err := scanOpsSupportCase(tx.QueryRow(ctx, `UPDATE ops.support_cases SET version=version+1,
	 last_operation_id=$2,updated_at=clock_timestamp() WHERE case_id=$1 RETURNING `+opsSupportColumns,
		operation.Target.ID, operation.OperationID))
	if err != nil {
		return OpsExecutionResult{}, err
	}
	factID, err := uuidV7()
	if err != nil {
		return OpsExecutionResult{}, err
	}
	var fact OpsSupportVerificationFact
	err = tx.QueryRow(ctx, `INSERT INTO ops.support_verification_facts(
	 verification_fact_id,case_id,fact_type,result,safe_reference,actor_newapi_user_id,operation_id)
	 VALUES($1,$2,$3,$4,NULLIF($5,''),$6,$7)
	 RETURNING verification_fact_id::text,case_id::text,fact_type,result,COALESCE(safe_reference,''),
	  actor_newapi_user_id,operation_id::text,recorded_at`,
		factID, operation.Target.ID, input.FactType, input.Result, input.SafeReference,
		actor.UserID, operation.OperationID).Scan(&fact.FactID, &fact.CaseID, &fact.FactType, &fact.Result,
		&fact.SafeReference, &fact.ActorUserID, &fact.OperationID, &fact.RecordedAt)
	if err != nil {
		return OpsExecutionResult{}, err
	}
	fact.RecordedAt = fact.RecordedAt.UTC()
	result := struct {
		Case             OpsSupportCase             `json:"case"`
		VerificationFact OpsSupportVerificationFact `json:"verification_fact"`
	}{after, fact}
	return OpsExecutionResult{BeforeSnapshot: rawOpsValue(before), AfterSnapshot: rawOpsValue(after),
		Result: rawOpsValue(result), RelatedBusinessID: after.CaseID}, nil
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

// OpsSupportBindings supplies O04 support-case and Master moderation commands.
func OpsSupportBindings(store *Store) []OpsOperationBinding {
	if store == nil {
		return []OpsOperationBinding{}
	}
	return []OpsOperationBinding{
		{Descriptor: OpsOperationDescriptor{OperationType: "SUPPORT_CASE_CREATE", Risk: OpsRiskRoutine,
			RequiredPermission: "support-cases.write", AllowedRoles: []string{"SUPER_ADMIN", "OPERATOR"},
			TargetType: "SUPPORT_CASE", InputSchemaVersion: "support-case-create.v1",
			ImpactSchemaVersion: "support-case-create-impact.v1", RequiresReason: true,
			ConfirmationMode: OpsConfirmNone, ExecutionMode: OpsSameDatabase},
			Handler: supportCaseCreateOpsHandler{store: store}},
		{Descriptor: OpsOperationDescriptor{OperationType: "SUPPORT_CASE_TRANSITION", Risk: OpsRiskImpactful,
			RequiredPermission: "support-cases.write", AllowedRoles: []string{"SUPER_ADMIN", "OPERATOR"},
			TargetType: "SUPPORT_CASE", InputSchemaVersion: "support-case-transition.v1",
			ImpactSchemaVersion: "support-case-transition-impact.v1", RequiresReason: true,
			ConfirmationMode: OpsConfirmExplicit, ExecutionMode: OpsSameDatabase},
			Handler: supportCaseTransitionOpsHandler{store: store}},
		{Descriptor: OpsOperationDescriptor{OperationType: "SUPPORT_VERIFICATION_ADD", Risk: OpsRiskImpactful,
			RequiredPermission: "support-cases.write", AllowedRoles: []string{"SUPER_ADMIN", "OPERATOR"},
			TargetType: "SUPPORT_CASE", InputSchemaVersion: "support-verification-add.v1",
			ImpactSchemaVersion: "support-verification-add-impact.v1", RequiresReason: true,
			ConfirmationMode: OpsConfirmExplicit, ExecutionMode: OpsSameDatabase},
			Handler: supportVerificationOpsHandler{store: store}},
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
