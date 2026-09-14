package platform

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
)

type OpsRisk string
type OpsConfirmationMode string
type OpsExecutionMode string

const (
	OpsRiskRoutine   OpsRisk = "LEVEL_1_ROUTINE"
	OpsRiskImpactful OpsRisk = "LEVEL_2_IMPACTFUL"
	OpsRiskCritical  OpsRisk = "LEVEL_3_CRITICAL"

	OpsConfirmNone     OpsConfirmationMode = "NONE"
	OpsConfirmExplicit OpsConfirmationMode = "EXPLICIT"
	OpsConfirmTyped    OpsConfirmationMode = "TYPED"

	OpsSameDatabase  OpsExecutionMode = "SAME_DATABASE"
	OpsDurableRemote OpsExecutionMode = "DURABLE_REMOTE"
)

var (
	opsOperationTypePattern = regexp.MustCompile(`^[A-Z][A-Z0-9_]{0,127}$`)
	opsUUIDPattern          = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
	opsWireNamePattern      = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.:-]{0,127}$`)
)

type OpsTarget struct {
	Type            string `json:"type"`
	ID              string `json:"id"`
	ExpectedVersion string `json:"expected_version"`
}

type OpsPrepareRequest struct {
	OperationID        string          `json:"operation_id"`
	OperationType      string          `json:"operation_type"`
	AuthzEpoch         int64           `json:"authz_epoch,string"`
	InputSchemaVersion string          `json:"input_schema_version"`
	Target             OpsTarget       `json:"target"`
	Input              json.RawMessage `json:"input"`
	Reason             string          `json:"reason"`
	RequestID          string          `json:"request_id,omitempty"`
}

type OpsImpact struct {
	Target                  OpsTarget       `json:"target"`
	Environment             string          `json:"environment"`
	CurrentState            json.RawMessage `json:"current_state"`
	ProposedChange          json.RawMessage `json:"proposed_change"`
	AffectedUsers           *int64          `json:"affected_users,omitempty"`
	AffectedItems           *int64          `json:"affected_items,omitempty"`
	Before                  json.RawMessage `json:"before,omitempty"`
	Delta                   json.RawMessage `json:"delta,omitempty"`
	After                   json.RawMessage `json:"after,omitempty"`
	BlockingFacts           []string        `json:"blocking_facts"`
	ContinuingAcceptedWork  []string        `json:"continuing_accepted_work"`
	RelatedIDs              []string        `json:"related_ids"`
	UnavailableMeasurements []string        `json:"unavailable_measurements"`
}

type OpsPreparedMaterial struct {
	Impact        OpsImpact
	TargetVersion string
	TargetLocator string
}

type OpsExecutionResult struct {
	BeforeSnapshot    json.RawMessage
	AfterSnapshot     json.RawMessage
	Result            json.RawMessage
	RelatedBusinessID string
}

// Implementations must decode Input into a concrete type. Canonicalize is pure.
type OpsOperationHandler interface {
	Canonicalize(json.RawMessage) (json.RawMessage, error)
}

// Same-database handlers prepare and mutate only through the supplied tx.
type OpsSameDatabaseHandler interface {
	OpsOperationHandler
	Prepare(context.Context, pgx.Tx, OpsPrincipal, OpsPrepareRequest, bool) (OpsPreparedMaterial, error)
	Execute(context.Context, pgx.Tx, OpsPrincipal, OpsOperation, OpsPreparedMaterial) (OpsExecutionResult, error)
}

// Accepted SUCCEEDED and NEEDS_REVIEW receipts both carry safe typed execution
// snapshots; the latter may represent a committed remote quarantine action.
type OpsRemoteCompletion struct {
	State       string
	FailureCode string
	Execution   OpsExecutionResult
}

// OpsRemoteFault is returned only after the authenticated private-process
// response or a transport outcome is classified. Unknown errors remain
// retryable by default.
type OpsRemoteFault struct {
	Code        string
	Retryable   bool
	NeedsReview bool
}

func (f *OpsRemoteFault) Error() string {
	if f == nil || f.Code == "" {
		return "OPS_REMOTE_FAILURE"
	}
	return f.Code
}

// Durable remote handlers read without a platform transaction, build a
// canonical outbox command, and dispatch/query it only after platform commit.
type OpsDurableRemoteHandler interface {
	OpsOperationHandler
	PrepareRemote(context.Context, OpsPrincipal, OpsPrepareRequest) (OpsPreparedMaterial, error)
	BuildRemoteCommand(OpsPrincipal, OpsOperation, json.RawMessage, OpsPreparedMaterial) (json.RawMessage, error)
	QueryOperation(context.Context, string, string) (json.RawMessage, bool, error)
	ExecuteOperation(context.Context, string, json.RawMessage) (json.RawMessage, error)
	AcceptRemoteReceipt(OpsOperation, json.RawMessage, json.RawMessage) (OpsRemoteCompletion, error)
}

type OpsOperationDescriptor struct {
	OperationType       string              `json:"operation_type"`
	Risk                OpsRisk             `json:"risk_level"`
	RequiredPermission  string              `json:"required_permission"`
	AllowedRoles        []string            `json:"allowed_roles"`
	TargetType          string              `json:"target_type"`
	InputSchemaVersion  string              `json:"input_schema_version"`
	ImpactSchemaVersion string              `json:"impact_schema_version"`
	RequiresReason      bool                `json:"requires_reason"`
	RequiresFreshAuth   bool                `json:"requires_fresh_auth"`
	ConfirmationMode    OpsConfirmationMode `json:"confirmation_mode"`
	SupportsSchedule    bool                `json:"supports_schedule"`
	ExecutionMode       OpsExecutionMode    `json:"execution_mode"`
	Available           bool                `json:"available"`
}

type OpsOperationBinding struct {
	Descriptor OpsOperationDescriptor
	Handler    OpsOperationHandler
}

type opsRegistryEntry struct {
	descriptor OpsOperationDescriptor
	handler    OpsOperationHandler
}

type OpsRegistry struct{ entries map[string]opsRegistryEntry }

func validateOpsDescriptor(descriptor OpsOperationDescriptor) error {
	permission, known := canonicalOpsPermission(descriptor.RequiredPermission)
	if !opsOperationTypePattern.MatchString(descriptor.OperationType) || !known ||
		permission != descriptor.RequiredPermission || !opsWireNamePattern.MatchString(descriptor.TargetType) ||
		descriptor.InputSchemaVersion == "" || len(descriptor.InputSchemaVersion) > 64 ||
		descriptor.ImpactSchemaVersion == "" || len(descriptor.ImpactSchemaVersion) > 64 ||
		!slices.Contains([]OpsRisk{OpsRiskRoutine, OpsRiskImpactful, OpsRiskCritical}, descriptor.Risk) ||
		!slices.Contains([]OpsConfirmationMode{OpsConfirmNone, OpsConfirmExplicit, OpsConfirmTyped}, descriptor.ConfirmationMode) ||
		!slices.Contains([]OpsExecutionMode{OpsSameDatabase, OpsDurableRemote}, descriptor.ExecutionMode) {
		return ErrOpsInvalid
	}
	if descriptor.Risk == OpsRiskRoutine && (descriptor.ConfirmationMode != OpsConfirmNone || descriptor.RequiresFreshAuth) ||
		descriptor.Risk == OpsRiskImpactful && descriptor.ConfirmationMode != OpsConfirmExplicit ||
		descriptor.Risk == OpsRiskCritical && (!descriptor.RequiresReason || !descriptor.RequiresFreshAuth || descriptor.ConfirmationMode != OpsConfirmTyped) {
		return ErrOpsInvalid
	}
	if len(descriptor.AllowedRoles) == 0 {
		return ErrOpsInvalid
	}
	seen := map[string]bool{}
	for _, role := range descriptor.AllowedRoles {
		if seen[role] || !slices.Contains([]string{"SUPER_ADMIN", "OPERATOR", "AUDITOR"}, role) || role == "AUDITOR" && descriptor.Risk != OpsRiskRoutine {
			return ErrOpsInvalid
		}
		seen[role] = true
	}
	return nil
}

func NewOpsRegistry(bindings ...OpsOperationBinding) (*OpsRegistry, error) {
	registry := &OpsRegistry{entries: map[string]opsRegistryEntry{}}
	all := append(accessControlOpsBindings(), bindings...)
	for _, binding := range all {
		if err := validateOpsDescriptor(binding.Descriptor); err != nil {
			return nil, ErrOpsInvalid
		}
		if binding.Handler != nil {
			_, local := binding.Handler.(OpsSameDatabaseHandler)
			_, remote := binding.Handler.(OpsDurableRemoteHandler)
			if binding.Descriptor.ExecutionMode == OpsSameDatabase && (!local || remote) ||
				binding.Descriptor.ExecutionMode == OpsDurableRemote && (!remote || local) {
				return nil, ErrOpsInvalid
			}
		}
		if _, exists := registry.entries[binding.Descriptor.OperationType]; exists {
			return nil, ErrOpsConflict
		}
		descriptor := binding.Descriptor
		descriptor.AllowedRoles = append([]string{}, descriptor.AllowedRoles...)
		descriptor.Available = binding.Handler != nil
		registry.entries[descriptor.OperationType] = opsRegistryEntry{descriptor: descriptor, handler: binding.Handler}
	}
	return registry, nil
}

func (r *OpsRegistry) Descriptors() []OpsOperationDescriptor {
	if r == nil {
		return []OpsOperationDescriptor{}
	}
	result := make([]OpsOperationDescriptor, 0, len(r.entries))
	for _, entry := range r.entries {
		descriptor := entry.descriptor
		descriptor.AllowedRoles = append([]string{}, descriptor.AllowedRoles...)
		result = append(result, descriptor)
	}
	slices.SortFunc(result, func(a, b OpsOperationDescriptor) int { return strings.Compare(a.OperationType, b.OperationType) })
	return result
}

func (r *OpsRegistry) lookup(operationType string) (opsRegistryEntry, bool) {
	if r == nil {
		return opsRegistryEntry{}, false
	}
	entry, ok := r.entries[operationType]
	return entry, ok
}

type OpsService struct {
	store       *Store
	registry    *OpsRegistry
	environment string
}

func NewOpsService(store *Store, environment string, bindings ...OpsOperationBinding) (*OpsService, error) {
	if store == nil || store.pool == nil || !slices.Contains([]string{"DEVELOPMENT", "STAGING", "PRODUCTION"}, environment) {
		return nil, ErrOpsInvalid
	}
	registry, err := NewOpsRegistry(bindings...)
	if err != nil {
		return nil, err
	}
	return &OpsService{store: store, registry: registry, environment: environment}, nil
}

func (s *OpsService) Bootstrap(ctx context.Context, userID int64) (OpsBootstrap, error) {
	if s == nil || s.store == nil {
		return OpsBootstrap{}, ErrOpsUnavailable
	}
	principal, err := requireOpsPermission(ctx, s.store.pool, userID, 0, "operations.read", false)
	if err != nil {
		return OpsBootstrap{}, err
	}
	operations := []OpsOperationDescriptor{}
	for _, descriptor := range s.registry.Descriptors() {
		if slices.Contains(principal.Permissions, descriptor.RequiredPermission) && slices.Contains(descriptor.AllowedRoles, principal.Role) {
			operations = append(operations, descriptor)
		}
	}
	return OpsBootstrap{Principal: principal, RegistryVersion: OpsRegistryVersion, Operations: operations}, nil
}

type OpsOperation struct {
	OperationID         string    `json:"operation_id"`
	OperationType       string    `json:"operation_type"`
	ActorUserID         int64     `json:"actor_newapi_user_id,string"`
	ActorRole           string    `json:"actor_role_snapshot"`
	ActorScopes         []string  `json:"actor_scopes_snapshot"`
	ActorAuthzEpoch     int64     `json:"actor_authz_epoch_snapshot,string"`
	RequiredPermission  string    `json:"required_permission"`
	Risk                OpsRisk   `json:"risk_level"`
	Target              OpsTarget `json:"target"`
	InputSchemaVersion  string    `json:"input_schema_version"`
	inputPayload        json.RawMessage
	InputHash           string              `json:"input_hash"`
	Environment         string              `json:"environment"`
	ImpactSchemaVersion string              `json:"impact_schema_version"`
	Impact              OpsImpact           `json:"impact_preview"`
	ImpactHash          string              `json:"impact_hash"`
	ConfirmationMode    OpsConfirmationMode `json:"confirmation_mode"`
	RequiresFreshAuth   bool                `json:"requires_fresh_auth"`
	State               string              `json:"state"`
	FailureCode         string              `json:"failure_code,omitempty"`
	StateVersion        int64               `json:"state_version,string"`
	Reason              string              `json:"reason"`
	RequestID           string              `json:"request_id,omitempty"`
	RelatedBusinessID   string              `json:"related_business_id,omitempty"`
	Result              json.RawMessage     `json:"result"`
	CreatedAt           time.Time           `json:"created_at"`
	UpdatedAt           time.Time           `json:"updated_at"`
	AuthorizedAt        *time.Time          `json:"authorized_at,omitempty"`
	ExecutingAt         *time.Time          `json:"executing_at,omitempty"`
	CompletedAt         *time.Time          `json:"completed_at,omitempty"`
	commandHash         string
	challengeHash       []byte
	freshRenewalHash    []byte
	freshRenewalUntil   *time.Time
	freshVerifiedAt     *time.Time
}

// CanonicalInput returns an isolated copy of the input accepted at prepare.
// Domain handlers may decode it but cannot mutate the operation authority.
func (o OpsOperation) CanonicalInput() json.RawMessage {
	return append(json.RawMessage{}, o.inputPayload...)
}

type OpsPrepared struct {
	Operation          OpsOperation           `json:"operation"`
	Descriptor         OpsOperationDescriptor `json:"descriptor"`
	ConfirmationPhrase string                 `json:"confirmation_phrase,omitempty"`
}

// OpsFreshRenewal carries the unchanged, frozen operation preview together
// with a server-only lease that authorizes exactly one replacement challenge.
type OpsFreshRenewal struct {
	OpsPrepared
	Lease string `json:"-"`
}

type OpsFreshEvidence struct {
	OperationID string
	ChallengeID string
	ContextHash string
	Method      string
	VerifiedAt  time.Time
}

type OpsExecuteRequest struct {
	AuthzEpoch        int64             `json:"authz_epoch,string"`
	ImpactHash        string            `json:"impact_hash"`
	Confirmed         bool              `json:"confirmed"`
	TypedConfirmation string            `json:"typed_confirmation,omitempty"`
	Fresh             *OpsFreshEvidence `json:"-"`
}

type OpsOperationEvent struct {
	EventID      string          `json:"event_id"`
	State        string          `json:"state"`
	Substate     string          `json:"substate,omitempty"`
	StateVersion int64           `json:"state_version,string"`
	FailureCode  string          `json:"failure_code,omitempty"`
	SafeDetails  json.RawMessage `json:"safe_details"`
	OccurredAt   time.Time       `json:"occurred_at"`
}

type OpsOperationView struct {
	Operation OpsOperation        `json:"operation"`
	Timeline  []OpsOperationEvent `json:"timeline"`
	AuditID   string              `json:"audit_id,omitempty"`
}

type OpsOperationListQuery struct {
	State         string
	OperationType string
	ActorUserID   int64
	Before        time.Time
	BeforeID      string
	Limit         int
}

type OpsOperationPage struct {
	Items        []OpsOperation `json:"items"`
	NextBefore   *time.Time     `json:"next_before,omitempty"`
	NextBeforeID string         `json:"next_before_id,omitempty"`
}

func canonicalOpsObject(raw json.RawMessage, maximum int) (json.RawMessage, error) {
	if len(raw) == 0 || len(raw) > maximum || !utf8.Valid(raw) {
		return nil, ErrOpsInvalid
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if !uniqueOpsJSON(decoder, 0) {
		return nil, ErrOpsInvalid
	}
	if _, err := decoder.Token(); !errors.Is(err, io.EOF) {
		return nil, ErrOpsInvalid
	}
	decoder = json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, ErrOpsInvalid
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return nil, ErrOpsInvalid
	}
	if _, ok := value.(map[string]any); !ok {
		return nil, ErrOpsInvalid
	}
	canonical, err := json.Marshal(value)
	if err != nil || len(canonical) > maximum {
		return nil, ErrOpsInvalid
	}
	return canonical, nil
}

func uniqueOpsJSON(decoder *json.Decoder, depth int) bool {
	if depth > 16 {
		return false
	}
	token, err := decoder.Token()
	if err != nil {
		return false
	}
	delimiter, ok := token.(json.Delim)
	if !ok {
		return true
	}
	switch delimiter {
	case '{':
		seen := map[string]bool{}
		for decoder.More() {
			name, err := decoder.Token()
			key, ok := name.(string)
			if err != nil || !ok || seen[key] {
				return false
			}
			seen[key] = true
			if !uniqueOpsJSON(decoder, depth+1) {
				return false
			}
		}
		end, err := decoder.Token()
		return err == nil && end == json.Delim('}')
	case '[':
		for decoder.More() {
			if !uniqueOpsJSON(decoder, depth+1) {
				return false
			}
		}
		end, err := decoder.Token()
		return err == nil && end == json.Delim(']')
	default:
		return false
	}
}

func opsDigest(domain string, value any) ([32]byte, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return [32]byte{}, err
	}
	return sha256.Sum256(append(append([]byte(domain), 0), raw...)), nil
}

func validOpsText(value string, maximum int, required bool) bool {
	return (!required && value == "" || value != "") && len(value) <= maximum && utf8.ValidString(value) &&
		strings.TrimSpace(value) == value && !strings.ContainsRune(value, 0)
}

func validOpsOpaque(value string) bool {
	raw, err := base64.RawURLEncoding.DecodeString(value)
	return err == nil && len(raw) == 32 && base64.RawURLEncoding.EncodeToString(raw) == value
}

func validOpsDigest(value string) bool {
	raw, err := hex.DecodeString(value)
	return err == nil && len(raw) == sha256.Size && hex.EncodeToString(raw) == value
}

func maintenanceCreateOperation(operationType string) bool {
	return slices.Contains([]string{
		"MAINTENANCE_START", "MAINTENANCE_SCHEDULE",
		"MAINTENANCE_START_CRITICAL", "MAINTENANCE_SCHEDULE_CRITICAL",
	}, operationType)
}

func zeroVersionOpsOperation(operationType string) bool {
	return maintenanceCreateOperation(operationType) ||
		operationType == "RANKING_REBUILD_CREATE" || operationType == "RANKING_REPAIR_PUBLISH"
}

func validatePrepareRequest(request OpsPrepareRequest, descriptor OpsOperationDescriptor) error {
	if !opsUUIDPattern.MatchString(request.OperationID) || request.OperationType != descriptor.OperationType ||
		request.AuthzEpoch <= 0 || request.InputSchemaVersion != descriptor.InputSchemaVersion ||
		request.Target.Type != descriptor.TargetType || !validOpsText(request.Target.ID, 512, true) ||
		!validOpsText(request.Target.ExpectedVersion, 128, true) ||
		!validOpsText(request.Reason, 2048, descriptor.RequiresReason) || !validOpsText(request.RequestID, 128, false) {
		return ErrOpsInvalid
	}
	if descriptor.RequiresFreshAuth {
		version, err := strconv.ParseInt(request.Target.ExpectedVersion, 10, 64)
		zeroAllowed := zeroVersionOpsOperation(descriptor.OperationType) && version == 0
		if err != nil || version < 0 || version == 0 && !zeroAllowed || strconv.FormatInt(version, 10) != request.Target.ExpectedVersion {
			return ErrOpsInvalid
		}
	}
	return nil
}

func normalizeImpact(impact OpsImpact, target OpsTarget, environment string) (OpsImpact, json.RawMessage, error) {
	impact.Target, impact.Environment = target, environment
	for name, value := range map[string]*json.RawMessage{
		"current": &impact.CurrentState, "proposed": &impact.ProposedChange, "before": &impact.Before,
		"delta": &impact.Delta, "after": &impact.After,
	} {
		if len(*value) == 0 && (name == "current" || name == "proposed") {
			*value = json.RawMessage(`{}`)
		}
		if len(*value) > 0 {
			canonical, err := canonicalOpsObject(*value, 32768)
			if err != nil {
				return impact, nil, err
			}
			*value = canonical
		}
	}
	if impact.BlockingFacts == nil {
		impact.BlockingFacts = []string{}
	}
	if impact.ContinuingAcceptedWork == nil {
		impact.ContinuingAcceptedWork = []string{}
	}
	if impact.RelatedIDs == nil {
		impact.RelatedIDs = []string{}
	}
	if impact.UnavailableMeasurements == nil {
		impact.UnavailableMeasurements = []string{}
	}
	for _, values := range [][]string{impact.BlockingFacts, impact.ContinuingAcceptedWork, impact.RelatedIDs, impact.UnavailableMeasurements} {
		if len(values) > 100 {
			return impact, nil, ErrOpsInvalid
		}
		for _, value := range values {
			if !validOpsText(value, 256, true) {
				return impact, nil, ErrOpsInvalid
			}
		}
	}
	raw, err := json.Marshal(impact)
	if err != nil || len(raw) > 65536 {
		return impact, nil, ErrOpsInvalid
	}
	return impact, raw, nil
}

func confirmationPhrase(environment, operationID, targetType, targetID string) string {
	locator := targetID
	if len(locator) > 64 || !regexp.MustCompile(`^[A-Za-z0-9_.:-]+$`).MatchString(locator) {
		hash := sha256.Sum256([]byte(targetType + "\x00" + targetID))
		locator = "TARGET-" + strings.ToUpper(hex.EncodeToString(hash[:6]))
	}
	return environment + " " + strings.ToUpper(strings.ReplaceAll(operationID[:8], "-", "")) + " " + locator
}

func (s *OpsService) Prepare(ctx context.Context, userID int64, request OpsPrepareRequest) (OpsPrepared, error) {
	if s == nil || s.store == nil || s.registry == nil {
		return OpsPrepared{}, ErrOpsUnavailable
	}
	entry, known := s.registry.lookup(request.OperationType)
	if !known || entry.handler == nil {
		return OpsPrepared{}, ErrOpsUnavailable
	}
	if err := validatePrepareRequest(request, entry.descriptor); err != nil {
		return OpsPrepared{}, err
	}
	validatedInput, err := canonicalOpsObject(request.Input, 32768)
	if err != nil {
		return OpsPrepared{}, err
	}
	canonicalInput, err := entry.handler.Canonicalize(validatedInput)
	if err != nil {
		return OpsPrepared{}, err
	}
	canonicalInput, err = canonicalOpsObject(canonicalInput, 32768)
	if err != nil {
		return OpsPrepared{}, err
	}
	request.Input = canonicalInput
	commandDigest, err := opsDigest("CHALDEA-OPS-COMMAND-V1", struct {
		Actor               int64
		Epoch               int64
		Type                string
		Target              OpsTarget
		Schema              string
		Input               json.RawMessage
		Reason, Environment string
	}{userID, request.AuthzEpoch, request.OperationType, request.Target, request.InputSchemaVersion, canonicalInput, request.Reason, s.environment})
	if err != nil {
		return OpsPrepared{}, err
	}
	commandHash := hex.EncodeToString(commandDigest[:])
	if entry.descriptor.ExecutionMode == OpsDurableRemote {
		remote, ok := entry.handler.(OpsDurableRemoteHandler)
		if !ok {
			return OpsPrepared{}, ErrOpsUnavailable
		}
		return s.prepareDurableRemote(ctx, userID, request, canonicalInput, commandHash, entry, remote)
	}
	local, ok := entry.handler.(OpsSameDatabaseHandler)
	if !ok {
		return OpsPrepared{}, ErrOpsUnavailable
	}
	tx, err := s.store.pool.Begin(ctx)
	if err != nil {
		return OpsPrepared{}, err
	}
	defer rollback(tx)
	if err = lockIdentity(ctx, tx, "ops-operation", request.OperationID); err != nil {
		return OpsPrepared{}, err
	}
	stored, err := loadOpsOperation(ctx, tx, request.OperationID, false)
	if err == nil {
		if stored.ActorUserID != userID || stored.commandHash != commandHash {
			return OpsPrepared{}, ErrOpsConflict
		}
		principal, authorityErr := requireOpsPermission(ctx, tx, userID, request.AuthzEpoch, stored.RequiredPermission, true)
		if authorityErr != nil {
			return OpsPrepared{}, authorityErr
		}
		if principal.Epoch != stored.ActorAuthzEpoch {
			return OpsPrepared{}, ErrOpsAuthorizationStale
		}
		if err = tx.Commit(ctx); err != nil {
			return OpsPrepared{}, err
		}
		return preparedView(stored, entry.descriptor), nil
	}
	if !errors.Is(err, ErrOpsNotFound) {
		return OpsPrepared{}, err
	}
	principal, err := requireOpsPermission(ctx, tx, userID, request.AuthzEpoch, entry.descriptor.RequiredPermission, true)
	if err != nil {
		return OpsPrepared{}, err
	}
	if !slices.Contains(entry.descriptor.AllowedRoles, principal.Role) {
		return OpsPrepared{}, ErrOpsForbidden
	}
	material, err := local.Prepare(ctx, tx, principal, request, true)
	if err != nil {
		return OpsPrepared{}, err
	}
	if material.TargetVersion != request.Target.ExpectedVersion {
		return OpsPrepared{}, ErrOpsPreviewStale
	}
	impact, impactRaw, err := normalizeImpact(material.Impact, request.Target, s.environment)
	if err != nil {
		return OpsPrepared{}, err
	}
	inputDigest := sha256.Sum256(append([]byte("CHALDEA-OPS-INPUT-V1\x00"), canonicalInput...))
	impactDigest := sha256.Sum256(append([]byte("CHALDEA-OPS-IMPACT-V1\x00"), impactRaw...))
	details, _ := json.Marshal(map[string]any{"reason": request.Reason, "request_id": request.RequestID})
	now := time.Now().UTC()
	_, err = tx.Exec(ctx, `INSERT INTO ops.admin_operations(
	 operation_id,actor_kind,newapi_user_id,action,request_hash,details,result,operation_type,
	 actor_role_snapshot,actor_scopes_snapshot,actor_authz_epoch_snapshot,required_permission,risk_level,
	 target_type,target_id,target_version_snapshot,input_schema_version,input_payload,input_hash,environment,
	 impact_schema_version,impact_preview,impact_hash,confirmation_mode,requires_fresh_auth,
	 state,state_version,request_id,created_at,updated_at)
	 VALUES($1,'ADMIN',$2,$3,$4,$5,'{}'::jsonb,$3,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,'PREPARED',1,$23,$24,$24)`,
		request.OperationID, userID, request.OperationType, commandHash, details, principal.Role, principal.Scopes,
		principal.Epoch, entry.descriptor.RequiredPermission, entry.descriptor.Risk, request.Target.Type, request.Target.ID,
		request.Target.ExpectedVersion, request.InputSchemaVersion, canonicalInput, inputDigest[:], s.environment,
		entry.descriptor.ImpactSchemaVersion, impactRaw, impactDigest[:], entry.descriptor.ConfirmationMode,
		entry.descriptor.RequiresFreshAuth, request.RequestID, now)
	if err != nil {
		return OpsPrepared{}, err
	}
	if err = insertOpsEvent(ctx, tx, request.OperationID, "PREPARED", "", 1, "", json.RawMessage(`{}`), now); err != nil {
		return OpsPrepared{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return OpsPrepared{}, err
	}
	operation := OpsOperation{OperationID: request.OperationID, OperationType: request.OperationType, ActorUserID: userID,
		ActorRole: principal.Role, ActorScopes: append([]string{}, principal.Scopes...), ActorAuthzEpoch: principal.Epoch,
		RequiredPermission: entry.descriptor.RequiredPermission, Risk: entry.descriptor.Risk, Target: request.Target,
		InputSchemaVersion: request.InputSchemaVersion, inputPayload: canonicalInput, InputHash: hex.EncodeToString(inputDigest[:]),
		Environment: s.environment, ImpactSchemaVersion: entry.descriptor.ImpactSchemaVersion, Impact: impact,
		ImpactHash: hex.EncodeToString(impactDigest[:]), ConfirmationMode: entry.descriptor.ConfirmationMode,
		RequiresFreshAuth: entry.descriptor.RequiresFreshAuth, State: "PREPARED", StateVersion: 1, Reason: request.Reason,
		RequestID: request.RequestID, Result: json.RawMessage(`{}`), CreatedAt: now, UpdatedAt: now, commandHash: commandHash}
	return preparedView(operation, entry.descriptor), nil
}

func (s *OpsService) prepareDurableRemote(ctx context.Context, userID int64, request OpsPrepareRequest,
	canonicalInput json.RawMessage, commandHash string, entry opsRegistryEntry, remote OpsDurableRemoteHandler) (OpsPrepared, error) {
	// Query the operation authority before the remote preview so replay remains
	// available even while the remote dependency is unavailable.
	tx, err := s.store.pool.Begin(ctx)
	if err != nil {
		return OpsPrepared{}, err
	}
	defer rollback(tx)
	if err = lockIdentity(ctx, tx, "ops-operation", request.OperationID); err != nil {
		return OpsPrepared{}, err
	}
	stored, err := loadOpsOperation(ctx, tx, request.OperationID, false)
	if err == nil {
		if stored.ActorUserID != userID || stored.commandHash != commandHash {
			return OpsPrepared{}, ErrOpsConflict
		}
		principal, authorityErr := requireOpsPermission(ctx, tx, userID, request.AuthzEpoch, stored.RequiredPermission, true)
		if authorityErr != nil {
			return OpsPrepared{}, authorityErr
		}
		if principal.Epoch != stored.ActorAuthzEpoch {
			return OpsPrepared{}, ErrOpsAuthorizationStale
		}
		if err = tx.Commit(ctx); err != nil {
			return OpsPrepared{}, err
		}
		return preparedView(stored, entry.descriptor), nil
	}
	if !errors.Is(err, ErrOpsNotFound) {
		return OpsPrepared{}, err
	}
	principal, err := requireOpsPermission(ctx, tx, userID, request.AuthzEpoch, entry.descriptor.RequiredPermission, true)
	if err != nil {
		return OpsPrepared{}, err
	}
	if !slices.Contains(entry.descriptor.AllowedRoles, principal.Role) {
		return OpsPrepared{}, ErrOpsForbidden
	}
	if err = tx.Commit(ctx); err != nil {
		return OpsPrepared{}, err
	}

	material, err := remote.PrepareRemote(ctx, principal, request)
	if err != nil {
		return OpsPrepared{}, err
	}
	if material.TargetVersion != request.Target.ExpectedVersion {
		return OpsPrepared{}, ErrOpsPreviewStale
	}
	impact, impactRaw, err := normalizeImpact(material.Impact, request.Target, s.environment)
	if err != nil {
		return OpsPrepared{}, err
	}
	inputDigest := sha256.Sum256(append([]byte("CHALDEA-OPS-INPUT-V1\x00"), canonicalInput...))
	impactDigest := sha256.Sum256(append([]byte("CHALDEA-OPS-IMPACT-V1\x00"), impactRaw...))
	details, _ := json.Marshal(map[string]any{"reason": request.Reason, "request_id": request.RequestID})

	tx, err = s.store.pool.Begin(ctx)
	if err != nil {
		return OpsPrepared{}, err
	}
	defer rollback(tx)
	if err = lockIdentity(ctx, tx, "ops-operation", request.OperationID); err != nil {
		return OpsPrepared{}, err
	}
	stored, err = loadOpsOperation(ctx, tx, request.OperationID, false)
	if err == nil {
		if stored.ActorUserID != userID || stored.commandHash != commandHash {
			return OpsPrepared{}, ErrOpsConflict
		}
		current, authorityErr := requireOpsPermission(ctx, tx, userID, request.AuthzEpoch, stored.RequiredPermission, true)
		if authorityErr != nil {
			return OpsPrepared{}, authorityErr
		}
		if current.Epoch != stored.ActorAuthzEpoch {
			return OpsPrepared{}, ErrOpsAuthorizationStale
		}
		if err = tx.Commit(ctx); err != nil {
			return OpsPrepared{}, err
		}
		return preparedView(stored, entry.descriptor), nil
	}
	if !errors.Is(err, ErrOpsNotFound) {
		return OpsPrepared{}, err
	}
	principal, err = requireOpsPermission(ctx, tx, userID, request.AuthzEpoch, entry.descriptor.RequiredPermission, true)
	if err != nil {
		return OpsPrepared{}, err
	}
	if !slices.Contains(entry.descriptor.AllowedRoles, principal.Role) {
		return OpsPrepared{}, ErrOpsForbidden
	}
	now := time.Now().UTC()
	_, err = tx.Exec(ctx, `INSERT INTO ops.admin_operations(
	 operation_id,actor_kind,newapi_user_id,action,request_hash,details,result,operation_type,
	 actor_role_snapshot,actor_scopes_snapshot,actor_authz_epoch_snapshot,required_permission,risk_level,
	 target_type,target_id,target_version_snapshot,input_schema_version,input_payload,input_hash,environment,
	 impact_schema_version,impact_preview,impact_hash,confirmation_mode,requires_fresh_auth,
	 state,state_version,request_id,created_at,updated_at)
	 VALUES($1,'ADMIN',$2,$3,$4,$5,'{}'::jsonb,$3,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,'PREPARED',1,$23,$24,$24)`,
		request.OperationID, userID, request.OperationType, commandHash, details, principal.Role, principal.Scopes,
		principal.Epoch, entry.descriptor.RequiredPermission, entry.descriptor.Risk, request.Target.Type, request.Target.ID,
		request.Target.ExpectedVersion, request.InputSchemaVersion, canonicalInput, inputDigest[:], s.environment,
		entry.descriptor.ImpactSchemaVersion, impactRaw, impactDigest[:], entry.descriptor.ConfirmationMode,
		entry.descriptor.RequiresFreshAuth, request.RequestID, now)
	if err != nil {
		return OpsPrepared{}, err
	}
	if err = insertOpsEvent(ctx, tx, request.OperationID, "PREPARED", "", 1, "", json.RawMessage(`{}`), now); err != nil {
		return OpsPrepared{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return OpsPrepared{}, err
	}
	operation := OpsOperation{OperationID: request.OperationID, OperationType: request.OperationType, ActorUserID: userID,
		ActorRole: principal.Role, ActorScopes: append([]string{}, principal.Scopes...), ActorAuthzEpoch: principal.Epoch,
		RequiredPermission: entry.descriptor.RequiredPermission, Risk: entry.descriptor.Risk, Target: request.Target,
		InputSchemaVersion: request.InputSchemaVersion, inputPayload: canonicalInput, InputHash: hex.EncodeToString(inputDigest[:]),
		Environment: s.environment, ImpactSchemaVersion: entry.descriptor.ImpactSchemaVersion, Impact: impact,
		ImpactHash: hex.EncodeToString(impactDigest[:]), ConfirmationMode: entry.descriptor.ConfirmationMode,
		RequiresFreshAuth: entry.descriptor.RequiresFreshAuth, State: "PREPARED", StateVersion: 1, Reason: request.Reason,
		RequestID: request.RequestID, Result: json.RawMessage(`{}`), CreatedAt: now, UpdatedAt: now, commandHash: commandHash}
	return preparedView(operation, entry.descriptor), nil
}

func preparedView(operation OpsOperation, descriptor OpsOperationDescriptor) OpsPrepared {
	operation.RequiresFreshAuth = descriptor.RequiresFreshAuth
	prepared := OpsPrepared{Operation: operation, Descriptor: descriptor}
	if descriptor.ConfirmationMode == OpsConfirmTyped {
		prepared.ConfirmationPhrase = confirmationPhrase(operation.Environment, operation.OperationID, operation.Target.Type, operation.Target.ID)
	}
	return prepared
}

func newOpsOpaque() (string, error) {
	var raw [32]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw[:]), nil
}

// BeginFreshRenewal invalidates the previously bound challenge before any new
// Native challenge is issued. The short lease prevents concurrent renewals
// from binding a challenge created by a different request.
func (s *OpsService) BeginFreshRenewal(ctx context.Context, userID, epoch int64, operationID string) (OpsFreshRenewal, error) {
	if s == nil || s.store == nil || s.registry == nil || !opsUUIDPattern.MatchString(operationID) || epoch <= 0 {
		return OpsFreshRenewal{}, ErrOpsInvalid
	}
	lease, err := newOpsOpaque()
	if err != nil {
		return OpsFreshRenewal{}, err
	}
	tx, err := s.store.pool.Begin(ctx)
	if err != nil {
		return OpsFreshRenewal{}, err
	}
	defer rollback(tx)
	operation, err := loadOpsOperation(ctx, tx, operationID, true)
	if err != nil {
		return OpsFreshRenewal{}, err
	}
	entry, known := s.registry.lookup(operation.OperationType)
	if !known || entry.handler == nil {
		return OpsFreshRenewal{}, ErrOpsUnavailable
	}
	if operation.ActorUserID != userID || operation.State != "PREPARED" || !operation.RequiresFreshAuth {
		return OpsFreshRenewal{}, ErrOpsConflict
	}
	if operation.Environment != s.environment {
		return OpsFreshRenewal{}, ErrOpsEnvironment
	}
	principal, err := requireOpsPermission(ctx, tx, userID, epoch, operation.RequiredPermission, true)
	if err != nil {
		return OpsFreshRenewal{}, err
	}
	if principal.Epoch != operation.ActorAuthzEpoch {
		return OpsFreshRenewal{}, ErrOpsAuthorizationStale
	}
	now := time.Now().UTC()
	if len(operation.freshRenewalHash) != 0 && operation.freshRenewalUntil != nil && operation.freshRenewalUntil.After(now) {
		return OpsFreshRenewal{}, ErrOpsConflict
	}
	leaseHash := sha256.Sum256([]byte(lease))
	until := now.Add(30 * time.Second)
	version := operation.StateVersion + 1
	if _, err = tx.Exec(ctx, `UPDATE ops.admin_operations SET confirmation_challenge_hash=NULL,
	 fresh_renewal_hash=$2,fresh_renewal_expires_at=$3,state_version=$4,updated_at=$5
	 WHERE operation_id=$1`, operationID, leaseHash[:], until, version, now); err != nil {
		return OpsFreshRenewal{}, err
	}
	if err = insertOpsEvent(ctx, tx, operationID, "PREPARED", "FRESH_CHALLENGE_INVALIDATED", version, "", json.RawMessage(`{}`), now); err != nil {
		return OpsFreshRenewal{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return OpsFreshRenewal{}, err
	}
	operation.challengeHash = nil
	operation.freshRenewalHash = append([]byte{}, leaseHash[:]...)
	operation.freshRenewalUntil = &until
	operation.StateVersion, operation.UpdatedAt = version, now
	return OpsFreshRenewal{OpsPrepared: preparedView(operation, entry.descriptor), Lease: lease}, nil
}

// CompleteFreshRenewal binds only the challenge issued by the holder of the
// active renewal lease. A timed-out or superseded request cannot win later.
func (s *OpsService) CompleteFreshRenewal(ctx context.Context, userID, epoch int64, operationID, lease, challengeID string) (OpsPrepared, error) {
	if s == nil || s.store == nil || s.registry == nil || !opsUUIDPattern.MatchString(operationID) || epoch <= 0 ||
		!validOpsOpaque(lease) || !validOpsOpaque(challengeID) {
		return OpsPrepared{}, ErrOpsInvalid
	}
	tx, err := s.store.pool.Begin(ctx)
	if err != nil {
		return OpsPrepared{}, err
	}
	defer rollback(tx)
	operation, err := loadOpsOperation(ctx, tx, operationID, true)
	if err != nil {
		return OpsPrepared{}, err
	}
	entry, known := s.registry.lookup(operation.OperationType)
	if !known || entry.handler == nil {
		return OpsPrepared{}, ErrOpsUnavailable
	}
	if operation.ActorUserID != userID || operation.State != "PREPARED" || !operation.RequiresFreshAuth {
		return OpsPrepared{}, ErrOpsConflict
	}
	if operation.Environment != s.environment {
		return OpsPrepared{}, ErrOpsEnvironment
	}
	principal, err := requireOpsPermission(ctx, tx, userID, epoch, operation.RequiredPermission, true)
	if err != nil {
		return OpsPrepared{}, err
	}
	if principal.Epoch != operation.ActorAuthzEpoch {
		return OpsPrepared{}, ErrOpsAuthorizationStale
	}
	leaseHash := sha256.Sum256([]byte(lease))
	now := time.Now().UTC()
	if len(operation.challengeHash) != 0 || len(operation.freshRenewalHash) != sha256.Size ||
		!bytes.Equal(operation.freshRenewalHash, leaseHash[:]) || operation.freshRenewalUntil == nil ||
		!operation.freshRenewalUntil.After(now) {
		return OpsPrepared{}, ErrOpsConflict
	}
	challengeHash := sha256.Sum256([]byte(challengeID))
	version := operation.StateVersion + 1
	if _, err = tx.Exec(ctx, `UPDATE ops.admin_operations SET confirmation_challenge_hash=$2,
	 fresh_renewal_hash=NULL,fresh_renewal_expires_at=NULL,state_version=$3,updated_at=$4
	 WHERE operation_id=$1`, operationID, challengeHash[:], version, now); err != nil {
		return OpsPrepared{}, err
	}
	if err = insertOpsEvent(ctx, tx, operationID, "PREPARED", "FRESH_CHALLENGE_RENEWED", version, "", json.RawMessage(`{}`), now); err != nil {
		return OpsPrepared{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return OpsPrepared{}, err
	}
	operation.challengeHash = append([]byte{}, challengeHash[:]...)
	operation.freshRenewalHash, operation.freshRenewalUntil = nil, nil
	operation.StateVersion, operation.UpdatedAt = version, now
	return preparedView(operation, entry.descriptor), nil
}

// AbortFreshRenewal releases the caller's lease after a downstream issuance
// failure. The prior challenge remains invalidated.
func (s *OpsService) AbortFreshRenewal(ctx context.Context, userID int64, operationID, lease string) error {
	if s == nil || s.store == nil || userID <= 0 || !opsUUIDPattern.MatchString(operationID) || !validOpsOpaque(lease) {
		return ErrOpsInvalid
	}
	tx, err := s.store.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer rollback(tx)
	operation, err := loadOpsOperation(ctx, tx, operationID, true)
	if err != nil {
		return err
	}
	leaseHash := sha256.Sum256([]byte(lease))
	if operation.ActorUserID != userID || operation.State != "PREPARED" || len(operation.freshRenewalHash) != sha256.Size ||
		!bytes.Equal(operation.freshRenewalHash, leaseHash[:]) {
		return ErrOpsConflict
	}
	now := time.Now().UTC()
	version := operation.StateVersion + 1
	if _, err = tx.Exec(ctx, `UPDATE ops.admin_operations SET fresh_renewal_hash=NULL,
	 fresh_renewal_expires_at=NULL,state_version=$2,updated_at=$3 WHERE operation_id=$1`, operationID, version, now); err != nil {
		return err
	}
	if err = insertOpsEvent(ctx, tx, operationID, "PREPARED", "FRESH_RENEWAL_ABORTED", version, "", json.RawMessage(`{}`), now); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *OpsService) BindFreshChallenge(ctx context.Context, userID, epoch int64, operationID, challengeID string) error {
	if s == nil || !opsUUIDPattern.MatchString(operationID) || !validOpsOpaque(challengeID) {
		return ErrOpsInvalid
	}
	tx, err := s.store.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer rollback(tx)
	operation, err := loadOpsOperation(ctx, tx, operationID, true)
	if err != nil {
		return err
	}
	if operation.ActorUserID != userID || operation.State != "PREPARED" || !operation.RequiresFreshAuth ||
		len(operation.freshRenewalHash) != 0 {
		return ErrOpsConflict
	}
	principal, err := requireOpsPermission(ctx, tx, userID, epoch, operation.RequiredPermission, true)
	if err != nil {
		return err
	}
	if principal.Epoch != operation.ActorAuthzEpoch {
		return ErrOpsAuthorizationStale
	}
	hash := sha256.Sum256([]byte(challengeID))
	if len(operation.challengeHash) != 0 {
		if bytes.Equal(operation.challengeHash, hash[:]) {
			return tx.Commit(ctx)
		}
		return ErrOpsConflict
	}
	now := time.Now().UTC()
	version := operation.StateVersion + 1
	if _, err = tx.Exec(ctx, `UPDATE ops.admin_operations SET confirmation_challenge_hash=$2,
	 state_version=$3,updated_at=$4 WHERE operation_id=$1`, operationID, hash[:], version, now); err != nil {
		return err
	}
	if err = insertOpsEvent(ctx, tx, operationID, "PREPARED", "FRESH_CHALLENGE_BOUND", version, "", json.RawMessage(`{}`), now); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *OpsService) validateExecution(ctx context.Context, q announcementQuerier, userID int64,
	operation OpsOperation, request OpsExecuteRequest, entry opsRegistryEntry, lock bool) (OpsPrincipal, error) {
	if operation.ActorUserID != userID || operation.State != "PREPARED" {
		return OpsPrincipal{}, ErrOpsConflict
	}
	if operation.Environment != s.environment {
		return OpsPrincipal{}, ErrOpsEnvironment
	}
	principal, err := requireOpsPermission(ctx, q, userID, request.AuthzEpoch, operation.RequiredPermission, lock)
	if err != nil {
		return OpsPrincipal{}, err
	}
	if principal.Epoch != operation.ActorAuthzEpoch {
		return OpsPrincipal{}, ErrOpsAuthorizationStale
	}
	if !slices.Contains(entry.descriptor.AllowedRoles, principal.Role) {
		return OpsPrincipal{}, ErrOpsForbidden
	}
	if request.ImpactHash != operation.ImpactHash {
		return OpsPrincipal{}, ErrOpsPreviewStale
	}
	if entry.descriptor.ConfirmationMode == OpsConfirmExplicit && !request.Confirmed {
		return OpsPrincipal{}, ErrOpsConfirmation
	}
	if entry.descriptor.ConfirmationMode == OpsConfirmTyped && request.TypedConfirmation != confirmationPhrase(operation.Environment, operation.OperationID, operation.Target.Type, operation.Target.ID) {
		return OpsPrincipal{}, ErrOpsConfirmation
	}
	if entry.descriptor.RequiresFreshAuth {
		fresh := request.Fresh
		if fresh == nil || fresh.OperationID != operation.OperationID || !validOpsOpaque(fresh.ChallengeID) ||
			!validOpsDigest(fresh.ContextHash) || !slices.Contains([]string{"password", "discord"}, fresh.Method) ||
			fresh.VerifiedAt.After(time.Now().Add(time.Minute)) || time.Since(fresh.VerifiedAt) > 10*time.Minute {
			return OpsPrincipal{}, ErrOpsFreshRequired
		}
		hash := sha256.Sum256([]byte(fresh.ChallengeID))
		if len(operation.challengeHash) != sha256.Size || !bytes.Equal(operation.challengeHash, hash[:]) {
			return OpsPrincipal{}, ErrOpsFreshRequired
		}
	} else if request.Fresh != nil {
		return OpsPrincipal{}, ErrOpsInvalid
	}
	return principal, nil
}

func (s *OpsService) Execute(ctx context.Context, userID int64, operationID string, request OpsExecuteRequest) (OpsOperationView, error) {
	if s == nil || !opsUUIDPattern.MatchString(operationID) || request.AuthzEpoch <= 0 || !validOpsDigest(request.ImpactHash) {
		return OpsOperationView{}, ErrOpsInvalid
	}
	observed, err := loadOpsOperation(ctx, s.store.pool, operationID, false)
	if err != nil {
		return OpsOperationView{}, err
	}
	entry, known := s.registry.lookup(observed.OperationType)
	if !known || entry.handler == nil {
		return OpsOperationView{}, ErrOpsUnavailable
	}
	if entry.descriptor.ExecutionMode == OpsDurableRemote {
		remote, ok := entry.handler.(OpsDurableRemoteHandler)
		if !ok {
			return OpsOperationView{}, ErrOpsUnavailable
		}
		return s.executeDurableRemote(ctx, userID, operationID, request, observed, entry, remote)
	}
	local, ok := entry.handler.(OpsSameDatabaseHandler)
	if !ok {
		return OpsOperationView{}, ErrOpsUnavailable
	}
	tx, err := s.store.pool.Begin(ctx)
	if err != nil {
		return OpsOperationView{}, err
	}
	defer rollback(tx)
	operation, err := loadOpsOperation(ctx, tx, operationID, true)
	if err != nil {
		return OpsOperationView{}, err
	}
	if operation.OperationType != observed.OperationType {
		return OpsOperationView{}, ErrOpsUnavailable
	}
	principal, err := s.validateExecution(ctx, tx, userID, operation, request, entry, true)
	if err != nil {
		return OpsOperationView{}, err
	}
	prepare := OpsPrepareRequest{OperationID: operation.OperationID, OperationType: operation.OperationType,
		AuthzEpoch: operation.ActorAuthzEpoch, InputSchemaVersion: operation.InputSchemaVersion,
		Target: operation.Target, Input: operation.inputPayload, Reason: operation.Reason, RequestID: operation.RequestID}
	material, err := local.Prepare(ctx, tx, principal, prepare, true)
	if err != nil {
		return OpsOperationView{}, err
	}
	if material.TargetVersion != operation.Target.ExpectedVersion {
		return OpsOperationView{}, ErrOpsPreviewStale
	}
	impact, impactRaw, err := normalizeImpact(material.Impact, operation.Target, s.environment)
	if err != nil {
		return OpsOperationView{}, err
	}
	impactDigest := sha256.Sum256(append([]byte("CHALDEA-OPS-IMPACT-V1\x00"), impactRaw...))
	if hex.EncodeToString(impactDigest[:]) != operation.ImpactHash {
		return OpsOperationView{}, ErrOpsPreviewStale
	}
	now := time.Now().UTC()
	version := operation.StateVersion + 1
	freshAt := any(nil)
	if request.Fresh != nil {
		freshAt = request.Fresh.VerifiedAt.UTC()
	}
	if _, err = tx.Exec(ctx, `UPDATE ops.admin_operations SET state='AUTHORIZED',state_version=$2,
	 authorized_at=$3,fresh_auth_verified_at=$4,updated_at=$3 WHERE operation_id=$1`, operationID, version, now, freshAt); err != nil {
		return OpsOperationView{}, err
	}
	if err = insertOpsEvent(ctx, tx, operationID, "AUTHORIZED", "", version, "", json.RawMessage(`{}`), now); err != nil {
		return OpsOperationView{}, err
	}
	version++
	if _, err = tx.Exec(ctx, `UPDATE ops.admin_operations SET state='EXECUTING',state_version=$2,
	 executing_at=$3,updated_at=$3 WHERE operation_id=$1`, operationID, version, now); err != nil {
		return OpsOperationView{}, err
	}
	if err = insertOpsEvent(ctx, tx, operationID, "EXECUTING", "", version, "", json.RawMessage(`{}`), now); err != nil {
		return OpsOperationView{}, err
	}
	result, err := local.Execute(ctx, tx, principal, operation, OpsPreparedMaterial{Impact: impact, TargetVersion: material.TargetVersion, TargetLocator: material.TargetLocator})
	if err != nil {
		return OpsOperationView{}, err
	}
	audit, err := normalizeOpsAudit(result)
	if err != nil {
		return OpsOperationView{}, err
	}
	operation.RelatedBusinessID = result.RelatedBusinessID
	auditID, err := insertOpsAudit(ctx, tx, principal, operation, audit, now)
	if err != nil {
		return OpsOperationView{}, err
	}
	version++
	if _, err = tx.Exec(ctx, `UPDATE ops.admin_operations SET state='SUCCEEDED',state_version=$2,
	 result=$3,related_business_id=NULLIF($4,''),completed_at=$5,updated_at=$5 WHERE operation_id=$1`,
		operationID, version, audit.Result, result.RelatedBusinessID, now); err != nil {
		return OpsOperationView{}, err
	}
	if err = insertOpsEvent(ctx, tx, operationID, "SUCCEEDED", "", version, "", json.RawMessage(`{}`), now); err != nil {
		return OpsOperationView{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return OpsOperationView{}, err
	}
	operation.State, operation.StateVersion, operation.Result = "SUCCEEDED", version, audit.Result
	operation.AuthorizedAt, operation.ExecutingAt, operation.CompletedAt = &now, &now, &now
	operation.UpdatedAt, operation.RelatedBusinessID = now, result.RelatedBusinessID
	events, _ := s.operationEvents(ctx, operationID)
	return OpsOperationView{Operation: operation, Timeline: events, AuditID: auditID}, nil
}

func (s *OpsService) executeDurableRemote(ctx context.Context, userID int64, operationID string,
	request OpsExecuteRequest, observed OpsOperation, entry opsRegistryEntry, remote OpsDurableRemoteHandler) (OpsOperationView, error) {
	principal, err := s.validateExecution(ctx, s.store.pool, userID, observed, request, entry, false)
	if err != nil {
		return OpsOperationView{}, err
	}
	prepare := OpsPrepareRequest{OperationID: observed.OperationID, OperationType: observed.OperationType,
		AuthzEpoch: observed.ActorAuthzEpoch, InputSchemaVersion: observed.InputSchemaVersion,
		Target: observed.Target, Input: observed.inputPayload, Reason: observed.Reason, RequestID: observed.RequestID}
	material, err := remote.PrepareRemote(ctx, principal, prepare)
	if err != nil {
		return OpsOperationView{}, err
	}
	if material.TargetVersion != observed.Target.ExpectedVersion {
		return OpsOperationView{}, ErrOpsPreviewStale
	}
	impact, impactRaw, err := normalizeImpact(material.Impact, observed.Target, s.environment)
	if err != nil {
		return OpsOperationView{}, err
	}
	impactDigest := sha256.Sum256(append([]byte("CHALDEA-OPS-IMPACT-V1\x00"), impactRaw...))
	if hex.EncodeToString(impactDigest[:]) != observed.ImpactHash {
		return OpsOperationView{}, ErrOpsPreviewStale
	}

	tx, err := s.store.pool.Begin(ctx)
	if err != nil {
		return OpsOperationView{}, err
	}
	defer rollback(tx)
	operation, err := loadOpsOperation(ctx, tx, operationID, true)
	if err != nil {
		return OpsOperationView{}, err
	}
	if operation.OperationType != observed.OperationType || operation.commandHash != observed.commandHash {
		return OpsOperationView{}, ErrOpsConflict
	}
	principal, err = s.validateExecution(ctx, tx, userID, operation, request, entry, true)
	if err != nil {
		return OpsOperationView{}, err
	}
	command, err := remote.BuildRemoteCommand(principal, operation, append(json.RawMessage{}, operation.inputPayload...),
		OpsPreparedMaterial{Impact: impact, TargetVersion: material.TargetVersion, TargetLocator: material.TargetLocator})
	if err != nil {
		return OpsOperationView{}, err
	}
	command, err = canonicalOpsObject(command, 32768)
	if err != nil {
		return OpsOperationView{}, err
	}
	commandDigest := sha256.Sum256(append([]byte("CHALDEA-OPS-REMOTE-COMMAND-V1\x00"), command...))
	now := time.Now().UTC()
	version := operation.StateVersion + 1
	freshAt := any(nil)
	if request.Fresh != nil {
		freshAt = request.Fresh.VerifiedAt.UTC()
	}
	if _, err = tx.Exec(ctx, `UPDATE ops.admin_operations SET state='AUTHORIZED',state_version=$2,
	 authorized_at=$3,fresh_auth_verified_at=$4,updated_at=$3 WHERE operation_id=$1`, operationID, version, now, freshAt); err != nil {
		return OpsOperationView{}, err
	}
	if err = insertOpsEvent(ctx, tx, operationID, "AUTHORIZED", "REMOTE_DISPATCH_ACCEPTED", version, "", json.RawMessage(`{}`), now); err != nil {
		return OpsOperationView{}, err
	}
	version++
	if _, err = tx.Exec(ctx, `UPDATE ops.admin_operations SET state='EXECUTING',state_version=$2,
	 executing_at=$3,updated_at=$3 WHERE operation_id=$1`, operationID, version, now); err != nil {
		return OpsOperationView{}, err
	}
	if err = insertOpsEvent(ctx, tx, operationID, "EXECUTING", "REMOTE_DISPATCH_PENDING", version, "", json.RawMessage(`{}`), now); err != nil {
		return OpsOperationView{}, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO ops.remote_dispatches(
	 operation_id,operation_type,command_payload,command_hash,state,next_attempt_at,created_at,updated_at)
	 VALUES($1,$2,$3,$4,'PENDING',$5,$5,$5)`, operationID, operation.OperationType, command, commandDigest[:], now); err != nil {
		return OpsOperationView{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return OpsOperationView{}, err
	}
	operation.State, operation.StateVersion = "EXECUTING", version
	operation.AuthorizedAt, operation.ExecutingAt = &now, &now
	operation.UpdatedAt = now
	return s.operationView(ctx, operation)
}

