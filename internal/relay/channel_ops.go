// Model discovery conventions follow official new-api@f1164142 controller/
// channel_upstream_update.go and relay/channel/gemini/relay-gemini.go.
package relay

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/QuantumNous/new-api/relaykit/dto"
	"github.com/QuantumNous/new-api/relaykit/types"
	"github.com/cy4268/momiao/internal/platform"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

func channelBaseURL(c platform.RelayChannel) string {
	if c.BaseURL != "" {
		return strings.TrimRight(c.BaseURL, "/")
	}
	return map[int]string{1: "https://api.openai.com", 14: "https://api.anthropic.com", 24: "https://generativelanguage.googleapis.com", 43: "https://api.deepseek.com"}[c.Type]
}
func channelHeaders(headers http.Header, c platform.RelayChannel) error {
	var overrides map[string]string
	if len(c.HeaderOverride) > 0 {
		if err := json.Unmarshal(c.HeaderOverride, &overrides); err != nil {
			return err
		}
	}
	for name, value := range overrides {
		headers.Set(name, strings.ReplaceAll(value, "{api_key}", c.Key))
	}
	return nil
}
func channelClient() *http.Client {
	return &http.Client{Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}

func FetchChannelModels(ctx context.Context, c platform.RelayChannel) ([]string, error) {
	base := channelBaseURL(c)
	if base == "" {
		return nil, errors.New("此渠道需要手动填写模型列表")
	}
	headers := make(http.Header)
	headers.Set("Authorization", "Bearer "+c.Key)
	path := "/v1/models"
	if c.Type == 14 {
		headers.Del("Authorization")
		headers.Set("x-api-key", c.Key)
		headers.Set("anthropic-version", "2023-06-01")
	}
	if c.Type == 24 {
		headers.Del("Authorization")
		headers.Set("x-goog-api-key", c.Key)
		path = "/v1beta/models"
	}
	if err := channelHeaders(headers, c); err != nil {
		return nil, errors.New("渠道请求头配置有误")
	}
	client := channelClient()
	items := []string{}
	next := ""
	for page := 0; page < 100; page++ {
		address := base + path
		if next != "" {
			address += "?pageToken=" + url.QueryEscape(next)
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
		if err != nil {
			return nil, errors.New("渠道 URL 有误")
		}
		req.Header = headers
		res, err := client.Do(req)
		if err != nil {
			return nil, errors.New("上游模型列表连接失败")
		}
		var body struct {
			Data []struct {
				ID string `json:"id"`
			} `json:"data"`
			Models []struct {
				Name string `json:"name"`
			} `json:"models"`
			Next string `json:"nextPageToken"`
		}
		err = json.NewDecoder(res.Body).Decode(&body)
		res.Body.Close()
		if res.StatusCode != 200 {
			return nil, fmt.Errorf("上游模型列表 HTTP %d", res.StatusCode)
		}
		if err != nil {
			return nil, errors.New("上游模型列表格式有误")
		}
		if c.Type == 24 {
			for _, m := range body.Models {
				items = append(items, strings.TrimPrefix(m.Name, "models/"))
			}
			next = body.Next
		} else {
			for _, m := range body.Data {
				items = append(items, m.ID)
			}
			next = ""
		}
		if next == "" {
			slices.Sort(items)
			items = slices.Compact(items)
			items = slices.DeleteFunc(items, func(id string) bool { return id == "" })
			return items, nil
		}
	}
	return nil, errors.New("上游模型列表分页超出 100 页")
}

type ChannelTestResult struct {
	OK            bool      `json:"ok"`
	Model         string    `json:"model"`
	Status        int       `json:"status_code"`
	Duration      int64     `json:"duration_ms"`
	Message       string    `json:"message"`
	UpstreamModel string    `json:"upstream_model"`
	MaxTokens     int64     `json:"max_tokens"`
	TestAt        time.Time `json:"test_at"`
}

type ChannelTestOptions struct {
	Model    string `json:"model"`
	Stream   bool   `json:"stream"`
	Endpoint string `json:"endpoint"`
}

func (v ChannelTestOptions) Valid() bool {
	return slices.Contains([]string{"", "auto", "chat/completions", "responses", "messages"}, v.Endpoint)
}

// Copyright QuantumNous and contributors, AGPL-3.0-or-later.
// Request shapes/budgets follow official f1164142 controller/channel-test.go.
// Reuse the live protocol converter and response reader, not the billing handler.
func TestChannel(ctx context.Context, c platform.RelayChannel, options ChannelTestOptions) ChannelTestResult {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()
	start := time.Now()
	result := ChannelTestResult{Model: options.Model, UpstreamModel: c.UpstreamModel}
	finish := func(message string) ChannelTestResult {
		result.Message = message
		result.Duration = time.Since(start).Milliseconds()
		result.TestAt = time.Now().UTC()
		return result
	}
	if options.Model == "" {
		return finish("请先配置至少一个模型")
	}
	endpoint := options.Endpoint
	if endpoint == "" || endpoint == "auto" {
		endpoint = "chat/completions"
		if c.Type == 14 {
			endpoint = "messages"
		} else if strings.Contains(c.UpstreamModel, "codex") {
			endpoint = "responses"
		}
	}
	format := types.RelayFormatOpenAI
	payload := map[string]any{"model": options.Model, "messages": []map[string]string{{"role": "user", "content": "hi"}}, "max_tokens": 16, "stream": options.Stream}
	if endpoint == "responses" {
		format = types.RelayFormatOpenAIResponses
		payload = map[string]any{"model": options.Model, "input": "hi", "max_output_tokens": 16, "stream": options.Stream, "store": false}
	} else if endpoint == "messages" {
		format = types.RelayFormatClaude
	}
	body, _ := json.Marshal(payload)
	incoming, _ := http.NewRequestWithContext(ctx, "POST", "http://localhost/v1/"+endpoint, nil)
	req, format, meta, err := upstreamRequest(ctx, incoming, body, format, options.Model, options.Stream, c)
	if err != nil {
		return finish("渠道配置或协议转换失败")
	}
	// The test budget wins over saved parameter overrides. No retry or billing.
	body, err = io.ReadAll(req.Body)
	req.Body.Close()
	if err != nil {
		return finish("测试请求生成失败")
	}
	field, budget := "max_tokens", int64(16)
	name := strings.ToLower(c.UpstreamModel)
	if strings.Contains(name, "thinking") && !strings.Contains(name, "claude") {
		budget = 50
	}
	if strings.Contains(name, "gemini") {
		budget = 3000
	}
	switch format {
	case types.RelayFormatGemini:
		field = "generationConfig.maxOutputTokens"
		budget = 3000
	case types.RelayFormatOpenAIResponses:
		field = "max_output_tokens"
	case types.RelayFormatOpenAI:
		if dto.IsOpenAIReasoningOModel(name) || dto.IsOpenAIGPT5Model(name) || gjson.GetBytes(body, "max_completion_tokens").Exists() {
			field = "max_completion_tokens"
		}
	case types.RelayFormatClaude:
		// Extended thinking needs at least 1024 tokens and max_tokens > budget.
		if gjson.GetBytes(body, "thinking.type").String() == "enabled" {
			body, _ = sjson.SetBytes(body, "thinking.budget_tokens", 1024)
			budget = 1025
		}
	}
	// Remove alternate caps from imported overrides; the probe owns its small budget.
	for _, other := range []string{"max_tokens", "max_completion_tokens", "max_output_tokens", "generationConfig.maxOutputTokens"} {
		if other != field {
			body, _ = sjson.DeleteBytes(body, other)
		}
	}
	if format != types.RelayFormatGemini {
		body, _ = sjson.SetBytes(body, "stream", options.Stream)
		body, _ = sjson.SetBytes(body, "model", c.UpstreamModel)
	}
	if format == types.RelayFormatOpenAI {
		if options.Stream {
			body, _ = sjson.SetBytes(body, "stream_options.include_usage", true)
		} else {
			body, _ = sjson.DeleteBytes(body, "stream_options")
		}
	}
	body, err = sjson.SetBytes(body, field, budget)
	if err != nil {
		return finish("测试请求生成失败")
	}
	result.MaxTokens = budget
	req.Body = io.NopCloser(strings.NewReader(string(body)))
	req.ContentLength = int64(len(body))
	req.GetBody = nil
	client := channelClient()
	client.Timeout = requestTimeout
	res, err := client.Do(req)
	if err != nil {
		detail := transportFailure(err, "upstream connection failed")
		result.Status = detail.Status
		return finish(detail.Message)
	}
	defer res.Body.Close()
	result.Status = res.StatusCode
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		detail := readUpstreamError(res, c.Key)
		return finish(fmt.Sprintf("上游 HTTP %d: %s", detail.Status, detail.Message))
	}
	response := forwardResponse(ctx, httptest.NewRecorder(), res, format, format, meta, options.Stream, "channel-test", c.Key)
	if response.failure != "" {
		result.Status = response.detail.Status
		return finish(fmt.Sprintf("上游 HTTP %d: %s", result.Status, response.detail.Message))
	}
	result.OK = true
	return finish("测试成功")
}
