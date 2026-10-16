package relay

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
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
	"github.com/tidwall/gjson"
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
	// Gemini tool arguments must be forwarded before the model finishes, not
	// turned into a new call with null arguments for every upstream fragment.
	for _, tc := range []struct {
		channel             int
		stream, tools, want bool
	}{{24, true, true, true}, {41, true, true, true}, {41, false, true, false}, {41, true, false, false}, {1, true, true, false}} {
		body := fmt.Sprintf(`{"model":"example","messages":[{"role":"user","content":"hello"}],"stream":%t}`, tc.stream)
		if tc.tools {
			body = strings.TrimSuffix(body, "}") + `,"tools":[{"type":"function","function":{"name":"router_emit_test","parameters":{"type":"object","properties":{"content":{"type":"string"}}}}}]}`
		}
		channel := platform.RelayChannel{Type: tc.channel, BaseURL: "https://example.invalid", Key: "synthetic", UpstreamModel: "gemini-4-argon", Settings: json.RawMessage(`{"vertex_key_type":"api_key"}`), ParamOverride: json.RawMessage(`{"toolConfig":{"functionCallingConfig":{"mode":"ANY","allowedFunctionNames":["router_emit_test"]}}}`)}
		request, _, _, err := upstreamRequest(t.Context(), httptest.NewRequest("POST", "/v1/chat/completions", nil), []byte(body), types.RelayFormatOpenAI, "example", tc.stream, channel)
		if err != nil {
			t.Fatal(err)
		}
		raw, _ := io.ReadAll(request.Body)
		request.Body.Close()
		config := gjson.GetBytes(raw, "toolConfig.functionCallingConfig")
		if config.Get("streamFunctionCallArguments").Bool() != tc.want || config.Get("mode").String() != "ANY" || config.Get("allowedFunctionNames.0").String() != "router_emit_test" {
			t.Errorf("channel=%d stream=%t tools=%t: config=%s; want flag=%t and existing fields", tc.channel, tc.stream, tc.tools, config.Raw, tc.want)
		}
	}
	synctest.Test(t, func(t *testing.T) {
		body, upstream := io.Pipe()
		defer upstream.Close()
		recorder := httptest.NewRecorder()
		finished := make(chan responseResult, 1)
		go func() {
			finished <- forwardResponse(t.Context(), recorder, &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: body}, types.RelayFormatGemini, types.RelayFormatOpenAI, &convmeta.Values{OriginModelName: "example", UpstreamModelName: "gemini-4-argon"}, true, "test", "", nil)
		}()
		sendCall := func(call string) {
			t.Helper()
			if _, err := fmt.Fprintf(upstream, "data: {\"candidates\":[{\"index\":0,\"content\":{\"parts\":[{\"functionCall\":%s}]}}]}\n\n", call); err != nil {
				t.Fatal(err)
			}
			synctest.Wait()
		}
		sendCall(`{"id":"native1","name":"router_emit_test","willContinue":true}`)
		sendCall(`{"partialArgs":[{"jsonPath":"$.content","stringValue":"first \"\ud83d","willContinue":true}],"willContinue":true}`)
		if !strings.Contains(recorder.Body.String(), "first") {
			t.Error("first argument fragment was not delivered before upstream completion")
		}
		sendCall(`{"partialArgs":[{"jsonPath":"$.content","stringValue":"\ude00\ntail\\","willContinue":true}],"willContinue":true}`)
		// Google also sends a no-value fragment to close a streamed string.
		sendCall(`{"partialArgs":[{"jsonPath":"$.content"},{"jsonPath":"$.details[0].count","numberValue":2},{"jsonPath":"$.details[0]['enabled']","boolValue":false},{"jsonPath":"$.details[0].note","nullValue":null}],"willContinue":true}`)
		sendCall(`{}`)
		sendCall(`{"id":"native2","name":"lookup","args":{"q":"ok"}}`)
		_, _ = io.WriteString(upstream, "data: {\"candidates\":[{\"index\":0,\"finishReason\":\"STOP\"}],\"usageMetadata\":{\"promptTokenCount\":7,\"candidatesTokenCount\":1,\"totalTokenCount\":8}}\n\n")
		upstream.Close()
		result := <-finished
		arguments, ids, names := map[int]string{}, map[int]string{}, map[int]string{}
		finish := ""
		for _, line := range strings.Split(recorder.Body.String(), "\n") {
			raw := strings.TrimPrefix(line, "data: ")
			for _, choice := range gjson.Get(raw, "choices").Array() {
				if reason := choice.Get("finish_reason").String(); reason != "" {
					finish = reason
				}
				for _, call := range choice.Get("delta.tool_calls").Array() {
					i := int(call.Get("index").Int())
					arguments[i] += call.Get("function.arguments").String()
					ids[i] += call.Get("id").String()
					names[i] += call.Get("function.name").String()
				}
			}
		}
		first := gjson.Parse(arguments[0])
		if result.failure != "" || !result.reported || result.usage.Prompt != 7 || result.usage.Completion != 1 || finish != "tool_calls" || !strings.Contains(recorder.Body.String(), "data: [DONE]") {
			t.Errorf("incremental stream result=%+v finish=%q", result, finish)
		}
		if len(arguments) != 2 || !json.Valid([]byte(arguments[0])) || first.Get("content").String() != "first \"😀\ntail\\" || first.Get("details.0.count").Int() != 2 || first.Get("details.0.enabled").Raw != "false" || first.Get("details.0.note").Raw != "null" || arguments[1] != `{"q":"ok"}` || ids[0] != "native1" || ids[1] != "native2" || names[0] != "router_emit_test" || names[1] != "lookup" {
			t.Errorf("corrupt tool deltas: args=%v ids=%v names=%v", arguments, ids, names)
		}
	})
	// The same Gemini flag is used by all downstream protocols. Check the two
	// composed converters plus ordinary text and incomplete arguments here too.
	const toolStream = "data: " + `{"candidates":[{"index":0,"content":{"parts":[{"functionCall":{"name":"emit","partialArgs":[{"jsonPath":"$.content","stringValue":"hello"}]}}]}}]}` + "\n\n"
	const stopStream = "data: " + `{"candidates":[{"index":0,"finishReason":"STOP"}]}` + "\n\n"
	for _, tc := range []struct {
		target       types.RelayFormat
		body, marker string
		incomplete   bool
	}{
		{types.RelayFormatClaude, toolStream + stopStream, `"partial_json":"{\"content\":\"hello\"}"`, false},
		{types.RelayFormatOpenAIResponses, toolStream + stopStream, `"arguments":"{\"content\":\"hello\"}"`, false},
		{types.RelayFormatOpenAI, "data: " + `{"candidates":[{"index":0,"content":{"parts":[{"text":"hello"}]},"finishReason":"STOP"}]}` + "\n\n", `"content":"hello"`, false},
		{types.RelayFormatOpenAI, strings.Replace(toolStream, `"name":"emit",`, `"name":"emit","willContinue":true,`, 1) + stopStream, `"type":"error"`, true},
	} {
		recorder := httptest.NewRecorder()
		result := forwardResponse(t.Context(), recorder, &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"text/event-stream"}}, Body: io.NopCloser(strings.NewReader(tc.body))}, types.RelayFormatGemini, tc.target, &convmeta.Values{OriginModelName: "example", UpstreamModelName: "gemini-4-argon"}, true, "test", "", nil)
		if (result.failure != "") != tc.incomplete || !strings.Contains(recorder.Body.String(), tc.marker) || (tc.incomplete && strings.Contains(recorder.Body.String(), "[DONE]")) {
			t.Errorf("Gemini to %s incomplete=%t: result=%+v body=%s", tc.target, tc.incomplete, result, recorder.Body.String())
		}
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