// CancelOperation is the durable reset boundary for an operation that has not
// begun. Terminal receipts are returned idempotently; uncertain execution
// states are never rewritten as cancelled.
func (s *OpsService) CancelOperation(ctx context.Context, userID, epoch int64, operationID string) (OpsOperationView, error) {
	if s == nil || s.store == nil || !opsUUIDPattern.MatchString(operationID) || userID <= 0 || epoch <= 0 {
		return OpsOperationView{}, ErrOpsInvalid
	}
	tx, err := s.store.pool.Begin(ctx)
	if err != nil {
		return OpsOperationView{}, err
	}
	defer rollback(tx)
	operation, err := loadOpsOperation(ctx, tx, operationID, true)
	if err != nil {
		return OpsOperationView{}, err
	}
	if operation.ActorUserID != userID {
		return OpsOperationView{}, ErrOpsConflict
	}
	if operation.Environment != s.environment {
		return OpsOperationView{}, ErrOpsEnvironment
	}
	if _, err = requireOpsPermission(ctx, tx, userID, epoch, operation.RequiredPermission, true); err != nil {
		return OpsOperationView{}, err
	}
	switch operation.State {
	case "PREPARED":
		now := time.Now().UTC()
		version := operation.StateVersion + 1
		if _, err = tx.Exec(ctx, `UPDATE ops.admin_operations SET state='CANCELLED',
		 confirmation_challenge_hash=NULL,fresh_renewal_hash=NULL,fresh_renewal_expires_at=NULL,
		 state_version=$2,completed_at=$3,updated_at=$3 WHERE operation_id=$1`, operationID, version, now); err != nil {
			return OpsOperationView{}, err
		}
		if err = insertOpsEvent(ctx, tx, operationID, "CANCELLED", "CANCELLED_BY_ACTOR", version, "", json.RawMessage(`{}`), now); err != nil {
			return OpsOperationView{}, err
		}
		operation.State, operation.StateVersion = "CANCELLED", version
		operation.challengeHash, operation.freshRenewalHash, operation.freshRenewalUntil = nil, nil, nil
		operation.CompletedAt, operation.UpdatedAt = &now, now
	case "CANCELLED", "SUCCEEDED", "FAILED_NO_EFFECT":
		// Stable terminal receipts are safe to return unchanged.
	default:
		return OpsOperationView{}, ErrOpsConflict
	}
	if err = tx.Commit(ctx); err != nil {
		return OpsOperationView{}, err
	}
	return s.operationView(ctx, operation)
}

