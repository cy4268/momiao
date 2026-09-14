package platform

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const nativeAttributionMaxResponse = 4 << 20

var (
	ErrNativeAttributionDependency = errors.New("native attribution dependency unavailable")
	ErrKeyPurposeRejected          = errors.New("native key purpose rejected")
)

type KeyPurpose string

const (
	KeyPurposeGeneral      KeyPurpose = "GENERAL"
	KeyPurposeRoleplay     KeyPurpose = "ROLEPLAY"
	KeyPurposeUnclassified KeyPurpose = "UNCLASSIFIED"
)

type NativeKeyPurposeChange struct {
	OperationID string
	UserID      int64
	TokenID     int64
	Purpose     KeyPurpose
	Version     int64
	EffectiveAt time.Time
}

type NativeKeyPurposeReceipt struct {
	OperationID string     `json:"operation_id"`
	UserID      int64      `json:"user_id,string"`
	TokenID     int64      `json:"token_id,string"`
	Purpose     KeyPurpose `json:"purpose"`
	Version     int64      `json:"version,string"`
	EffectiveAt time.Time  `json:"effective_at"`
	Result      string     `json:"result"`
}

type NativePurposeWriter interface {
	TokenOwnedByUser(context.Context, int64, int64) (bool, error)
	ApplyKeyPurpose(context.Context, NativeKeyPurposeChange) (NativeKeyPurposeReceipt, error)
}

type RequestAttribution struct {
	SourceInstanceID        string     `json:"source_instance_id"`
	SourceEventID           int64      `json:"source_event_id,string"`
	LogicalRequestID        string     `json:"logical_request_id"`
	UserID                  int64      `json:"user_id,string"`
	TokenID                 int64      `json:"token_id,string"`
	KeyPurposeSnapshot      KeyPurpose `json:"key_purpose_snapshot"`
	PurposeVersionSnapshot  int64      `json:"purpose_version_snapshot,string"`
	RequestModelIDSnapshot  string     `json:"request_model_id_snapshot"`
	RequestModelNameSnapshot string    `json:"request_model_name_snapshot"`
	RequestKind             string     `json:"request_kind"`
	EnteredModelFlow        bool       `json:"entered_model_flow"`
	ProviderAttemptCount    int        `json:"provider_attempt_count,string"`
	FinalStatus             string     `json:"final_status"`
	ErrorCategory           string     `json:"error_category"`
	ChargedRawQuota         int64      `json:"charged_raw_quota,string"`
	RequestedAt             time.Time  `json:"requested_at"`
	CompletedAt             time.Time  `json:"completed_at"`
}

type AttributionPage struct {
	SourceInstanceID string               `json:"source_instance_id"`
	ObservedAt       time.Time            `json:"observed_at"`
	NextCursor       int64                `json:"next_cursor,string"`
	HasMore          bool                 `json:"has_more"`
	Items            []RequestAttribution `json:"items"`
}

type NativeAttributionReader interface {
	ReadAttributions(context.Context, int64, int) (AttributionPage, error)
}

type NativeAttributionPort struct {
	client           http.Client
	secret           string
	expectedSourceID string
}

func NewNativeAttributionPort(transport http.RoundTripper, secret, expectedSourceID string) (*NativeAttributionPort, error) {
	decoded, err := hex.DecodeString(secret)
	if transport == nil || err != nil || len(decoded) != 32 || strings.ToLower(secret) != secret || !ValidOperationKey(expectedSourceID) {
		return nil, ErrInvalidMutation
	}
	return &NativeAttributionPort{
		client: http.Client{Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error {
			return errors.New("native attribution redirect refused")
		}},
		secret: secret, expectedSourceID: expectedSourceID,
	}, nil
}

