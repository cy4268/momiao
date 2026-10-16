package relay

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/QuantumNous/new-api/relaykit/relayconvert/convmeta"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/cy4268/momiao/internal/platform"
)

// Exercise the same response reader used by paid requests and channel tests.
func TestChannelErrorFeedback(t *testing.T) {
	if requestTimeout != 330*time.Second {
		t.Errorf("business timeout=%s, want 330s", requestTimeout)
	}
	const secret = "synthetic-channel-key"
	const quota = `{"error":{"code":429,"type":"RESOURCE_EXHAUSTED","message":"quota exceeded synthetic-channel-key ?key=synthetic-google-key"}}`
	for _, tc := range []struct {
		name, body string
		status     int
		stream     bool
	}{
		{"http429", quota, 429, false},
		{"json_error", quota, 200, false},
		{"stream_error", "data: " + quota + "\n\n", 200, true},
		{"heartbeat_then_error", ": keep-alive\n\ndata: " + quota + "\n\n", 200, true},
		{"partial_then_error", "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"hello\"}}],\"usage\":{\"prompt_tokens\":7,\"completion_tokens\":1}}\n\ndata: " + quota + "\n\n", 200, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if tc.stream {
					w.Header().Set("Content-Type", "text/event-stream")
				}
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.body)
			}))
			defer server.Close()
			channel := platform.RelayChannel{Type: 1, BaseURL: server.URL, Key: secret, UpstreamModel: "example"}
			result := TestChannel(context.Background(), channel, ChannelTestOptions{Model: "example", Stream: tc.stream})
			if result.OK || result.Status != 429 || !strings.Contains(result.Message, "quota exceeded") || strings.Contains(result.Message, "synthetic-") {
				t.Errorf("feedback=%+v; want redacted upstream 429", result)
			}
			if tc.stream && strings.HasPrefix(tc.body, "data: {\"choices\"") {
				res, _ := http.Get(server.URL)
				defer res.Body.Close()
				recorder := httptest.NewRecorder()
				response := forwardResponse(context.Background(), recorder, res, types.RelayFormatOpenAI, types.RelayFormatOpenAI, &convmeta.Values{OriginModelName: "example"}, true, "test", secret)
				if !response.reported || response.usage.Prompt != 7 || response.usage.Completion != 1 {
					t.Errorf("partial usage lost: %+v", response)
				}
				if !strings.Contains(recorder.Body.String(), `"code":429`) || strings.Contains(recorder.Body.String(), "[DONE]") {
					t.Errorf("stream error lost: %s", recorder.Body.String())
				}
			}
		})
	}
	t.Run("timeout", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = io.Copy(io.Discard, r.Body)
			select {
			case <-r.Context().Done():
			case <-time.After(time.Second):
			}
		}))
		defer server.Close()
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()
		result := TestChannel(ctx, platform.RelayChannel{Type: 1, BaseURL: server.URL, Key: secret, UpstreamModel: "example"}, ChannelTestOptions{Model: "example"})
		if result.OK || result.Status != 504 {
			t.Errorf("timeout=%+v; want 504", result)
		}
	})
}
