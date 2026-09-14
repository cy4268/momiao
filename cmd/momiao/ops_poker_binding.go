package main

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"regexp"
	"strconv"

	"github.com/cy4268/momiao/internal/platform"
	"github.com/cy4268/momiao/internal/poker"
)

var pokerOpsCodePattern = regexp.MustCompile(`^[A-Z][A-Z0-9_]{0,127}$`)

type pokerOpsInput struct {
	Accepting       *bool  `json:"accepting,omitempty"`
	AllowNewHands   *bool  `json:"allow_new_hands,omitempty"`
	TargetSessionID string `json:"target_session_id,omitempty"`
	TargetUserID    string `json:"target_user_id,omitempty"`
	TableID         string `json:"table_id,omitempty"`
	Muted           *bool  `json:"muted,omitempty"`
	Hidden          *bool  `json:"hidden,omitempty"`
	RecoveryAction  string `json:"recovery_action,omitempty"`
}

type pokerOpsOperation struct {
	operationType string
	port          PokerOpsPort
}

type pokerOpsBindingSpec struct {
	operationType string
	permission    string
	targetType    string
	inputVersion  string
	critical      bool
}

func pokerOpsBindings(port PokerOpsPort) []platform.OpsOperationBinding {
	specs := []pokerOpsBindingSpec{
		{"POKER_ACCEPTING_PLAYERS_SET", "poker.accepting_players.write", "poker_table", "poker-accepting-players.v1", false},
		{"POKER_NEW_HANDS_SET", "poker.accepting_players.write", "poker_table", "poker-new-hands.v1", false},
		{"POKER_CLOSE_AFTER_HAND", "poker.accepting_players.write", "poker_table", "poker-close-after-hand.v1", false},
		{"POKER_REMOVE_PLAYER_AFTER_HAND", "poker.accepting_players.write", "poker_table", "poker-remove-player.v1", false},
		{"POKER_REMOVE_SPECTATOR", "poker.accepting_players.write", "poker_table", "poker-remove-spectator.v1", false},
		{"POKER_CHAT_MUTE_SET", "poker.chat.moderate", "poker_table", "poker-chat-mute.v1", false},
		{"POKER_CHAT_VISIBILITY_SET", "poker.chat.moderate", "poker_message", "poker-chat-visibility.v1", false},
		{"POKER_TABLE_PAUSE", "poker.accepting_players.write", "poker_table", "poker-table-pause.v1", false},
		{"POKER_TABLE_RESUME", "poker.accepting_players.write", "poker_table", "poker-table-resume.v1", false},
		{"POKER_RECOVERY_REQUEST", "poker.recovery.request", "poker_table", "poker-recovery-request.v1", false},
		{"POKER_EMERGENCY_PAUSE", "poker.emergency_pause", "poker_table", "poker-emergency-pause.v1", true},
	}
	bindings := make([]platform.OpsOperationBinding, 0, len(specs))
	for _, spec := range specs {
		risk, confirmation, roles := platform.OpsRiskImpactful, platform.OpsConfirmExplicit, []string{"SUPER_ADMIN", "OPERATOR"}
		requiresFresh := false
		if spec.critical {
			risk, confirmation, roles = platform.OpsRiskCritical, platform.OpsConfirmTyped, []string{"SUPER_ADMIN"}
			requiresFresh = true
		}
		var handler platform.OpsOperationHandler
		if port != nil {
			handler = pokerOpsOperation{operationType: spec.operationType, port: port}
		}
		bindings = append(bindings, platform.OpsOperationBinding{
			Descriptor: platform.OpsOperationDescriptor{
				OperationType: spec.operationType, Risk: risk, RequiredPermission: spec.permission,
				AllowedRoles: roles, TargetType: spec.targetType, InputSchemaVersion: spec.inputVersion,
				ImpactSchemaVersion: "poker-operation-impact.v1", RequiresReason: true,
				RequiresFreshAuth: requiresFresh, ConfirmationMode: confirmation,
				ExecutionMode: platform.OpsDurableRemote,
			},
			Handler: handler,
		})
	}
	return bindings
}

