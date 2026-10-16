package relay

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/relayconvert"
	"github.com/QuantumNous/new-api/relaykit/relayconvert/convmeta"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/cy4268/momiao/internal/platform"
	"github.com/tidwall/gjson"
)

type responseResult struct {
	usage           platform.RelayUsage
	reported, wrote bool
	failure         string
	detail          *upstreamError
}

// Claude's message_delta omits start usage fields. Decode into the same usage
// value so omitted input/cache counters survive while cumulative output updates.
type responseUsage struct {
	claude   dto.ClaudeUsage
	value    platform.RelayUsage
	reported bool
}

func (u *responseUsage) decode(raw []byte, format types.RelayFormat, stream bool) (any, bool, error) {
	var value any
	switch format {
	case types.RelayFormatOpenAI:
		if stream {
			value = &dto.ChatCompletionsStreamResponse{}
		} else {
			value = &dto.OpenAITextResponse{}
		}
	case types.RelayFormatOpenAIResponses:
		if stream {
			value = &dto.ResponsesStreamResponse{}
		} else {
			value = &dto.OpenAIResponsesResponse{}
		}
	case types.RelayFormatClaude:
		value = &dto.ClaudeResponse{}
	case types.RelayFormatGemini:
		value = &dto.GeminiChatResponse{}
	}
	if err := json.Unmarshal(raw, value); err != nil {
		return nil, false, err
	}
	var usage *dto.Usage
	terminal := false
	switch v := value.(type) {
	case *dto.OpenAITextResponse:
		if gjson.GetBytes(raw, "usage").IsObject() {
			usage = &v.Usage
		}
	case *dto.ChatCompletionsStreamResponse:
		usage = v.Usage
		for _, choice := range v.Choices {
			if choice.FinishReason != nil && *choice.FinishReason != "" {
				terminal = true
			}
		}
	case *dto.OpenAIResponsesResponse:
		if v.Usage != nil {
			usage = relayconvert.UsageFromResponsesUsage(v.Usage)
		}
	case *dto.ResponsesStreamResponse:
		terminal = v.Type == "response.completed" || v.Type == "response.incomplete" || v.Type == "response.failed"
		if v.Response != nil && v.Response.Usage != nil {
			usage = relayconvert.UsageFromResponsesUsage(v.Response.Usage)
		}
	case *dto.ClaudeResponse:
		path := "usage"
		if v.Type == "message_start" {
			path = "message.usage"
		}
		if data := gjson.GetBytes(raw, path); data.IsObject() {
			if err := json.Unmarshal([]byte(data.Raw), &u.claude); err != nil {
				return nil, false, err
			}
			usage = relayconvert.UsageFromClaudeAPIUsage(&u.claude)
			if v.Type == "message_delta" {
				merged := u.claude
				v.Usage = &merged
			}
		}
		terminal = v.Type == "message_stop"
	case *dto.GeminiChatResponse:
		if data := v.GetUsageMetadata(); data != nil {
			usage = relayconvert.UsageFromGeminiMetadata(data, 0)
		}
		for _, candidate := range v.Candidates {
			if candidate.FinishReason != nil && *candidate.FinishReason != "" {
				terminal = true
			}
		}
	}
	if usage != nil {
		details := usage.PromptTokensDetails
		if usage.InputTokensDetails != nil {
			details = *usage.InputTokensDetails
		}
		// DeepSeek's native cache-hit field is an official OpenAI usage extension.
		cacheRead := max(details.CachedTokens, usage.PromptCacheHitTokens)
		normalized := platform.RelayUsage{Prompt: int64(max(usage.PromptTokens, usage.InputTokens)), Completion: int64(max(usage.CompletionTokens, usage.OutputTokens)),
			CacheRead: int64(cacheRead), CacheWrite: int64(details.CacheCreationTokensTotal()), CacheWrite1h: int64(usage.ClaudeCacheCreation1hTokens)}
		if normalized.Prompt < 0 || normalized.Completion < 0 || normalized.CacheRead < 0 || normalized.CacheWrite < 0 || normalized.CacheWrite1h < 0 {
			return nil, false, errors.New("invalid upstream usage")
		}
		u.value, u.reported = normalized, true
	}
	if e := gjson.GetBytes(raw, "error"); e.Exists() && e.Type != gjson.Null {
		return value, terminal, errors.New("upstream error")
	}
	if gjson.GetBytes(raw, "type").String() == "error" || gjson.GetBytes(raw, "type").String() == "response.failed" || gjson.GetBytes(raw, "status").String() == "failed" {
		return value, terminal, errors.New("upstream error")
	}
	return value, terminal, nil
}