func (port *NativeAttributionPort) post(ctx context.Context, path string, input, output any) error {
	if port == nil {
		return ErrNativeAttributionDependency
	}
	body, err := json.Marshal(input)
	if err != nil {
		return ErrInvalidMutation
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://unix"+path, bytes.NewReader(body))
	if err != nil {
		return ErrNativeAttributionDependency
	}
	request.Host = "localhost"
	request.Header.Set("Authorization", "Bearer "+port.secret)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	response, err := port.client.Do(request)
	if err != nil || response == nil {
		if response != nil && response.Body != nil {
			_ = response.Body.Close()
		}
		return ErrNativeAttributionDependency
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, nativeAttributionMaxResponse+1))
	if err != nil || len(raw) == 0 || len(raw) > nativeAttributionMaxResponse || !utf8.Valid(raw) || ctx.Err() != nil {
		return ErrNativeAttributionDependency
	}
	mediaType, _, mediaErr := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if mediaErr != nil || mediaType != "application/json" {
		return ErrNativeAttributionDependency
	}
	if response.StatusCode != http.StatusOK {
		var fault nativeQuotaFault
		if decodeQuotaJSON(raw, &fault) != nil || fault.Code == "" {
			return ErrNativeAttributionDependency
		}
		switch response.StatusCode {
		case http.StatusBadRequest:
			return ErrInvalidMutation
		case http.StatusConflict:
			return ErrIdempotencyConflict
		default:
			return ErrNativeAttributionDependency
		}
	}
	if decodeQuotaJSON(raw, output) != nil {
		return ErrNativeAttributionDependency
	}
	return nil
}

type nativeOwnerRequest struct {
	UserID  string `json:"user_id"`
	TokenID string `json:"token_id"`
}
type nativeOwnerResponse struct {
	UserID  string `json:"user_id"`
	TokenID string `json:"token_id"`
	Owned   bool   `json:"owned"`
}

func (port *NativeAttributionPort) TokenOwnedByUser(ctx context.Context, user, token int64) (bool, error) {
	if !validateQuotaUser(user) || !validateQuotaUser(token) {
		return false, ErrInvalidMutation
	}
	input := nativeOwnerRequest{UserID: strconv.FormatInt(user, 10), TokenID: strconv.FormatInt(token, 10)}
	var result nativeOwnerResponse
	if err := port.post(ctx, "/internal/momiao/key-purpose/owner", input, &result); err != nil {
		return false, err
	}
	if result.UserID != input.UserID || result.TokenID != input.TokenID {
		return false, ErrNativeAttributionDependency
	}
	return result.Owned, nil
}

type nativePurposeRequest struct {
	OperationID string `json:"operation_id"`
	UserID      string `json:"user_id"`
	TokenID     string `json:"token_id"`
	Purpose     string `json:"purpose"`
	Version     string `json:"version"`
	EffectiveAt string `json:"effective_at"`
}

func validWritablePurpose(value KeyPurpose) bool {
	return value == KeyPurposeGeneral || value == KeyPurposeRoleplay
}

func (port *NativeAttributionPort) ApplyKeyPurpose(ctx context.Context, change NativeKeyPurposeChange) (NativeKeyPurposeReceipt, error) {
	if !ValidOperationKey(change.OperationID) || !validateQuotaUser(change.UserID) || !validateQuotaUser(change.TokenID) ||
		!validWritablePurpose(change.Purpose) || change.Version <= 0 || change.EffectiveAt.IsZero() {
		return NativeKeyPurposeReceipt{}, ErrInvalidMutation
	}
	input := nativePurposeRequest{
		OperationID: change.OperationID, UserID: strconv.FormatInt(change.UserID, 10), TokenID: strconv.FormatInt(change.TokenID, 10),
		Purpose: string(change.Purpose), Version: strconv.FormatInt(change.Version, 10), EffectiveAt: change.EffectiveAt.UTC().Format(time.RFC3339Nano),
	}
	var receipt NativeKeyPurposeReceipt
	if err := port.post(ctx, "/internal/momiao/key-purpose/apply", input, &receipt); err != nil {
		return NativeKeyPurposeReceipt{}, err
	}
	if receipt.OperationID != change.OperationID || receipt.UserID != change.UserID || receipt.TokenID != change.TokenID || receipt.Purpose != change.Purpose ||
		receipt.Version != change.Version || !receipt.EffectiveAt.Equal(change.EffectiveAt) {
		return NativeKeyPurposeReceipt{}, ErrNativeAttributionDependency
	}
	switch receipt.Result {
	case "APPLIED", "ALREADY_APPLIED", "STALE_VERSION", "ACCOUNT_RESTRICTED":
	default:
		return NativeKeyPurposeReceipt{}, ErrNativeAttributionDependency
	}
	return receipt, nil
}