func decodePokerOpsObject(raw json.RawMessage, out any) error {
	if len(raw) == 0 || len(raw) > 32768 {
		return platform.ErrOpsInvalid
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(out); err != nil {
		return platform.ErrOpsInvalid
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return platform.ErrOpsInvalid
	}
	return nil
}

func validPokerOpsUser(value string) bool {
	parsed, err := strconv.ParseInt(value, 10, 64)
	return err == nil && parsed > 0 && strconv.FormatInt(parsed, 10) == value
}

func validPokerOpsVersion(value string) bool {
	parsed, err := strconv.ParseUint(value, 10, 64)
	return err == nil && parsed > 0 && strconv.FormatUint(parsed, 10) == value
}

func (input pokerOpsInput) empty() bool {
	return input.Accepting == nil && input.AllowNewHands == nil && input.TargetSessionID == "" &&
		input.TargetUserID == "" && input.TableID == "" && input.Muted == nil && input.Hidden == nil &&
		input.RecoveryAction == ""
}

func (input pokerOpsInput) valid(operationType string) bool {
	switch operationType {
	case "POKER_ACCEPTING_PLAYERS_SET":
		copy := input
		copy.Accepting = nil
		return input.Accepting != nil && copy.empty()
	case "POKER_NEW_HANDS_SET":
		copy := input
		copy.AllowNewHands = nil
		return input.AllowNewHands != nil && copy.empty()
	case "POKER_CLOSE_AFTER_HAND", "POKER_TABLE_PAUSE", "POKER_TABLE_RESUME", "POKER_EMERGENCY_PAUSE":
		return input.empty()
	case "POKER_REMOVE_PLAYER_AFTER_HAND":
		copy := input
		copy.TargetSessionID, copy.TargetUserID = "", ""
		return opsUUIDPatternHTTP(input.TargetSessionID) && validPokerOpsUser(input.TargetUserID) && copy.empty()
	case "POKER_REMOVE_SPECTATOR":
		copy := input
		copy.TargetUserID = ""
		return validPokerOpsUser(input.TargetUserID) && copy.empty()
	case "POKER_CHAT_MUTE_SET":
		copy := input
		copy.TargetUserID, copy.Muted = "", nil
		return validPokerOpsUser(input.TargetUserID) && input.Muted != nil && copy.empty()
	case "POKER_CHAT_VISIBILITY_SET":
		copy := input
		copy.TableID, copy.Hidden = "", nil
		return opsUUIDPatternHTTP(input.TableID) && input.Hidden != nil && copy.empty()
	case "POKER_RECOVERY_REQUEST":
		copy := input
		copy.RecoveryAction = ""
		return (input.RecoveryAction == "RECOVER" || input.RecoveryAction == "RESUME" ||
			input.RecoveryAction == "SAFE_CLOSE" || input.RecoveryAction == "ESCALATE") && copy.empty()
	default:
		return false
	}
}

func (handler pokerOpsOperation) Canonicalize(raw json.RawMessage) (json.RawMessage, error) {
	var input pokerOpsInput
	if err := decodePokerOpsObject(raw, &input); err != nil || !input.valid(handler.operationType) {
		return nil, platform.ErrOpsInvalid
	}
	canonical, err := json.Marshal(input)
	if err != nil {
		return nil, platform.ErrOpsInvalid
	}
	return canonical, nil
}

type pokerOpsImpactTarget struct {
	Kind             string `json:"kind"`
	UserID           string `json:"user_id,omitempty"`
	SessionID        string `json:"session_id,omitempty"`
	MessageID        string `json:"message_id,omitempty"`
	State            string `json:"state,omitempty"`
	Connected        *bool  `json:"connected,omitempty"`
	LeaveAfterHand   *bool  `json:"leave_after_hand,omitempty"`
	ModerationState  string `json:"moderation_state,omitempty"`
}

type pokerOpsImpactState struct {
	TableID          string                `json:"table_id"`
	TableVersion     string                `json:"table_version"`
	LifecycleState   string                `json:"lifecycle_state"`
	AcceptingPlayers bool                  `json:"accepting_players"`
	AllowNewHands    bool                  `json:"allow_new_hands"`
	ChatEnabled      bool                  `json:"chat_enabled"`
	CurrentHandID    string                `json:"current_hand_id,omitempty"`
	NeedsReview      bool                  `json:"needs_review"`
	RecoveryState    string                `json:"recovery_state,omitempty"`
	OccupiedSeats    int64                 `json:"occupied_seats"`
	SpectatorCount   int64                 `json:"spectator_count"`
	Target           *pokerOpsImpactTarget `json:"target,omitempty"`
}

type pokerOpsProposedChange struct {
	CommandType      string `json:"command_type"`
	TableID          string `json:"table_id"`
	Accepting        *bool  `json:"accepting,omitempty"`
	AllowNewHands    *bool  `json:"allow_new_hands,omitempty"`
	TargetSessionID  string `json:"target_session_id,omitempty"`
	TargetUserID     string `json:"target_user_id,omitempty"`
	MessageID        string `json:"message_id,omitempty"`
	Muted            *bool  `json:"muted,omitempty"`
	Hidden           *bool  `json:"hidden,omitempty"`
	RecoveryAction   string `json:"recovery_action,omitempty"`
}

func pokerOpsRaw(value any) (json.RawMessage, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, platform.ErrOpsInvalid
	}
	return raw, nil
}