const opsOperationColumns = `operation_id::text,operation_type,newapi_user_id,actor_role_snapshot,
 actor_scopes_snapshot,actor_authz_epoch_snapshot,required_permission,risk_level,target_type,target_id,
 target_version_snapshot,input_schema_version,input_payload,input_hash,environment,impact_schema_version,
 impact_preview,impact_hash,confirmation_mode,requires_fresh_auth,confirmation_challenge_hash,
 fresh_renewal_hash,fresh_renewal_expires_at,fresh_auth_verified_at,state,
 COALESCE(failure_code,''),state_version,COALESCE(details->>'reason',''),COALESCE(request_id,''),
 COALESCE(related_business_id,''),result,created_at,updated_at,authorized_at,executing_at,completed_at,request_hash`

func loadOpsOperation(ctx context.Context, q announcementQuerier, operationID string, lock bool) (OpsOperation, error) {
	query := "SELECT " + opsOperationColumns + " FROM ops.admin_operations WHERE operation_id=$1 AND operation_type IS NOT NULL"
	if lock {
		query += " FOR UPDATE"
	}
	operation, err := scanLoadedOpsOperation(q.QueryRow(ctx, query, operationID))
	if errors.Is(err, pgx.ErrNoRows) {
		return operation, ErrOpsNotFound
	}
	return operation, err
}