type nativeAttributionReadRequest struct {
	AfterCursor string `json:"after_cursor"`
	Limit       string `json:"limit"`
}

func validAttributionLabel(value string, maximum int) bool {
	return value != "" && len(value) <= maximum && utf8.ValidString(value) && strings.TrimSpace(value) == value && !strings.ContainsRune(value, 0)
}

func validAttributionStatus(value string) bool {
	switch value {
	case "SUCCESS", "ERROR", "CANCELLED_PRE_UPSTREAM", "CANCELLED_POST_UPSTREAM":
		return true
	default:
		return false
	}
}

func (port *NativeAttributionPort) ReadAttributions(ctx context.Context, after int64, limit int) (AttributionPage, error) {
	if port == nil || after < 0 || limit < 1 || limit > 500 {
		return AttributionPage{}, ErrInvalidMutation
	}
	var page AttributionPage
	if err := port.post(ctx, "/internal/momiao/attribution/read", nativeAttributionReadRequest{AfterCursor: strconv.FormatInt(after, 10), Limit: strconv.Itoa(limit)}, &page); err != nil {
		return AttributionPage{}, err
	}
	if page.SourceInstanceID != port.expectedSourceID || page.ObservedAt.IsZero() || page.ObservedAt.After(time.Now().UTC().Add(time.Minute)) || page.NextCursor < after || len(page.Items) > limit || page.HasMore && len(page.Items) != limit {
		return AttributionPage{}, ErrNativeAttributionDependency
	}
	last := after
	for i := range page.Items {
		row := &page.Items[i]
		row.SourceInstanceID = page.SourceInstanceID
		purposeOK := validWritablePurpose(row.KeyPurposeSnapshot) && row.PurposeVersionSnapshot > 0 || row.KeyPurposeSnapshot == KeyPurposeUnclassified && row.PurposeVersionSnapshot == 0
		statusOK := validAttributionStatus(row.FinalStatus) && (row.FinalStatus == "SUCCESS") == (row.ErrorCategory == "")
		if row.SourceEventID <= last || !validateQuotaUser(row.UserID) || !validateQuotaUser(row.TokenID) || !purposeOK ||
			!validAttributionLabel(row.LogicalRequestID, 128) || !validAttributionLabel(row.RequestModelIDSnapshot, 128) ||
			!validAttributionLabel(row.RequestModelNameSnapshot, 128) || !validAttributionLabel(row.RequestKind, 64) ||
			row.ProviderAttemptCount < 0 || row.EnteredModelFlow != (row.ProviderAttemptCount > 0) || !statusOK || len(row.ErrorCategory) > 64 ||
			row.ChargedRawQuota < 0 || row.ChargedRawQuota > 1<<31-1 || row.RequestedAt.IsZero() || row.CompletedAt.Before(row.RequestedAt) {
			return AttributionPage{}, ErrNativeAttributionDependency
		}
		last = row.SourceEventID
	}
	if last != page.NextCursor || len(page.Items) == 0 && page.NextCursor != after {
		return AttributionPage{}, ErrNativeAttributionDependency
	}
	page.ObservedAt = page.ObservedAt.UTC()
	return page, nil
}
