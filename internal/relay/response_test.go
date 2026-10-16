package relay

import (
	"bufio"
	"context"
	"io"
	"net"
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

func TestDelayedStreamHeaders(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		recorder := httptest.NewRecorder()
		output := newStreamResponse(t.Context(), recorder, 20*time.Second)
		defer output.close()
		clientConn, serverConn := net.Pipe()
		defer clientConn.Close()
		defer serverConn.Close()
		transport := &http.Transport{DialContext: func(context.Context, string, string) (net.Conn, error) { return clientConn, nil }}
		defer transport.CloseIdleConnections()
		client := &http.Client{Transport: transport}
		go func() {
			request, err := http.ReadRequest(bufio.NewReader(serverConn))
			if err != nil {
				t.Error(err)
				return
			}
			request.Body.Close()
			time.Sleep(30 * time.Second)
			_, _ = io.WriteString(serverConn, "HTTP/1.1 200 OK\r\nContent-Type: text/event-stream\r\nConnection: close\r\n\r\n")
			time.Sleep(120 * time.Second)
			_, _ = io.WriteString(serverConn, "data: {\"choices\":[{\"index\":0,\"delta\":{\"content\":\"hello\"},\"finish_reason\":\"stop\"}],\"usage\":{\"prompt_tokens\":7,\"completion_tokens\":1}}\n\ndata: [DONE]\n\n")
			serverConn.Close()
		}()
		finished, headers := make(chan responseResult, 1), make(chan struct{})
		go func() {
			request, _ := http.NewRequestWithContext(output.ctx, "POST", "http://upstream.invalid/v1/chat/completions", nil)
			response, err := client.Do(request)
			if err != nil {
				t.Error(err)
				finished <- responseResult{failure: "transport"}
				return
			}
			close(headers)
			finished <- forwardResponse(output.ctx, recorder, response, types.RelayFormatOpenAI, types.RelayFormatOpenAI, &convmeta.Values{OriginModelName: "example"}, true, "test", "", output)
		}()
		synctest.Wait()
		time.Sleep(19 * time.Second)
		synctest.Wait()
		if recorder.Flushed || recorder.Body.Len() != 0 {
			t.Fatal("stream committed before the grace period")
		}
		time.Sleep(time.Second)
		synctest.Wait()
		select {
		case <-headers:
			t.Fatal("upstream headers arrived before the delayed-header check")
		default:
		}
		if !recorder.Flushed || recorder.Code != 200 || recorder.Body.String() != ": ping\n\n" || recorder.Header().Get("Content-Type") != "text/event-stream" || recorder.Header().Get("X-Accel-Buffering") != "no" {
			t.Fatalf("no keepalive while client.Do waits: %v %q", recorder.Header(), recorder.Body.String())
		}
		time.Sleep(120 * time.Second) // t=140: headers at 30 reset the same idle timer.
		synctest.Wait()
		if strings.Count(recorder.Body.String(), ": ping\n\n") != 8 {
			t.Fatalf("duplicated or missing heartbeat after handoff: %q", recorder.Body.String())
		}
		result := <-finished
		output.close()
		if result.failure != "" || !result.reported || result.usage.Prompt != 7 || result.usage.Completion != 1 || strings.Count(recorder.Body.String(), "[DONE]") != 1 {
			t.Fatalf("150s stream did not finish normally: %+v %q", result, recorder.Body.String())
		}
		completed := recorder.Body.String()
		time.Sleep(30 * time.Second)
		synctest.Wait()
		if recorder.Body.String() != completed {
			t.Fatal("heartbeat continued after request completion")
		}
		// Fast failures retain the real HTTP status; late failures use SSE error
		// detail and never emit DONE. Neither path may leave a heartbeat running.
		for _, tc := range []struct {
			delay  time.Duration
			format types.RelayFormat
		}{
			{time.Second, types.RelayFormatOpenAI}, {30 * time.Second, types.RelayFormatOpenAI},
			{30 * time.Second, types.RelayFormatClaude}, {30 * time.Second, types.RelayFormatOpenAIResponses},
		} {
			delay := tc.delay
			r := httptest.NewRecorder()
			s := newStreamResponse(t.Context(), r, 20*time.Second)
			time.Sleep(delay)
			synctest.Wait()
			s.fail(&upstreamError{Status: 429, Message: "quota exhausted", Type: "RESOURCE_EXHAUSTED", Code: 429}, tc.format)
			if s.ctx.Err() != nil || (tc.format != types.RelayFormatOpenAI && !strings.Contains(r.Body.String(), "event: error\n")) {
				t.Errorf("provider error changed cancellation or event format: %v %q", s.ctx.Err(), r.Body.String())
			}
			s.close()
			before := r.Body.String()
			time.Sleep(30 * time.Second)
			synctest.Wait()
			if !strings.Contains(before, `"message":"quota exhausted"`) || strings.Contains(before, "[DONE]") || r.Body.String() != before || (delay < 20*time.Second && (r.Code != 429 || r.Header().Get("Content-Type") != "application/json")) || (delay > 20*time.Second && (r.Code != 200 || !strings.Contains(before, "data: {\"error\":"))) {
				t.Errorf("failure after %s: status=%d headers=%v body=%q", delay, r.Code, r.Header(), before)
			}
		}
		ctx, cancel := context.WithTimeout(t.Context(), 40*time.Second)
		defer cancel()
		r := httptest.NewRecorder()
		s := newStreamResponse(ctx, r, 20*time.Second)
		defer s.close()
		body, upstream := io.Pipe()
		defer upstream.Close()
		go func() {
			finished <- forwardResponse(s.ctx, r, &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: body}, types.RelayFormatOpenAI, types.RelayFormatOpenAI, &convmeta.Values{OriginModelName: "example"}, true, "deadline", "", s)
		}()
		result = <-finished
		if result.detail == nil || result.detail.Status != 504 || !strings.Contains(r.Body.String(), `"code":504`) || strings.Contains(r.Body.String(), "[DONE]") {
			t.Errorf("deadline lost its final SSE error: %+v %q", result, r.Body.String())
		}
	})
}

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
			finished <- forwardResponse(t.Context(), recorder, response, types.RelayFormatOpenAI, types.RelayFormatOpenAI, &convmeta.Values{OriginModelName: "example"}, true, "test", "", nil)
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
			finished <- forwardResponse(ctx, recorder, response, types.RelayFormatOpenAI, types.RelayFormatOpenAI, &convmeta.Values{OriginModelName: "example"}, true, "test", "", nil)
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
				response := forwardResponse(context.Background(), recorder, res, types.RelayFormatOpenAI, types.RelayFormatOpenAI, &convmeta.Values{OriginModelName: "example"}, true, "test", secret, nil)
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