func scanLoadedOpsOperation(row pgx.Row) (OpsOperation, error) {
	var operation OpsOperation
	var input, impact, result []byte
	var inputHash, impactHash, challengeHash []byte
	err := row.Scan(&operation.OperationID, &operation.OperationType, &operation.ActorUserID,
		&operation.ActorRole, &operation.ActorScopes, &operation.ActorAuthzEpoch, &operation.RequiredPermission, &operation.Risk,
		&operation.Target.Type, &operation.Target.ID, &operation.Target.ExpectedVersion, &operation.InputSchemaVersion, &input,
		&inputHash, &operation.Environment, &operation.ImpactSchemaVersion, &impact, &impactHash, &operation.ConfirmationMode,
		&operation.RequiresFreshAuth,
		&challengeHash, &operation.freshRenewalHash, &operation.freshRenewalUntil, &operation.freshVerifiedAt,
		&operation.State, &operation.FailureCode, &operation.StateVersion,
		&operation.Reason, &operation.RequestID, &operation.RelatedBusinessID, &result, &operation.CreatedAt, &operation.UpdatedAt,
		&operation.AuthorizedAt, &operation.ExecutingAt, &operation.CompletedAt, &operation.commandHash)
	if err != nil {
		return operation, err
	}
	operation.inputPayload = append(json.RawMessage{}, input...)
	operation.InputHash, operation.ImpactHash = hex.EncodeToString(inputHash), hex.EncodeToString(impactHash)
	operation.challengeHash = append([]byte{}, challengeHash...)
	operation.Result = append(json.RawMessage{}, result...)
	if err = json.Unmarshal(impact, &operation.Impact); err != nil {
		return operation, err
	}
	return operation, nil
}