func pokerOpsReadError(err error) error {
	switch {
	case errors.Is(err, poker.ErrInvalid):
		return platform.ErrOpsInvalid
	case errors.Is(err, poker.ErrDenied), errors.Is(err, errPokerOpsNotFound):
		return platform.ErrOpsNotFound
	default:
		return platform.ErrOpsUnavailable
	}
}

func (handler pokerOpsOperation) PrepareRemote(ctx context.Context, _ platform.OpsPrincipal,
	request platform.OpsPrepareRequest) (platform.OpsPreparedMaterial, error) {
	if handler.port == nil || request.OperationType != handler.operationType ||
		!opsUUIDPatternHTTP(request.Target.ID) || !validPokerOpsVersion(request.Target.ExpectedVersion) {
		return platform.OpsPreparedMaterial{}, platform.ErrOpsInvalid
	}
	canonical, err := handler.Canonicalize(request.Input)
	if err != nil {
		return platform.OpsPreparedMaterial{}, err
	}
	var input pokerOpsInput
	if err = decodePokerOpsObject(canonical, &input); err != nil {
		return platform.OpsPreparedMaterial{}, err
	}
	tableID := request.Target.ID
	if handler.operationType == "POKER_CHAT_VISIBILITY_SET" {
		tableID = input.TableID
	}
	detail, err := handler.port.ReadOpsTable(ctx, tableID)
	if err != nil {
		return platform.OpsPreparedMaterial{}, pokerOpsReadError(err)
	}
	if detail.Table.TableID != tableID || !validPokerOpsVersion(detail.Table.TableVersion) {
		return platform.OpsPreparedMaterial{}, platform.ErrOpsConflict
	}
	state := pokerOpsImpactState{
		TableID: detail.Table.TableID, TableVersion: detail.Table.TableVersion,
		LifecycleState: detail.Table.LifecycleState, AcceptingPlayers: detail.Table.AcceptingPlayers,
		AllowNewHands: detail.Table.AllowNewHands, ChatEnabled: detail.Table.ChatEnabled,
		CurrentHandID: detail.Table.CurrentHandID, NeedsReview: detail.Table.NeedsReview,
		RecoveryState: detail.Table.RecoveryState, OccupiedSeats: detail.Table.OccupiedSeats,
		SpectatorCount: detail.Table.SpectatorCount,
	}
	proposed := pokerOpsProposedChange{
		CommandType: handler.operationType, TableID: tableID, Accepting: input.Accepting,
		AllowNewHands: input.AllowNewHands, TargetSessionID: input.TargetSessionID,
		TargetUserID: input.TargetUserID, Muted: input.Muted, Hidden: input.Hidden,
		RecoveryAction: input.RecoveryAction,
	}
	blocking, continuing, related := []string{}, []string{}, []string{tableID}
	affectedUsers, affectedItems := int64(0), int64(1)
	if detail.Table.LifecycleState == "CLOSED" {
		blocking = append(blocking, "POKER_TABLE_CLOSED")
	}
	switch handler.operationType {
	case "POKER_ACCEPTING_PLAYERS_SET":
		if detail.Table.AcceptingPlayers == *input.Accepting {
			blocking = append(blocking, "POKER_TARGET_STATE_UNCHANGED")
		}
	case "POKER_NEW_HANDS_SET":
		if detail.Table.AllowNewHands == *input.AllowNewHands {
			blocking = append(blocking, "POKER_TARGET_STATE_UNCHANGED")
		}
	case "POKER_CLOSE_AFTER_HAND":
		if detail.Table.LifecycleState == "CLOSING" {
			blocking = append(blocking, "POKER_TARGET_STATE_UNCHANGED")
		}
		if detail.Table.CurrentHandID != "" {
			continuing = append(continuing, "POKER_CURRENT_HAND_COMPLETES_OR_RECOVERS_BEFORE_CLOSE")
		}
	case "POKER_REMOVE_PLAYER_AFTER_HAND":
		var found *poker.OpsPlayer
		for index := range detail.Players {
			player := &detail.Players[index]
			if player.SessionID == input.TargetSessionID && player.UserID == input.TargetUserID {
				found = player
				break
			}
		}
		if found == nil {
			return platform.OpsPreparedMaterial{}, platform.ErrOpsNotFound
		}
		connected, leave := found.Connected, found.LeaveAfterHand
		state.Target = &pokerOpsImpactTarget{Kind: "PLAYER", UserID: found.UserID, SessionID: found.SessionID,
			State: found.State, Connected: &connected, LeaveAfterHand: &leave}
		affectedUsers, related = 1, append(related, found.SessionID, found.UserID)
		continuing = append(continuing, "POKER_CURRENT_HAND_COMPLETES_BEFORE_PLAYER_REMOVAL")
	case "POKER_REMOVE_SPECTATOR":
		state.Target = &pokerOpsImpactTarget{Kind: "SPECTATOR", UserID: input.TargetUserID}
		affectedUsers, related = 1, append(related, input.TargetUserID)
	case "POKER_CHAT_MUTE_SET":
		state.Target = &pokerOpsImpactTarget{Kind: "CHAT_USER", UserID: input.TargetUserID}
		affectedUsers, related = 1, append(related, input.TargetUserID)
	case "POKER_CHAT_VISIBILITY_SET":
		proposed.MessageID = request.Target.ID
		var found *poker.OpsMessage
		for index := range detail.Messages {
			message := &detail.Messages[index]
			if message.MessageID == request.Target.ID {
				found = message
				break
			}
		}
		if found == nil || found.Kind == "SYSTEM" {
			return platform.OpsPreparedMaterial{}, platform.ErrOpsNotFound
		}
		state.Target = &pokerOpsImpactTarget{Kind: "CHAT_MESSAGE", MessageID: found.MessageID,
			ModerationState: found.ModerationState}
		desired := "VISIBLE"
		if *input.Hidden {
			desired = "HIDDEN"
		}
		if found.ModerationState == desired {
			blocking = append(blocking, "POKER_TARGET_STATE_UNCHANGED")
		}
		related = append(related, found.MessageID)
	case "POKER_TABLE_PAUSE":
		if !detail.Table.AllowNewHands && detail.Table.LifecycleState == "PAUSED" {
			blocking = append(blocking, "POKER_TARGET_STATE_UNCHANGED")
		}
		if detail.Table.CurrentHandID != "" {
			continuing = append(continuing, "POKER_CURRENT_HAND_REMAINS_DURABLE")
		}
	case "POKER_TABLE_RESUME":
		if detail.Table.NeedsReview || detail.Table.LifecycleState == "RECOVERING" {
			blocking = append(blocking, "POKER_RECOVERY_REVIEW_REQUIRED")
		} else if detail.Table.AllowNewHands && detail.Table.LifecycleState != "PAUSED" {
			blocking = append(blocking, "POKER_TARGET_STATE_UNCHANGED")
		}
	case "POKER_RECOVERY_REQUEST":
		switch input.RecoveryAction {
		case "RECOVER":
			if !detail.Table.NeedsReview {
				blocking = append(blocking, "POKER_RECOVERY_REVIEW_NOT_PRESENT")
			}
		case "RESUME":
			if detail.Table.NeedsReview {
				blocking = append(blocking, "POKER_RECOVERY_REVIEW_REQUIRED")
			}
		case "SAFE_CLOSE":
			if !detail.Table.NeedsReview && detail.Table.LifecycleState != "RECOVERING" {
				blocking = append(blocking, "POKER_RECOVERY_NOT_ACTIVE")
			}
		}
	case "POKER_EMERGENCY_PAUSE":
		if detail.Table.CurrentHandID != "" {
			continuing = append(continuing, "POKER_CURRENT_HAND_FACTS_REMAIN_UNCHANGED")
		}
	}
	currentRaw, err := pokerOpsRaw(state)
	if err != nil {
		return platform.OpsPreparedMaterial{}, err
	}
	proposedRaw, err := pokerOpsRaw(proposed)
	if err != nil {
		return platform.OpsPreparedMaterial{}, err
	}
	impact := platform.OpsImpact{CurrentState: currentRaw, ProposedChange: proposedRaw,
		AffectedItems: &affectedItems, BlockingFacts: blocking, ContinuingAcceptedWork: continuing,
		RelatedIDs: related, UnavailableMeasurements: []string{}}
	if affectedUsers > 0 {
		impact.AffectedUsers = &affectedUsers
	}
	return platform.OpsPreparedMaterial{Impact: impact, TargetVersion: detail.Table.TableVersion,
		TargetLocator: tableID}, nil
}

