package relay

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/synctest"
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
	// Reuse this response-path test without spending upstream quota or real time.
	synctest.Test(t, func(t *testing.T) {
		body, upstream := io.Pipe()
		defer upstream.Close()
		recorder := httptest.NewRecorder()
		response := &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: body}
		finished := make(chan responseResult, 1)
		go func() {
			finished <- forwardResponse(t.Context(), recorder, response, types.RelayFormatOpenAI, types.RelayFormatOpenAI, &convmeta.Values{OriginModelName: "example"}, true, "test", "")
		}()
		synctest.Wait()
		if !recorder.Flushed || recorder.Code != 200 || recorder.Header().Get("Content-Type") != "text/event-stream" || recorder.Body.Len() != 0 {
			t.Fatal("confirmed SSE headers were not flushed before the first upstream event")
		}
		time.Sleep(14 * time.Second)
		synctest.Wait()
		if recorder.Body.Len() != 0 {
			t.Fatal("heartbeat arrived before the idle interval")
		}
		time.Sleep(time.Second)
		synctest.Wait()
		if recorder.Body.String() != ": ping\n\n" {
			t.Fatalf("idle heartbeat = %q", recorder.Body.String())
		}
		time.Sleep(5 * time.Second)
		const partial = "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"hello\"}}],\"usage\":{\"prompt_tokens\":7,\"completion_tokens\":1}}\n\n"
		if _, err := io.WriteString(upstream, partial); err != nil {
			t.Fatal(err)
		}
		synctest.Wait()
		time.Sleep(14 * time.Second)
		synctest.Wait()
		if recorder.Body.String() != ": ping\n\n"+partial {
			t.Fatal("normal output did not reset the idle heartbeat")
		}
		// Reach 140 seconds: one initial ping and eight since the data at t=20.
		time.Sleep(106 * time.Second)
		synctest.Wait()
		if strings.Count(recorder.Body.String(), ": ping\n\n") != 9 {
			t.Fatalf("keepalive stopped during the long upstream pause: %q", recorder.Body.String())
		}
		if _, err := io.WriteString(upstream, "data: [DONE]\n\n"); err != nil {
			t.Fatal(err)
		}
		result := <-finished
		if result.failure != "" || !result.reported || result.usage.Prompt != 7 || result.usage.Completion != 1 {
			t.Fatalf("stream completion/usage changed: %+v", result)
		}
		completed := recorder.Body.String()
		time.Sleep(30 * time.Second)
		synctest.Wait()
		if recorder.Body.String() != completed {
			t.Fatal("heartbeat continued after stream completion")
		}
		// Cancellation must also unblock a pending upstream read and stop output.
		ctx, cancel := context.WithCancel(t.Context())
		body, upstream = io.Pipe()
		defer upstream.Close()
		response.Body = body
		go func() {
			finished <- forwardResponse(ctx, recorder, response, types.RelayFormatOpenAI, types.RelayFormatOpenAI, &convmeta.Values{OriginModelName: "example"}, true, "test", "")
		}()
		synctest.Wait()
		cancel()
		result = <-finished
		if result.detail == nil || result.detail.Status != 499 {
			t.Fatalf("cancelled stream = %+v; want 499", result)
		}
		time.Sleep(30 * time.Second)
		synctest.Wait()
		if recorder.Body.String() != completed {
			t.Fatal("heartbeat continued after cancellation")
		}
	})
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
