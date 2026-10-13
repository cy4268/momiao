// Protocol conversion and channel conventions reuse QuantumNous/new-api
// f116414284162ad15d8925f7bca494c109b83e93. RelayKit owns DTO/conversion logic;
// this host supplies HTTP, local channel credentials and imported overrides.
package relay

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/relayconvert"
	"github.com/QuantumNous/new-api/relaykit/relayconvert/convmeta"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/cy4268/momiao/internal/platform"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
	"golang.org/x/oauth2/google"
)

func upstreamRequest(ctx context.Context, incoming *http.Request, body []byte, from types.RelayFormat, model string, stream bool, channel platform.RelayChannel) (*http.Request, types.RelayFormat, *convmeta.Values, error) {
	to := from
	switch channel.Type {
	case 1:
		if from == types.RelayFormatClaude {
			to = types.RelayFormatOpenAI
		}
	case 43:
	case 14:
		to = types.RelayFormatClaude
	case 24, 41:
		to = types.RelayFormatGemini
	default:
		return nil, "", nil, errors.New("unsupported channel type")
	}
	meta := &convmeta.Values{OriginModelName: model, UpstreamModelName: channel.UpstreamModel, ChannelMetaAttached: true,
		ChannelID: int(channel.ID), ChannelType: channel.Type, IsStream: stream,
		Options: &convmeta.Options{
			Claude: convmeta.ClaudeOptions{ThinkingAdapterEnabled: true, ThinkingAdapterBudgetTokensPercentage: 0.8, DefaultMaxTokens: func(string) int { return 8192 }},
			Gemini: convmeta.GeminiOptions{ThinkingAdapterBudgetTokensPercentage: 0.6, FunctionCallThoughtSignatureEnabled: true, SafetySetting: func(string) string { return "OFF" }},
		},
	}
	var err error
	body, err = sjson.SetBytes(body, "model", channel.UpstreamModel)
	if err != nil {
		return nil, to, meta, err
	}
	if from != to {
		var input any
		switch from {
		case types.RelayFormatOpenAI:
			input = &dto.GeneralOpenAIRequest{}
		case types.RelayFormatOpenAIResponses:
			input = &dto.OpenAIResponsesRequest{}
		case types.RelayFormatClaude:
			input = &dto.ClaudeRequest{}
		}
		if err = json.Unmarshal(body, input); err != nil {
			return nil, to, meta, err
		}
		converted, err := relayconvert.ConvertRequest(ctx, meta, to, input)
		if err != nil {
			return nil, to, meta, err
		}
		// Official Gemini/Vertex adapters remove response IDs; thought signatures
		// and tool-call IDs on all other parts stay under RelayKit's control.
		if gemini, ok := converted.Value.(*dto.GeminiChatRequest); ok {
			for i := range gemini.Contents {
				for j := range gemini.Contents[i].Parts {
					if part := gemini.Contents[i].Parts[j].FunctionResponse; part != nil {
						part.ID = nil
					}
				}
			}
		}
		body, err = json.Marshal(converted.Value)
		if err != nil {
			return nil, to, meta, err
		}
	}
	if stream && to == types.RelayFormatOpenAI {
		body, err = sjson.SetBytes(body, "stream_options.include_usage", true)
		if err != nil {
			return nil, to, meta, err
		}
	}
	// Apply only explicit imported settings, not a DTO whitelist. Unknown fields
	// survive unchanged on same-protocol routes.
	for field, setting := range map[string]string{"service_tier": "allow_service_tier", "inference_geo": "allow_inference_geo", "safety_identifier": "allow_safety_identifier", "include_obfuscation": "allow_include_obfuscation"} {
		if value := gjson.GetBytes(channel.Settings, setting); value.Exists() && !value.Bool() {
			body, err = sjson.DeleteBytes(body, field)
			if err != nil {
				return nil, to, meta, err
			}
		}
	}
	if gjson.GetBytes(channel.Settings, "disable_store").Bool() {
		body, err = sjson.SetBytes(body, "store", false)
		if err != nil {
			return nil, to, meta, err
		}
	}
	body, err = overrideParams(body, channel.ParamOverride)
	if err != nil {
		return nil, to, meta, err
	}
	base := strings.TrimRight(channel.BaseURL, "/")
	headers := make(http.Header)
	headers.Set("Content-Type", "application/json")
	headers.Set("Accept", "application/json")
	if stream {
		headers.Set("Accept", "text/event-stream")
	}
	path := incoming.URL.Path
	switch channel.Type {
	case 1:
		if base == "" {
			base = "https://api.openai.com"
		}
		if from == types.RelayFormatClaude {
			path = "/v1/chat/completions"
		}
		headers.Set("Authorization", "Bearer "+channel.Key)
	case 43:
		if base == "" {
			base = "https://api.deepseek.com"
		}
		if from == types.RelayFormatOpenAIResponses {
			path = "/responses"
		}
		if from == types.RelayFormatClaude {
			path = "/anthropic/v1/messages"
		}
		headers.Set("Authorization", "Bearer "+channel.Key)
	case 14:
		if base == "" {
			base = "https://api.anthropic.com"
		}
		path = "/v1/messages"
		headers.Set("X-API-Key", channel.Key)
	case 24, 41:
		action := "generateContent"
		if stream {
			action = "streamGenerateContent?alt=sse"
		}
		if channel.Type == 24 {
			if base == "" {
				base = "https://generativelanguage.googleapis.com"
			}
			version := "v1beta"
			if channel.UpstreamModel == "gemini-1.0-pro" {
				version = "v1"
			}
			path = "/" + version + "/models/" + channel.UpstreamModel + ":" + action
			headers.Set("x-goog-api-key", channel.Key)
		} else {
			var project struct {
				ProjectID string `json:"project_id"`
			}
			region := channel.Other
			if strings.HasPrefix(strings.TrimSpace(region), "{") {
				var regions map[string]string
				if err := json.Unmarshal([]byte(region), &regions); err != nil {
					return nil, to, meta, err
				}
				region = regions[model]
				if region == "" {
					region = regions["default"]
				}
			}
			if gjson.GetBytes(channel.Settings, "vertex_key_type").String() == "api_key" {
				base = BuildGoogleModelURL(base, DefaultAPIVersion, "", region, channel.UpstreamModel, action)
				u, err := url.Parse(base)
				if err != nil {
					return nil, to, meta, err
				}
				query := u.Query()
				query.Set("key", channel.Key)
				u.RawQuery = query.Encode()
				base = u.String()
			} else {
				if err := json.Unmarshal([]byte(channel.Key), &project); err != nil {
					return nil, to, meta, err
				}
				config, err := google.JWTConfigFromJSON([]byte(channel.Key), "https://www.googleapis.com/auth/cloud-platform")
				if err != nil {
					return nil, to, meta, err
				}
				token, err := config.TokenSource(ctx).Token()
				if err != nil {
					return nil, to, meta, err
				}
				headers.Set("Authorization", "Bearer "+token.AccessToken)
				base = BuildGoogleModelURL(base, DefaultAPIVersion, project.ProjectID, region, channel.UpstreamModel, action)
			}
			path = ""
		}
	}
	if to == types.RelayFormatClaude {
		version := incoming.Header.Get("anthropic-version")
		if version == "" {
			version = "2023-06-01"
		}
		headers.Set("anthropic-version", version)
		if beta := incoming.Header.Get("anthropic-beta"); beta != "" {
			headers.Set("anthropic-beta", beta)
		}
		if incoming.URL.Query().Get("beta") == "true" || gjson.GetBytes(channel.Settings, "claude_beta_query").Bool() {
			path += "?beta=true"
		}
	}
	var overrides map[string]string
	if len(channel.HeaderOverride) > 0 {
		if err := json.Unmarshal(channel.HeaderOverride, &overrides); err != nil {
			return nil, to, meta, err
		}
	}
	for name, value := range overrides {
		headers.Set(name, strings.ReplaceAll(value, "{api_key}", channel.Key))
	}
	request, err := http.NewRequestWithContext(ctx, "POST", base+path, bytes.NewReader(body))
	if err != nil {
		return nil, to, meta, err
	}
	request.Header = headers
	return request, to, meta, nil
}

func overrideParams(body []byte, raw json.RawMessage) ([]byte, error) {
	if len(raw) == 0 {
		return body, nil
	}
	var values map[string]json.RawMessage
	if err := json.Unmarshal(raw, &values); err != nil {
		return nil, err
	}
	// The imported operations are deletes. Do not recreate NewAPI's unrelated
	// 2000-line override DSL. Unsupported modes fail visibly instead of vanishing.
	if operations, ok := values["operations"]; ok {
		var items []struct{ Mode, Path string }
		if err := json.Unmarshal(operations, &items); err != nil {
			return nil, err
		}
		for _, item := range items {
			if item.Mode != "delete" || item.Path == "" {
				return nil, errors.New("unsupported parameter override")
			}
			var err error
			body, err = sjson.DeleteBytes(body, item.Path)
			if err != nil {
				return nil, err
			}
		}
		return body, nil
	}
	for key, value := range values {
		var err error
		body, err = sjson.SetRawBytes(body, strings.NewReplacer("\\", "\\\\", ".", "\\.", ":", "\\:").Replace(key), value)
		if err != nil {
			return nil, err
		}
	}
	return body, nil
}
