package relay

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strconv"
	"strings"

	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/relayconvert"
	"github.com/QuantumNous/new-api/relaykit/relayconvert/convmeta"
	"github.com/google/uuid"
	"github.com/tidwall/gjson"
)

// The pinned RelayKit predates partialArgs. Keep its text/usage/finish handling,
// but replace function-call deltas before any OpenAI -> downstream conversion.
type geminiArgumentStream struct {
	state      *relayconvert.ResponseStreamState
	candidates map[int]*geminiCalls
}

type geminiCalls struct {
	calls   []*geminiCall
	current *geminiCall
}

type geminiCall struct {
	id, name  string
	index     int
	done      bool
	arguments partialJSON
}

func (s *geminiArgumentStream) convert(ctx context.Context, meta *convmeta.Values, raw []byte, value any) ([]relayconvert.ResponseResult, error) {
	deltas := make(map[int][]dto.ToolCallResponse)
	for _, candidate := range gjson.GetBytes(raw, "candidates").Array() {
		index := int(candidate.Get("index").Int())
		calls := s.candidates[index]
		for _, part := range candidate.Get("content.parts").Array() {
			fc := part.Get("functionCall")
			if !fc.IsObject() {
				continue
			}
			if calls == nil {
				calls = &geminiCalls{}
				s.candidates[index] = calls
			}
			delta, err := calls.append(fc)
			if err != nil {
				return nil, err
			}
			deltas[index] = append(deltas[index], delta)
		}
		if candidate.Get("finishReason").String() != "" && calls != nil && !calls.complete() {
			return nil, errors.New("unfinished Gemini function arguments")
		}
	}
	converted, err := relayconvert.ConvertStreamResponseChunk(ctx, meta, s.state, value)
	if err != nil {
		return nil, err
	}
	for _, result := range converted {
		chunk := result.Value.(*dto.ChatCompletionsStreamResponse)
		for i := range chunk.Choices {
			choice := &chunk.Choices[i]
			if len(choice.Delta.ToolCalls) > 0 {
				choice.Delta.ToolCalls = deltas[choice.Index]
			}
		}
	}
	return converted, nil
}

func (s *geminiArgumentStream) finalize(ctx context.Context, meta *convmeta.Values) ([]relayconvert.ResponseResult, error) {
	for _, calls := range s.candidates {
		if !calls.complete() {
			return nil, errors.New("unfinished Gemini function arguments")
		}
	}
	return relayconvert.FinalizeStreamResponse(ctx, meta, s.state)
}

func (s *geminiCalls) complete() bool {
	for _, call := range s.calls {
		if !call.done {
			return false
		}
	}
	return true
}

func (s *geminiCalls) append(fc gjson.Result) (dto.ToolCallResponse, error) {
	var delta dto.ToolCallResponse
	id, name := fc.Get("id").String(), fc.Get("name").String()
	call := s.current
	if id != "" {
		call = nil
		for _, existing := range s.calls {
			if existing.id == id {
				call = existing
				break
			}
		}
	}
	if call == nil {
		if name == "" {
			return delta, errors.New("Gemini function continuation has no active call")
		}
		if id == "" {
			id = "call_" + uuid.NewString()
		}
		call = &geminiCall{id: id, name: name, index: len(s.calls)}
		s.calls = append(s.calls, call)
		delta.ID, delta.Type, delta.Function.Name = id, "function", name
	} else if call.done || (name != "" && name != call.name) {
		return delta, errors.New("ambiguous Gemini function continuation")
	}
	s.current = call
	delta.SetIndex(call.index)
	var out strings.Builder
	if args := fc.Get("args"); args.Exists() {
		if !args.IsObject() || len(call.arguments.stack) != 0 || fc.Get("partialArgs").Exists() || fc.Get("willContinue").Bool() {
			return delta, errors.New("invalid complete Gemini function arguments")
		}
		out.WriteString(args.Raw)
	} else {
		for _, partial := range fc.Get("partialArgs").Array() {
			if err := call.arguments.append(&out, partial); err != nil {
				return delta, err
			}
		}
		if !fc.Get("willContinue").Bool() {
			if err := call.arguments.close(&out); err != nil {
				return delta, err
			}
		}
	}
	if !fc.Get("willContinue").Bool() {
		call.done, s.current = true, nil
	}
	delta.Function.Arguments = out.String()
	return delta, nil
}

// PartialArg uses concrete JSON paths, not JSONPath queries. Serialize the
// model's depth-first fragments as JSON without buffering the reply text.
// ponytail: reject path revisits; buffer a subtree only if the upstream needs it.
// https://docs.cloud.google.com/gemini-enterprise-agent-platform/reference/rest/v1/Content#PartialArg
type argumentPath struct {
	key   string
	index int
	array bool
}

type argumentContainer struct {
	at    argumentPath
	array bool
	count int
	keys  map[string]bool
}

type partialJSON struct {
	stack      []argumentContainer
	stringPath []argumentPath
}