func forwardResponse(ctx context.Context, w http.ResponseWriter, response *http.Response, from, to types.RelayFormat, meta *convmeta.Values, stream bool, id, secret string, output *streamResponse) (result responseResult) {
	var usage responseUsage
	defer func() {
		result.usage, result.reported = usage.value, usage.reported
		if result.failure != "" && result.detail == nil {
			result.detail = transportFailure(ctx.Err(), result.failure)
		}
		if result.detail != nil && (result.detail.Status == 504 || result.detail.Status == 499) {
			result.failure = result.detail.Type
		}
	}()
	upstreamFailure := func(raw []byte) {
		if detail := parseUpstreamError(raw, 0, secret); detail != nil {
			result.detail = detail
			result.failure = fmt.Sprintf("upstream_http_%d", detail.Status)
		}
	}
	if !stream {
		w.Header().Set("Cache-Control", "no-store")
		raw, err := io.ReadAll(response.Body)
		if err != nil {
			result.failure = "upstream_read_failed"
			result.detail = transportFailure(err, result.failure)
			return
		}
		value, _, err := usage.decode(raw, from, false)
		if err != nil {
			result.failure = "upstream_response_invalid"
			upstreamFailure(raw)
			return
		}
		if from != to {
			converted, err := relayconvert.ConvertResponse(ctx, meta, to, value)
			if err != nil {
				result.failure = "response_conversion_failed"
				return
			}
			raw, err = json.Marshal(converted.Value)
			if err != nil {
				result.failure = "response_conversion_failed"
				return
			}
		}
		w.Header().Set("Content-Type", "application/json")
		result.wrote = true
		w.WriteHeader(response.StatusCode)
		if _, err = w.Write(raw); err != nil {
			result.failure = "client_write_failed"
		}
		return
	}
	if !strings.HasPrefix(response.Header.Get("Content-Type"), "text/event-stream") {
		result.failure = "upstream_stream_expected"
		return
	}
	options := relayconvert.ResponseStreamOptions{ID: id, Model: meta.OriginModelName, Created: time.Now().Unix(), IncludeUsage: true}
	conversionFrom := from
	var gemini *geminiArgumentStream
	if from == types.RelayFormatGemini && to != types.RelayFormatGemini {
		state, err := relayconvert.NewResponseStreamState(from, types.RelayFormatOpenAI, options)
		if err != nil {
			result.failure = "response_conversion_failed"
			return
		}
		gemini = &geminiArgumentStream{state: state, candidates: make(map[int]*geminiCalls)}
		conversionFrom = types.RelayFormatOpenAI
	}
	state, err := relayconvert.NewResponseStreamState(conversionFrom, to, options)
	if err != nil {
		result.failure = "response_conversion_failed"
		return
	}
	if output == nil {
		// Non-/v1 callers (channel tests) already have confirmed upstream SSE.
		output = newStreamResponse(ctx, w, requestTimeout)
		defer output.close()
	}
	write := func(raw []byte) error {
		var err error
		result.wrote, err = output.write(raw, response.StatusCode)
		return err
	}
	// Both callers have already handled non-2xx responses and channel retries.
	// Flush confirmed SSE headers without waiting for the first model event.
	if err := write(nil); err != nil {
		result.failure = "stream_forward_failed"
		return
	}
	emit := func(values []relayconvert.ResponseResult) error {
		for _, value := range values {
			raw, err := json.Marshal(value.Value)
			if err != nil {
				return err
			}
			prefix := ""
			if to != types.RelayFormatOpenAI {
				prefix = "event: " + gjson.GetBytes(raw, "type").String() + "\n"
			}
			if err := write(append([]byte(prefix+"data: "), append(raw, '\n', '\n')...)); err != nil {
				return err
			}
		}
		return nil
	}
	emitConverted := func(values []relayconvert.ResponseResult) error {
		if gemini == nil {
			return emit(values)
		}
		for _, value := range values {
			converted, err := relayconvert.ConvertStreamResponseChunk(ctx, meta, state, value.Value)
			if err != nil {
				return err
			}
			if err := emit(converted); err != nil {
				return err
			}
		}
		return nil
	}
	type streamFrame struct {
		raw, data []byte
		err       error
	}
	frames := make(chan streamFrame)
	stop, readerDone := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(readerDone)
		reader := bufio.NewReader(response.Body)
		// Only one pending event is buffered, never the entire reply. Reader has
		// no Scanner's 64 KiB token limit (tool arguments can be larger).
		for {
			var frame, data bytes.Buffer
			var readErr error
			for {
				line, e := reader.ReadBytes('\n')
				frame.Write(line)
				trimmed := bytes.TrimRight(line, "\r\n")
				if bytes.HasPrefix(trimmed, []byte("data:")) {
					if data.Len() > 0 {
						data.WriteByte('\n')
					}
					data.Write(bytes.TrimPrefix(bytes.TrimPrefix(trimmed, []byte("data:")), []byte(" ")))
				}
				if e != nil {
					readErr = e
					break
				}
				if len(trimmed) == 0 {
					break
				}
			}
			select {
			case frames <- streamFrame{frame.Bytes(), data.Bytes(), readErr}:
			case <-stop:
				return
			}
			if readErr != nil {
				return
			}
		}
	}()
	defer func() {
		close(stop)
		_ = response.Body.Close() // Unblock a pending HTTP body read on cancellation.
		<-readerDone
	}()
	terminal, done := false, false