func pokerOpsCommand(operationType, operationID string, actor int64, target platform.OpsTarget,
	reason string, input pokerOpsInput) poker.OpsCommand {
	tableID := target.ID
	if operationType == "POKER_CHAT_VISIBILITY_SET" {
		tableID = input.TableID
	}
	command := poker.OpsCommand{OperationID: operationID, ActorUserID: actor, CommandType: operationType,
		TableID: tableID, ExpectedTableVersion: target.ExpectedVersion, Reason: reason}
	command.Accepting, command.AllowNewHands = input.Accepting, input.AllowNewHands
	command.TargetSessionID, command.TargetUserID = input.TargetSessionID, input.TargetUserID
	command.Muted, command.Hidden, command.RecoveryAction = input.Muted, input.Hidden, input.RecoveryAction
	if operationType == "POKER_CHAT_VISIBILITY_SET" {
		command.MessageID = target.ID
	}
	return command
}

func (handler pokerOpsOperation) BuildRemoteCommand(principal platform.OpsPrincipal, operation platform.OpsOperation,
	raw json.RawMessage, material platform.OpsPreparedMaterial) (json.RawMessage, error) {
	if handler.port == nil || operation.OperationType != handler.operationType || principal.UserID <= 0 ||
		principal.UserID != operation.ActorUserID || material.TargetVersion != operation.Target.ExpectedVersion ||
		material.Impact.Target != operation.Target {
		return nil, platform.ErrOpsConflict
	}
	var input pokerOpsInput
	if err := decodePokerOpsObject(raw, &input); err != nil || !input.valid(handler.operationType) {
		return nil, platform.ErrOpsInvalid
	}
	command := pokerOpsCommand(handler.operationType, operation.OperationID, principal.UserID,
		operation.Target, operation.Reason, input)
	if material.TargetLocator != command.TableID {
		return nil, platform.ErrOpsPreviewStale
	}
	result, err := json.Marshal(command)
	if err != nil {
		return nil, platform.ErrOpsInvalid
	}
	return result, nil
}

