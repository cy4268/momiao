package relay

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/cy4268/momiao/internal/platform"
	"github.com/google/uuid"
	"github.com/tidwall/gjson"
)

const requestTimeout = 5 * time.Minute

func Handler(store *platform.Store) http.Handler {
	client := &http.Client{Timeout: requestTimeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		format := requestFormat(r.URL.Path)
		if format == "" {
			writeError(w, 404, "unknown relay endpoint")
			return
		}
		if r.Method != http.MethodPost {
			w.Header().Set("Allow", "POST")
			writeError(w, 405, "method not allowed")
			return
		}
		key := apiKey(r)
		if key == "" {
			writeError(w, 401, "invalid API key")
			return
		}
		if store == nil {
			writeError(w, 503, "relay unavailable")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), requestTimeout)
		defer cancel()
		deadline, _ := ctx.Deadline()
		controller := http.NewResponseController(w)
		_ = controller.SetReadDeadline(deadline)
		_ = controller.SetWriteDeadline(deadline)
		body, err := io.ReadAll(r.Body)
		var request struct {
			Model    string `json:"model"`
			Stream   bool   `json:"stream"`
			Duration *int64 `json:"duration"`
		}
		if err != nil || json.Unmarshal(body, &request) != nil || request.Model == "" || (request.Duration != nil && *request.Duration < 0) {
			writeError(w, 400, "invalid generation request")
			return
		}
		route, err := store.PrepareRelay(ctx, key, request.Model, nil)
		if err != nil {
			admissionError(w, err)
			return
		}
		identity := route.Identity
		settlement := platform.RelaySettlement{
			RequestID: uuid.NewString(), UserID: identity.UserID, KeyID: identity.KeyID, KeyName: identity.KeyName,
			Purpose: identity.Purpose, Group: identity.Group, Model: request.Model, RequestKind: string(format),
			RequestedAt: time.Now().UTC(), IsStream: request.Stream, FinalStatus: "ERROR",
		}
		w.Header().Set("X-Request-ID", settlement.RequestID)
		var used []int64
		var usageReported bool
		// Billing must survive a disconnected client. Only reported usage is charged;
		// no token estimates and no request/response bodies enter logs.
		defer func() {
			billing, done := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
			defer done()
			settlement.CompletedAt = time.Now().UTC()
			settlement.DurationMS = settlement.CompletedAt.Sub(settlement.RequestedAt).Milliseconds()
			if settlement.FinalStatus == "SUCCESS" || usageReported {
				settlement.CostUnits, err = store.RelayCost(billing, request.Model, settlement.Group, settlement.Usage, request.Duration)
				if err != nil {
					slog.Error("relay price failed", "request_id", settlement.RequestID, "channel_id", settlement.ChannelID)
					return
				}
			}
			if !usageReported && settlement.FinalStatus == "SUCCESS" {
				settlement.Error = "usage_missing"
				slog.Warn("relay usage missing", "request_id", settlement.RequestID, "channel_id", settlement.ChannelID)
			}
			if _, err := store.SettleRelay(billing, settlement); err != nil {
				slog.Error("relay settlement failed", "request_id", settlement.RequestID, "channel_id", settlement.ChannelID)
			}
		}()
		for attempt := 1; attempt <= 3; attempt++ {
			attemptStarted := time.Now().UTC()
			channel := route.Channel
			settlement.ChannelID, settlement.Group = channel.ID, route.Identity.Group
			used = append(used, channel.ID)
			upstream, upstreamFormat, meta, err := upstreamRequest(ctx, r, body, format, request.Model, request.Stream, channel)
			if err != nil {
				settlement.StatusCode, settlement.Error = 400, "request_conversion_failed"
				writeError(w, 400, "request conversion or channel configuration failed")
				return
			}
			settlement.ProviderAttempts = attempt
			response, err := client.Do(upstream)
			if transportErr, ok := err.(*url.Error); ok {
				// Omit the request URL: Vertex credentials can be query parameters.
				slog.Warn("relay transport failed", "request_id", settlement.RequestID, "channel_id", channel.ID, "cause", transportErr.Err)
			}
			settlement.StatusCode, settlement.Error = 502, "upstream_connection_error"
			if err == nil {
				settlement.StatusCode = response.StatusCode
				settlement.Error = fmt.Sprintf("upstream_http_%d", response.StatusCode)
			}
			retryable := err != nil || response.StatusCode == 429 || response.StatusCode >= 500
			if retryable && attempt < 3 && ctx.Err() == nil {
				next, nextErr := store.PrepareRelay(ctx, key, request.Model, used)
				if nextErr == nil {
					if response != nil {
						response.Body.Close()
					}
					failed := settlement
					failed.RequestedAt, failed.DurationMS = attemptStarted, time.Since(attemptStarted).Milliseconds()
					if err := store.LogRelayAttempt(ctx, failed); err != nil {
						settlement.StatusCode, settlement.Error = 503, "attempt_log_failed"
						writeError(w, 503, "relay logging unavailable")
						return
					}
					route = next
					continue
				}
			}
			if err != nil {
				if ctx.Err() != nil {
					settlement.FinalStatus = "CANCELLED_POST_UPSTREAM"
				}
				writeError(w, 502, "upstream connection failed")
				return
			}
			defer response.Body.Close()
			if response.StatusCode < 200 || response.StatusCode >= 300 {
				message := settlement.Error
				if raw, err := io.ReadAll(response.Body); err == nil {
					if detail := gjson.GetBytes(raw, "error.message").String(); detail != "" {
						message = strings.ReplaceAll(detail, channel.Key, "[credential]")
					}
				}
				writeError(w, response.StatusCode, message)
				return
			}
			result := forwardResponse(ctx, w, response, upstreamFormat, format, meta, request.Stream, settlement.RequestID)
			settlement.Usage, usageReported = result.usage, result.reported
			settlement.Error = result.failure
			if result.failure == "" {
				settlement.FinalStatus = "SUCCESS"
			} else {
				settlement.StatusCode = 502
				if ctx.Err() != nil {
					settlement.FinalStatus = "CANCELLED_POST_UPSTREAM"
				}
				if !result.wrote {
					writeError(w, 502, result.failure)
				}
			}
			return
		}
	})
}

func requestFormat(path string) types.RelayFormat {
	switch path {
	case "/v1/chat/completions":
		return types.RelayFormatOpenAI
	case "/v1/responses":
		return types.RelayFormatOpenAIResponses
	case "/v1/messages":
		return types.RelayFormatClaude
	}
	return ""
}

func admissionError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, platform.ErrRelayKey):
		writeError(w, 401, "invalid API key")
	case errors.Is(err, platform.ErrRelayModel):
		writeError(w, 403, err.Error())
	case errors.Is(err, platform.ErrInsufficientBalance):
		writeError(w, 403, "Reserve balance is empty")
	case errors.Is(err, platform.ErrMaintenanceActive):
		writeError(w, 503, "maintenance active")
	case errors.Is(err, platform.ErrRelayChannel), errors.Is(err, platform.ErrRelayPrice):
		writeError(w, 503, err.Error())
	default:
		writeError(w, 503, "relay unavailable")
	}
}