readLoop:
	for !done {
		var frame streamFrame
		// The shared committer serializes body frames with idle comments.
		select {
		case <-output.ctx.Done():
			result.failure = "upstream_stream_interrupted"
			result.detail = transportFailure(output.ctx.Err(), result.failure)
			break readLoop
		case frame = <-frames:
		}
		if len(frame.raw) == 0 && frame.err == io.EOF {
			break
		}
		if frame.err != nil && frame.err != io.EOF {
			result.failure = "upstream_stream_interrupted"
			result.detail = transportFailure(frame.err, result.failure)
			break
		}
		raw := frame.data
		if bytes.Equal(raw, []byte("[DONE]")) {
			terminal, done = true, true
			if from == to {
				err = write(frame.raw)
			}
		} else if len(raw) > 0 {
			value, last, decodeErr := usage.decode(raw, from, true)
			terminal = terminal || last
			if decodeErr != nil {
				result.failure = "upstream_stream_invalid"
				upstreamFailure(raw)
				break
			}
			if from == to {
				err = write(frame.raw)
			} else {
				var converted []relayconvert.ResponseResult
				if gemini != nil {
					converted, err = gemini.convert(ctx, meta, raw, value)
				} else {
					converted, err = relayconvert.ConvertStreamResponseChunk(ctx, meta, state, value)
				}
				if err == nil {
					err = emitConverted(converted)
				}
			}
			if last && (from == types.RelayFormatClaude || from == types.RelayFormatOpenAIResponses) {
				done = true
			}
		} else if len(frame.raw) > 0 {
			err = write(frame.raw) // SSE comments/keepalives, without inventing data.
		}
		if err != nil {
			result.failure = "stream_forward_failed"
			break
		}
		if frame.err == io.EOF {
			break
		}
	}
	if result.failure == "" && !terminal {
		result.failure = "upstream_stream_incomplete"
	}
	if result.failure == "" && from != to {
		var values []relayconvert.ResponseResult
		if gemini != nil {
			values, err = gemini.finalize(ctx, meta)
			if err == nil {
				err = emitConverted(values)
			}
		}
		if err == nil {
			values, err = relayconvert.FinalizeStreamResponse(ctx, meta, state)
		}
		if err == nil {
			err = emit(values)
		}
		if err == nil && to == types.RelayFormatOpenAI {
			err = write([]byte("data: [DONE]\n\n"))
		}
		if err != nil {
			result.failure = "stream_finalize_failed"
		}
	}
	if result.failure != "" && result.wrote && !errors.Is(ctx.Err(), context.Canceled) {
		if result.detail == nil {
			result.detail = transportFailure(ctx.Err(), result.failure)
		}
		output.fail(result.detail, to)
	}
	return
}