func pokerOpsRemoteError(err error) error {
	switch {
	case errors.Is(err, poker.ErrConflict):
		return &platform.OpsRemoteFault{Code: "POKER_REQUEST_CONFLICT", NeedsReview: true}
	case errors.Is(err, poker.ErrInvalid):
		return &platform.OpsRemoteFault{Code: "POKER_INVALID_COMMAND"}
	case errors.Is(err, poker.ErrDenied):
		return &platform.OpsRemoteFault{Code: "POKER_COMMAND_DENIED"}
	case errors.Is(err, poker.ErrStaleVersion):
		return &platform.OpsRemoteFault{Code: "POKER_STALE_VERSION"}
	case errors.Is(err, poker.ErrNeedsReview), errors.Is(err, poker.ErrCorruptSnapshot):
		return &platform.OpsRemoteFault{Code: "POKER_HAND_NEEDS_REVIEW", NeedsReview: true}
	case errors.Is(err, poker.ErrBusy):
		return &platform.OpsRemoteFault{Code: "POKER_TABLE_BUSY", Retryable: true}
	case errors.Is(err, poker.ErrClosed), errors.Is(err, errPokerStartup):
		return &platform.OpsRemoteFault{Code: "POKER_SERVICE_UNAVAILABLE", Retryable: true}
	default:
		return err
	}
}