func insertOpsEvent(ctx context.Context, tx pgx.Tx, operationID, state, substate string, version int64, failure string, details json.RawMessage, at time.Time) error {
	id, err := uuidV7()
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO ops.admin_operation_events(event_id,operation_id,state,substate,state_version,failure_code,safe_details,occurred_at)
	 VALUES($1,$2,$3,NULLIF($4,''),$5,NULLIF($6,''),$7,$8)`, id, operationID, state, substate, version, failure, details, at)
	return err
}

func (s *OpsService) operationEvents(ctx context.Context, operationID string) ([]OpsOperationEvent, error) {
	rows, err := s.store.pool.Query(ctx, `SELECT event_id::text,state,COALESCE(substate,''),state_version,
	 COALESCE(failure_code,''),safe_details,occurred_at FROM ops.admin_operation_events
	 WHERE operation_id=$1 ORDER BY state_version`, operationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	events := []OpsOperationEvent{}
	for rows.Next() {
		var event OpsOperationEvent
		var details []byte
		if err = rows.Scan(&event.EventID, &event.State, &event.Substate, &event.StateVersion, &event.FailureCode, &details, &event.OccurredAt); err != nil {
			return nil, err
		}
		event.SafeDetails = append(json.RawMessage{}, details...)
		events = append(events, event)
	}
	return events, rows.Err()
}

func (s *OpsService) operationView(ctx context.Context, operation OpsOperation) (OpsOperationView, error) {
	events, err := s.operationEvents(ctx, operation.OperationID)
	if err != nil {
		return OpsOperationView{}, err
	}
	var auditID string
	err = s.store.pool.QueryRow(ctx, "SELECT audit_id::text FROM audit.audit_events WHERE operation_id=$1", operation.OperationID).Scan(&auditID)
	if errors.Is(err, pgx.ErrNoRows) {
		err = nil
	}
	if err != nil {
		return OpsOperationView{}, err
	}
	return OpsOperationView{Operation: operation, Timeline: events, AuditID: auditID}, nil
}

func (s *OpsService) Operation(ctx context.Context, userID, epoch int64, operationID string) (OpsOperationView, error) {
	if s == nil || !opsUUIDPattern.MatchString(operationID) {
		return OpsOperationView{}, ErrOpsInvalid
	}
	operation, err := loadOpsOperation(ctx, s.store.pool, operationID, false)
	if err != nil {
		return OpsOperationView{}, err
	}
	permission := "audit.read"
	if operation.ActorUserID == userID {
		permission = operation.RequiredPermission
	}
	if _, err = requireOpsPermission(ctx, s.store.pool, userID, epoch, permission, false); err != nil {
		return OpsOperationView{}, err
	}
	return s.operationView(ctx, operation)
}

func (s *OpsService) Operations(ctx context.Context, userID, epoch int64, query OpsOperationListQuery) (OpsOperationPage, error) {
	if s == nil || query.Limit < 1 || query.Limit > 100 || query.ActorUserID < 0 ||
		query.OperationType != "" && !opsOperationTypePattern.MatchString(query.OperationType) ||
		query.State != "" && !slices.Contains([]string{"PREPARED", "AUTHORIZED", "EXECUTING", "SUCCEEDED", "FAILED_NO_EFFECT", "RECOVERING", "NEEDS_REVIEW", "CANCELLED"}, query.State) ||
		query.Before.IsZero() != (query.BeforeID == "") || query.BeforeID != "" && !opsUUIDPattern.MatchString(query.BeforeID) {
		return OpsOperationPage{}, ErrOpsInvalid
	}
	if query.OperationType != "" {
		if _, known := s.registry.lookup(query.OperationType); !known {
			return OpsOperationPage{}, ErrOpsUnavailable
		}
	}
	principal, err := requireOpsPermission(ctx, s.store.pool, userID, epoch, "operations.read", false)
	if err != nil {
		return OpsOperationPage{}, err
	}
	global := slices.Contains(principal.Permissions, "audit.read")
	if !global {
		if query.ActorUserID != 0 && query.ActorUserID != userID {
			return OpsOperationPage{}, ErrOpsForbidden
		}
		query.ActorUserID = userID
	}
	rows, err := s.store.pool.Query(ctx, `SELECT `+opsOperationColumns+` FROM ops.admin_operations
	 WHERE operation_type IS NOT NULL AND ($1='' OR state=$1) AND ($2='' OR operation_type=$2)
	  AND ($3=0 OR newapi_user_id=$3)
	  AND ($4::timestamptz IS NULL OR (created_at,operation_id)<($4,NULLIF($5,'')::uuid))
	 ORDER BY created_at DESC,operation_id DESC LIMIT $6`, query.State, query.OperationType, query.ActorUserID,
		nullableTime(query.Before), query.BeforeID, query.Limit)
	if err != nil {
		return OpsOperationPage{}, err
	}
	defer rows.Close()
	page := OpsOperationPage{Items: []OpsOperation{}}
	var scanned int
	var lastScanned OpsOperation
	for rows.Next() {
		operation, scanErr := scanLoadedOpsOperation(rows)
		if scanErr != nil {
			return OpsOperationPage{}, scanErr
		}
		scanned++
		lastScanned = operation
		if global || slices.Contains(principal.Permissions, operation.RequiredPermission) {
			page.Items = append(page.Items, operation)
		}
	}
	if err = rows.Err(); err != nil {
		return OpsOperationPage{}, err
	}
	if scanned == query.Limit {
		at := lastScanned.CreatedAt
		page.NextBefore, page.NextBeforeID = &at, lastScanned.OperationID
	}
	return page, nil
}

type OpsFreshBinding struct {
	OperationID     string
	Action          string
	TargetKind      string
	TargetID        string
	ExpectedVersion string
	CommandDigest   [32]byte
	ImpactDigest    [32]byte
}

func (o OpsOperation) FreshBinding() (OpsFreshBinding, error) {
	var command, impact [32]byte
	commandRaw, err := hex.DecodeString(o.commandHash)
	if err != nil || len(commandRaw) != 32 {
		return OpsFreshBinding{}, ErrOpsInvalid
	}
	impactRaw, err := hex.DecodeString(o.ImpactHash)
	if err != nil || len(impactRaw) != 32 {
		return OpsFreshBinding{}, ErrOpsInvalid
	}
	copy(command[:], commandRaw)
	copy(impact[:], impactRaw)
	targetHash := sha256.Sum256([]byte(o.Target.Type + "\x00" + o.Target.ID))
	action := o.OperationType
	if o.Target.Type == "MAINTENANCE" && maintenanceCreateOperation(o.OperationType) && o.Target.ExpectedVersion == "0" {
		action = "maintenance.create"
	}
	return OpsFreshBinding{OperationID: o.OperationID, Action: action, TargetKind: o.Target.Type,
		TargetID: "target:" + hex.EncodeToString(targetHash[:]), ExpectedVersion: o.Target.ExpectedVersion,
		CommandDigest: command, ImpactDigest: impact}, nil
}

func (o OpsOperation) FreshChallengeBound() bool { return len(o.challengeHash) == sha256.Size }

func (s *OpsService) Descriptor(operationType string) (OpsOperationDescriptor, bool) {
	entry, ok := s.registry.lookup(operationType)
	return entry.descriptor, ok
}

func (s *OpsService) FreshOperation(ctx context.Context, userID, epoch int64, operationID string) (OpsOperation, error) {
	operation, err := loadOpsOperation(ctx, s.store.pool, operationID, false)
	if err != nil {
		return OpsOperation{}, err
	}
	if operation.ActorUserID != userID || operation.State != "PREPARED" || !operation.RequiresFreshAuth {
		return OpsOperation{}, ErrOpsConflict
	}
	principal, err := requireOpsPermission(ctx, s.store.pool, userID, epoch, operation.RequiredPermission, false)
	if err != nil {
		return OpsOperation{}, err
	}
	if principal.Epoch != operation.ActorAuthzEpoch {
		return OpsOperation{}, ErrOpsAuthorizationStale
	}
	return operation, nil
}
