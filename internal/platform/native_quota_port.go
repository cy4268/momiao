package platform

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const nativeQuotaPortMaxResponse = 16 << 10

var ErrNativeQuotaDependency = errors.New("native quota dependency unavailable")

type NativeQuotaPort struct {
	client http.Client
	secret string
}

func NewNativeQuotaPort(transport http.RoundTripper, secret string) (*NativeQuotaPort, error) {
	decoded, err := hex.DecodeString(secret)
	if transport == nil || err != nil || len(decoded) != 32 || strings.ToLower(secret) != secret {
		return nil, ErrInvalidMutation
	}
	return &NativeQuotaPort{
		client: http.Client{
			Transport: transport,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return errors.New("native quota redirect refused")
			},
		},
		secret: secret,
	}, nil
}

type nativeQuotaReadRequest struct {
	UserID string `json:"user_id"`
}

type nativeQuotaApplyRequest struct {
	OperationID   string `json:"operation_id"`
	UserID        string `json:"user_id"`
	DeltaRawQuota string `json:"delta_raw_quota"`
}

type nativeQuotaQueryRequest struct {
	OperationID string `json:"operation_id"`
	UserID      string `json:"user_id"`
}

type nativeQuotaFault struct {
	Code string `json:"code"`
}

func (port *NativeQuotaPort) post(ctx context.Context, path string, request, response any) error {
	body, err := json.Marshal(request)
	if err != nil {
		return ErrInvalidMutation
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://unix"+path, bytes.NewReader(body))
	if err != nil {
		return ErrNativeQuotaDependency
	}
	req.Host = "localhost"
	req.Header.Set("Authorization", "Bearer "+port.secret)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	upstream, err := port.client.Do(req)
	if err != nil || upstream == nil {
		if upstream != nil && upstream.Body != nil {
			_ = upstream.Body.Close()
		}
		return ErrNativeQuotaDependency
	}
	defer upstream.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(upstream.Body, nativeQuotaPortMaxResponse+1))
	if err != nil || len(raw) == 0 || len(raw) > nativeQuotaPortMaxResponse || !utf8.Valid(raw) || ctx.Err() != nil {
		return ErrNativeQuotaDependency
	}
	mediaType, _, mediaErr := mime.ParseMediaType(upstream.Header.Get("Content-Type"))
	if mediaErr != nil || mediaType != "application/json" {
		return ErrNativeQuotaDependency
	}
	if upstream.StatusCode != http.StatusOK {
		var fault nativeQuotaFault
		if decodeQuotaJSON(raw, &fault) != nil || fault.Code == "" {
			return ErrNativeQuotaDependency
		}
		switch upstream.StatusCode {
		case http.StatusBadRequest:
			return ErrInvalidMutation
		case http.StatusConflict:
			return ErrIdempotencyConflict
		default:
			return fmt.Errorf("%w: %s", ErrNativeQuotaDependency, fault.Code)
		}
	}
	if err := decodeQuotaJSON(raw, response); err != nil {
		return ErrNativeQuotaDependency
	}
	return nil
}

func decodeQuotaJSON(raw []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return errors.New("trailing native quota response")
	}
	return nil
}

func validateQuotaUser(user int64) bool { return user > 0 && user <= 1<<31-1 }
func validateQuotaDelta(delta int64) bool { return delta != 0 && delta >= -(1<<31-1) && delta <= 1<<31-1 }

func validQuotaResult(result string) bool {
	switch result {
	case "APPLIED", "NOT_APPLIED", "INSUFFICIENT", "ACCOUNT_RESTRICTED", "UNKNOWN", "SOURCE_INCOMPATIBLE":
		return true
	default:
		return false
	}
}

func validateNativeQuotaReceipt(receipt NativeQuotaReceipt, id string, user int64, delta *int64) (NativeQuotaReceipt, error) {
	if receipt.ID != id || receipt.UserID != user || !validQuotaResult(receipt.Result) {
		return NativeQuotaReceipt{}, ErrNativeQuotaDependency
	}
	if delta != nil && receipt.DeltaRawQuota != *delta {
		return NativeQuotaReceipt{}, ErrIdempotencyConflict
	}
	if receipt.Result == "APPLIED" {
		if receipt.Before == nil || receipt.After == nil || receipt.DeltaRawQuota == 0 || *receipt.After-*receipt.Before != receipt.DeltaRawQuota {
			return NativeQuotaReceipt{}, ErrNativeQuotaDependency
		}
	}
	receipt.Amount = receipt.DeltaRawQuota
	return receipt, nil
}

func (port *NativeQuotaPort) ReadNativeQuota(ctx context.Context, user int64) (NativeQuotaSnapshot, error) {
	if !validateQuotaUser(user) {
		return NativeQuotaSnapshot{}, ErrInvalidMutation
	}
	var snapshot NativeQuotaSnapshot
	if err := port.post(ctx, "/internal/momiao/quota/read", nativeQuotaReadRequest{UserID: strconv.FormatInt(user, 10)}, &snapshot); err != nil {
		return NativeQuotaSnapshot{}, err
	}
	if snapshot.UserID != user || (snapshot.Result != "APPLIED" && snapshot.Result != "ACCOUNT_RESTRICTED" && snapshot.Result != "SOURCE_INCOMPATIBLE") {
		return NativeQuotaSnapshot{}, ErrNativeQuotaDependency
	}
	snapshot.ObservedAt = snapshot.ObservedAt.UTC()
	if snapshot.ObservedAt.IsZero() || snapshot.ObservedAt.After(time.Now().UTC().Add(time.Minute)) ||
		snapshot.Result == "APPLIED" && (snapshot.RawQuota < 0 || snapshot.RawQuota > 1<<31-1) {
		return NativeQuotaSnapshot{}, ErrNativeQuotaDependency
	}
	snapshot.Amount = FormatAmount(snapshot.RawQuota)
	return snapshot, nil
}

func (port *NativeQuotaPort) ApplyRawQuotaDelta(ctx context.Context, id string, user, delta int64) (NativeQuotaReceipt, error) {
	if !ValidOperationKey(id) || !validateQuotaUser(user) || !validateQuotaDelta(delta) {
		return NativeQuotaReceipt{}, ErrInvalidMutation
	}
	var receipt NativeQuotaReceipt
	err := port.post(ctx, "/internal/momiao/quota/apply", nativeQuotaApplyRequest{
		OperationID: id, UserID: strconv.FormatInt(user, 10), DeltaRawQuota: strconv.FormatInt(delta, 10),
	}, &receipt)
	if err != nil {
		return NativeQuotaReceipt{}, err
	}
	return validateNativeQuotaReceipt(receipt, id, user, &delta)
}

func (port *NativeQuotaPort) QueryQuotaOperation(ctx context.Context, id string, user int64) (NativeQuotaReceipt, error) {
	if !ValidOperationKey(id) || !validateQuotaUser(user) {
		return NativeQuotaReceipt{}, ErrInvalidMutation
	}
	var receipt NativeQuotaReceipt
	if err := port.post(ctx, "/internal/momiao/quota/query", nativeQuotaQueryRequest{OperationID: id, UserID: strconv.FormatInt(user, 10)}, &receipt); err != nil {
		return NativeQuotaReceipt{}, err
	}
	return validateNativeQuotaReceipt(receipt, id, user, nil)
}

func (port *NativeQuotaPort) Credit(ctx context.Context, id string, user, amount int64) (NativeQuotaReceipt, error) {
	if amount <= 0 {
		return NativeQuotaReceipt{}, ErrInvalidMutation
	}
	return port.ApplyRawQuotaDelta(ctx, id, user, amount)
}