func (handler pokerOpsOperation) QueryOperation(ctx context.Context, operationType, operationID string) (json.RawMessage, bool, error) {
	if handler.port == nil || operationType != handler.operationType || !opsUUIDPatternHTTP(operationID) {
		return nil, false, &platform.OpsRemoteFault{Code: "OPS_REMOTE_COMMAND_INVALID", NeedsReview: true}
	}
	receipt, err := handler.port.ReadOpsOperation(ctx, operationID)
	if errors.Is(err, poker.ErrDenied) || errors.Is(err, errPokerOpsNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, pokerOpsRemoteError(err)
	}
	raw, err := json.Marshal(receipt)
	if err != nil {
		return nil, false, &platform.OpsRemoteFault{Code: "OPS_REMOTE_RECEIPT_INVALID", NeedsReview: true}
	}
	return raw, true, nil
}

func (handler pokerOpsOperation) ExecuteOperation(ctx context.Context, operationType string,
	raw json.RawMessage) (json.RawMessage, error) {
	if handler.port == nil || operationType != handler.operationType {
		return nil, &platform.OpsRemoteFault{Code: "OPS_REMOTE_COMMAND_INVALID", NeedsReview: true}
	}
	var command poker.OpsCommand
	if err := decodePokerOpsObject(raw, &command); err != nil || command.CommandType != handler.operationType {
		return nil, &platform.OpsRemoteFault{Code: "OPS_REMOTE_COMMAND_INVALID", NeedsReview: true}
	}
	receipt, err := handler.port.ExecuteOpsCommand(ctx, command)
	if err != nil {
		return nil, pokerOpsRemoteError(err)
	}
	result, err := json.Marshal(receipt)
	if err != nil {
		return nil, &platform.OpsRemoteFault{Code: "OPS_REMOTE_RECEIPT_INVALID", NeedsReview: true}
	}
	return result, nil
}

func validPokerOpsResult(command poker.OpsCommand, result string) bool {
	if !pokerOpsCodePattern.MatchString(result) {
		return false
	}
	switch command.CommandType {
	case "POKER_ACCEPTING_PLAYERS_SET":
		return result == "ACCEPTING_PLAYERS" || result == "NOT_ACCEPTING_PLAYERS"
	case "POKER_NEW_HANDS_SET":
		return result == "NEW_HANDS_RESUMED" || result == "NEW_HANDS_PAUSED"
	case "POKER_CLOSE_AFTER_HAND":
		return result == "CLOSE_AFTER_HAND"
	case "POKER_REMOVE_PLAYER_AFTER_HAND":
		return result == "PLAYER_REMOVE_AFTER_HAND" || result == "PLAYER_REMOVED"
	case "POKER_REMOVE_SPECTATOR":
		return result == "SPECTATOR_REMOVED"
	case "POKER_CHAT_MUTE_SET":
		return result == "CHAT_USER_MUTED" || result == "CHAT_USER_UNMUTED"
	case "POKER_CHAT_VISIBILITY_SET":
		return result == "CHAT_MESSAGE_HIDDEN" || result == "CHAT_MESSAGE_RESTORED"
	case "POKER_TABLE_PAUSE":
		return result == "TABLE_PAUSED"
	case "POKER_TABLE_RESUME":
		return result == "TABLE_RESUMED"
	case "POKER_EMERGENCY_PAUSE":
		return result == "EMERGENCY_PAUSED"
	case "POKER_RECOVERY_REQUEST":
		switch command.RecoveryAction {
		case "RECOVER":
			return result == "RECOVERY_RELEASED" || result == "RECOVERED" || result == "RECOVERING" || result == "NEEDS_REVIEW" ||
				result == "PREFLOP" || result == "FLOP" || result == "TURN" || result == "RIVER" || result == "SETTLED"
		case "RESUME":
			return result == "RECOVERY_RESUMED" || result == "BOUNDARY" || result == "CLOSED" ||
				result == "AUTO_START" || result == "PREFLOP" || result == "FLOP" || result == "TURN" ||
				result == "RIVER" || result == "SETTLED"
		case "SAFE_CLOSE":
			return result == "RECOVERY_SAFE_CLOSE_QUEUED"
		case "ESCALATE":
			return result == "RECOVERY_ESCALATED"
		}
	}
	return false
}