func (s *partialJSON) append(out *strings.Builder, partial gjson.Result) error {
	path, err := parseArgumentPath(partial.Get("jsonPath").String())
	if err != nil {
		return err
	}
	var value gjson.Result
	kind := ""
	for _, field := range []string{"stringValue", "numberValue", "boolValue", "nullValue"} {
		if v := partial.Get(field); v.Exists() {
			if kind != "" {
				return errors.New("multiple Gemini partial argument values")
			}
			kind, value = field, v
		}
	}
	continuing := partial.Get("willContinue").Bool()
	if s.stringPath != nil {
		if !slices.Equal(path, s.stringPath) || (kind != "stringValue" && kind != "") || (kind == "" && continuing) {
			return errors.New("interrupted Gemini string argument")
		}
	} else {
		if kind == "" {
			return errors.New("Gemini partial argument has no value")
		}
		if err := s.startValue(out, path); err != nil {
			return err
		}
		if kind == "stringValue" {
			out.WriteByte('"')
			s.stringPath = path
		}
	}
	if s.stringPath != nil {
		if kind != "" {
			if value.Type != gjson.String {
				return errors.New("invalid Gemini string argument")
			}
			// Preserve raw escapes: decoding each fragment would replace a split
			// UTF-16 surrogate pair with two replacement characters.
			out.WriteString(value.Raw[1 : len(value.Raw)-1])
		}
		if !continuing {
			out.WriteByte('"')
			s.stringPath = nil
		}
		return nil
	}
	if continuing || (kind == "numberValue" && value.Type != gjson.Number) || (kind == "boolValue" && value.Type != gjson.True && value.Type != gjson.False) || (kind == "nullValue" && value.Type != gjson.Null) {
		return errors.New("invalid Gemini scalar argument")
	}
	out.WriteString(value.Raw)
	return nil
}

func (s *partialJSON) startValue(out *strings.Builder, path []argumentPath) error {
	if len(s.stack) == 0 {
		out.WriteByte('{')
		s.stack = append(s.stack, argumentContainer{})
	}
	common := 0
	for common < len(path)-1 && common+1 < len(s.stack) && s.stack[common+1].at == path[common] {
		common++
	}
	for len(s.stack) > common+1 {
		s.closeContainer(out)
	}
	for depth := common; depth < len(path); depth++ {
		container := &s.stack[len(s.stack)-1]
		part := path[depth]
		if container.array != part.array || (part.array && part.index != container.count) || (!part.array && container.keys[part.key]) {
			return errors.New("non-appendable Gemini argument path")
		}
		if container.count > 0 {
			out.WriteByte(',')
		}
		container.count++
		if !part.array {
			if container.keys == nil {
				container.keys = make(map[string]bool)
			}
			container.keys[part.key] = true
			key, _ := json.Marshal(part.key)
			out.Write(key)
			out.WriteByte(':')
		}
		if depth+1 < len(path) {
			array := path[depth+1].array
			if array {
				out.WriteByte('[')
			} else {
				out.WriteByte('{')
			}
			s.stack = append(s.stack, argumentContainer{at: part, array: array})
		}
	}
	return nil
}

func (s *partialJSON) closeContainer(out *strings.Builder) {
	if s.stack[len(s.stack)-1].array {
		out.WriteByte(']')
	} else {
		out.WriteByte('}')
	}
	s.stack = s.stack[:len(s.stack)-1]
}

func (s *partialJSON) close(out *strings.Builder) error {
	if s.stringPath != nil {
		return errors.New("unfinished Gemini string argument")
	}
	if len(s.stack) == 0 {
		out.WriteString("{}")
	}
	for len(s.stack) > 0 {
		s.closeContainer(out)
	}
	return nil
}

func parseArgumentPath(path string) ([]argumentPath, error) {
	invalid := errors.New("invalid Gemini argument JSON path")
	if !strings.HasPrefix(path, "$") {
		return nil, invalid
	}
	var parts []argumentPath
	for pos := 1; pos < len(path); {
		var part argumentPath
		switch path[pos] {
		case '.':
			pos++
			start := pos
			for pos < len(path) && path[pos] != '.' && path[pos] != '[' {
				pos++
			}
			part.key = path[start:pos]
			if part.key == "" || strings.ContainsAny(part.key, "*] \t\r\n") {
				return nil, invalid
			}
		case '[':
			pos++
			if pos == len(path) {
				return nil, invalid
			}
			if quote := path[pos]; quote == '\'' || quote == '"' {
				pos++
				var quoted strings.Builder
				quoted.WriteByte('"')
				for pos < len(path) && path[pos] != quote {
					c := path[pos]
					pos++
					if c == '\\' {
						if pos == len(path) {
							return nil, invalid
						}
						c = path[pos]
						pos++
						if c != '\'' || quote != '\'' {
							quoted.WriteByte('\\')
						}
					} else if c == '"' {
						quoted.WriteByte('\\')
					}
					quoted.WriteByte(c)
				}
				if pos == len(path) {
					return nil, invalid
				}
				pos++
				quoted.WriteByte('"')
				if json.Unmarshal([]byte(quoted.String()), &part.key) != nil {
					return nil, invalid
				}
			} else {
				start := pos
				for pos < len(path) && path[pos] >= '0' && path[pos] <= '9' {
					pos++
				}
				index, err := strconv.Atoi(path[start:pos])
				if err != nil {
					return nil, invalid
				}
				part.array, part.index = true, index
			}
			if pos == len(path) || path[pos] != ']' {
				return nil, invalid
			}
			pos++
		default:
			return nil, invalid
		}
		parts = append(parts, part)
	}
	if len(parts) == 0 || parts[0].array {
		return nil, invalid
	}
	return parts, nil
}
