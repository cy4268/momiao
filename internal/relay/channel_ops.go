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
	"net/url"
	"slices"
	"strings"
	"time"

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
	OK       bool   `json:"ok"`
	Model    string `json:"model"`
	Status   int    `json:"status_code"`
	Duration int64  `json:"duration_ms"`
	Message  string `json:"message"`
}

func TestChannel(ctx context.Context, c platform.RelayChannel, model string) ChannelTestResult {
	start := time.Now()
	result := ChannelTestResult{Model: model}
	finish := func(message string) ChannelTestResult {
		result.Message = message
		result.Duration = time.Since(start).Milliseconds()
		return result
	}
	if model == "" {
		return finish("请先配置至少一个模型")
	}
	body, _ := json.Marshal(map[string]any{"model": model, "messages": []map[string]string{{"role": "user", "content": "hi"}}, "max_tokens": 1, "stream": false})
	incoming, _ := http.NewRequestWithContext(ctx, "POST", "http://localhost/v1/chat/completions", nil)
	req, format, _, err := upstreamRequest(ctx, incoming, body, types.RelayFormatOpenAI, model, false, c)
	if err != nil {
		return finish("渠道配置或协议转换失败")
	}
	// The test budget wins over saved parameter overrides. No retry or billing.
	body, err = io.ReadAll(req.Body)
	req.Body.Close()
	if err != nil {
		return finish("测试请求生成失败")
	}
	field := "max_tokens"
	if format == types.RelayFormatGemini {
		field = "generationConfig.maxOutputTokens"
	} else {
		body, _ = sjson.SetBytes(body, "stream", false)
		if format == types.RelayFormatOpenAI && gjson.GetBytes(body, "max_completion_tokens").Exists() {
			field = "max_completion_tokens"
			body, _ = sjson.DeleteBytes(body, "max_tokens")
		}
	}
	body, err = sjson.SetBytes(body, field, 1)
	if err != nil {
		return finish("测试请求生成失败")
	}
	req.Body = io.NopCloser(strings.NewReader(string(body)))
	req.ContentLength = int64(len(body))
	req.GetBody = nil
	res, err := channelClient().Do(req)
	if err != nil {
		return finish("上游连接失败")
	}
	defer res.Body.Close()
	result.Status = res.StatusCode
	body, err = io.ReadAll(res.Body)
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return finish(fmt.Sprintf("上游 HTTP %d", res.StatusCode))
	}
	problem := gjson.GetBytes(body, "error")
	if err != nil || !json.Valid(body) || problem.Exists() && problem.Type != gjson.Null {
		return finish("上游响应格式或错误字段异常")
	}
	result.OK = true
	return finish("测试成功")
}