type pokerOpsAfterSnapshot struct {
	TableID      string `json:"table_id"`
	TableVersion string `json:"table_version"`
	CommandType  string `json:"command_type"`
	State        string `json:"state"`
	ResultCode   string `json:"result_code"`
}

func (handler pokerOpsOperation) AcceptRemoteReceipt(operation platform.OpsOperation, commandRaw,
	receiptRaw json.RawMessage) (platform.OpsRemoteCompletion, error) {
	if operation.OperationType != handler.operationType {
		return platform.OpsRemoteCompletion{}, platform.ErrOpsInvalid
	}
	var input pokerOpsInput
	if err := decodePokerOpsObject(operation.CanonicalInput(), &input); err != nil || !input.valid(handler.operationType) {
		return platform.OpsRemoteCompletion{}, platform.ErrOpsInvalid
	}
	expectedCommand := pokerOpsCommand(handler.operationType, operation.OperationID, operation.ActorUserID,
		operation.Target, operation.Reason, input)
	var command poker.OpsCommand
	if err := decodePokerOpsObject(commandRaw, &command); err != nil || !reflect.DeepEqual(command, expectedCommand) {
		return platform.OpsRemoteCompletion{}, platform.ErrOpsInvalid
	}
	commandHash, err := poker.OpsCommandHash(command)
	if err != nil {
		return platform.OpsRemoteCompletion{}, platform.ErrOpsInvalid
	}
	var receipt poker.OpsReceipt
	if err = decodePokerOpsObject(receiptRaw, &receipt); err != nil || receipt.OperationID != operation.OperationID ||
		receipt.CommandType != handler.operationType || receipt.TableID != command.TableID ||
		len(receipt.CommandHash) != len(commandHash) || subtle.ConstantTimeCompare([]byte(receipt.CommandHash), []byte(commandHash)) != 1 ||
		!validPokerOpsResult(command, receipt.ResultCode) {
		return platform.OpsRemoteCompletion{}, platform.ErrOpsInvalid
	}
	expectedVersion, parseErr := strconv.ParseUint(operation.Target.ExpectedVersion, 10, 64)
	receiptVersion, receiptErr := strconv.ParseUint(receipt.TableVersion, 10, 64)
	if parseErr != nil || receiptErr != nil || expectedVersion == ^uint64(0) || receiptVersion != expectedVersion+1 ||
		strconv.FormatUint(receiptVersion, 10) != receipt.TableVersion ||
		(receipt.State == "NEEDS_REVIEW") != (receipt.ResultCode == "NEEDS_REVIEW") ||
		receipt.State != "SUCCEEDED" && receipt.State != "NEEDS_REVIEW" {
		return platform.OpsRemoteCompletion{}, platform.ErrOpsInvalid
	}
	afterRaw, err := pokerOpsRaw(pokerOpsAfterSnapshot{TableID: receipt.TableID,
		TableVersion: receipt.TableVersion, CommandType: receipt.CommandType,
		State: receipt.State, ResultCode: receipt.ResultCode})
	if err != nil {
		return platform.OpsRemoteCompletion{}, err
	}
	completion := platform.OpsRemoteCompletion{State: receipt.State,
		Execution: platform.OpsExecutionResult{BeforeSnapshot: operation.Impact.CurrentState,
			AfterSnapshot: afterRaw, Result: append(json.RawMessage{}, receiptRaw...),
			RelatedBusinessID: receipt.TableID}}
	if receipt.State == "NEEDS_REVIEW" {
		completion.FailureCode = "POKER_HAND_NEEDS_REVIEW"
	}
	return completion, nil
}
