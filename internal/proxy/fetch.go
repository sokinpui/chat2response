package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/sokinpui/chat2response/internal/translate/anthropic"
	"github.com/sokinpui/chat2response/internal/translate/openai"
	"github.com/sokinpui/chat2response/internal/types"
	"github.com/sokinpui/chat2response/internal/utils"
)

var droppedHeaders = map[string]bool{
	"host":            true,
	"content-length":  true,
	"connection":      true,
	"accept-encoding": true,
	"accept":          true,
	"user-agent":      true,
}

type ProxyOptions struct {
	UpstreamFormat  types.UpstreamFormat
	BaseURL         string
	APIVersion      string
	Model           string
	DefaultHeaders  map[string]string
	HTTPClient      *http.Client
	DropImages      bool
	OnCacheStats    func(stats types.CacheStats)
	ReasoningEffort string
	Thinking        any
	TimeoutMs       int
	Fallback        *ProxyOptions
}

func HandleResponses(ctx context.Context, req types.ResponsesRequest, opts ProxyOptions) (*http.Response, error) {
	if opts.DropImages && opts.Fallback != nil && lastUserMessageHasImage(req) {
		fb := opts.Fallback
		fb.Fallback = nil
		if fb.Model != "" {
			req.Model = fb.Model
		}
		return HandleResponses(ctx, req, *fb)
	}

	format := resolveFormat(opts)
	streaming := false
	if req.Stream != nil {
		streaming = *req.Stream
	}

	upstreamBody, metadata := buildUpstreamBody(req, format, streaming, opts)
	resolvedUrl := normalizeBaseUrl(opts.BaseURL, format)

	bodyBytes, err := json.Marshal(upstreamBody)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", resolvedUrl, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, err
	}

	applyHeaders(httpReq, format, opts)

	client := opts.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}

	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return resp, nil
	}

	if !streaming {
		return handleStandardResponse(resp, req.Model, format, opts)
	}

	return handleStreamResponse(resp, req.Model, format, metadata, opts)
}

func resolveFormat(opts ProxyOptions) types.UpstreamFormat {
	if opts.UpstreamFormat != "" {
		return opts.UpstreamFormat
	}
	if f := inferFormatFromUrl(opts.BaseURL); f != "" {
		return f
	}
	if f := inferFormatFromModel(opts.Model); f != "" {
		return f
	}
	return types.UpstreamFormatOpenAIChat
}

func inferFormatFromUrl(baseUrl string) types.UpstreamFormat {
	u, err := url.Parse(baseUrl)
	if err != nil {
		return ""
	}
	path := strings.TrimSuffix(u.Path, "/")
	if strings.HasSuffix(path, "/messages") || strings.Contains(strings.ToLower(u.Host), "anthropic") {
		return types.UpstreamFormatAnthropic
	}
	if strings.HasSuffix(path, "/chat/completions") {
		return types.UpstreamFormatOpenAIChat
	}
	return ""
}

func inferFormatFromModel(model string) types.UpstreamFormat {
	if strings.HasPrefix(strings.ToLower(model), "claude") {
		return types.UpstreamFormatAnthropic
	}
	return ""
}

func normalizeBaseUrl(baseUrl string, format types.UpstreamFormat) string {
	u, err := url.Parse(baseUrl)
	if err != nil {
		return baseUrl
	}
	path := strings.TrimSuffix(u.Path, "/")

	if format == types.UpstreamFormatAnthropic {
		if strings.HasSuffix(path, "/v1/messages") || strings.HasSuffix(path, "/messages") {
			return u.String()
		}
		if strings.HasSuffix(path, "/v1") {
			u.Path = path + "/messages"
			return u.String()
		}
		u.Path = "/v1/messages"
		return u.String()
	}

	if strings.HasSuffix(path, "/v1/chat/completions") || strings.HasSuffix(path, "/chat/completions") {
		return u.String()
	}
	if strings.HasSuffix(path, "/v1") {
		u.Path = path + "/chat/completions"
		return u.String()
	}
	u.Path = "/v1/chat/completions"
	return u.String()
}

func applyHeaders(req *http.Request, format types.UpstreamFormat, opts ProxyOptions) {
	for k, v := range opts.DefaultHeaders {
		req.Header.Set(k, v)
	}

	req.Header.Set("Content-Type", "application/json")

	if format == types.UpstreamFormatAnthropic {
		if req.Header.Get("anthropic-version") == "" {
			version := opts.APIVersion
			if version == "" {
				version = "2023-06-01"
			}
			req.Header.Set("anthropic-version", version)
		}

		auth := req.Header.Get("Authorization")
		if strings.HasPrefix(strings.ToLower(auth), "bearer ") {
			req.Header.Set("x-api-key", strings.TrimSpace(auth[7:]))
			req.Header.Del("Authorization")
		}
	}
}

func buildUpstreamBody(req types.ResponsesRequest, format types.UpstreamFormat, streaming bool, opts ProxyOptions) (any, anthropic.ResponsesStreamMetadata) {
	if format == types.UpstreamFormatAnthropic {
		res := anthropic.TranslateRequest(req, anthropic.TranslateRequestOptions{
			ReasoningBudgets: nil,
		})
		res.Request.Stream = &streaming

		if opts.Thinking != nil {
			if cfg, ok := opts.Thinking.(*types.AnthropicThinkingConfig); ok {
				res.Request.Thinking = cfg
			}
		}

		metadata := anthropic.ResponsesStreamMetadata{
			Temperature: req.Temperature,
			TopP:        req.TopP,
			Tools:       res.Request.Tools,
			ToolChoice:  res.Request.ToolChoice,
			Store:       req.Store,
			Metadata:    req.Metadata,
		}

		return res.Request, metadata
	}

	res := openai.TranslateRequest(req, openai.TranslateRequestOptions{
		DropImages: opts.DropImages,
	})
	res.Request.Stream = &streaming

	if streaming {
		if res.Request.Extra == nil {
			res.Request.Extra = make(map[string]any)
		}
		res.Request.Extra["stream_options"] = map[string]any{"include_usage": true}
	}

	if opts.ReasoningEffort != "" {
		if res.Request.Extra == nil {
			res.Request.Extra = make(map[string]any)
		}
		res.Request.Extra["reasoning_effort"] = opts.ReasoningEffort
	}

	metadata := anthropic.ResponsesStreamMetadata{
		Temperature: req.Temperature,
		TopP:        req.TopP,
		Tools:       res.Request.Tools,
		ToolChoice:  res.Request.ToolChoice,
		Store:       req.Store,
		Metadata:    req.Metadata,
	}

	return res.Request, metadata
}

func handleStandardResponse(resp *http.Response, model string, format types.UpstreamFormat, opts ProxyOptions) (*http.Response, error) {
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var translated types.ResponsesResponse
	if format == types.UpstreamFormatAnthropic {
		var ar types.AnthropicResponse
		if err := json.Unmarshal(body, &ar); err != nil {
			return nil, err
		}
		translated = anthropic.TranslateResponse(ar, anthropic.TranslateResponseOptions{Model: &model})
	} else {
		var or types.OpenAiChatResponse
		if err := json.Unmarshal(body, &or); err != nil {
			return nil, err
		}
		translated = openai.TranslateResponse(or, openai.TranslateResponseOptions{Model: &model})
	}

	if opts.OnCacheStats != nil && translated.Usage != nil {
		opts.OnCacheStats(extractCacheStats(translated.Usage))
	}

	resBody, _ := json.Marshal(translated)
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     make(http.Header),
		Body:       io.NopCloser(bytes.NewReader(resBody)),
	}, nil
}

func handleStreamResponse(resp *http.Response, model string, format types.UpstreamFormat, metadata anthropic.ResponsesStreamMetadata, opts ProxyOptions) (*http.Response, error) {
	sseChan := utils.ParseSseStream(resp.Body)
	var events <-chan types.ResponsesStreamEvent

	if format == types.UpstreamFormatAnthropic {
		events = anthropic.TranslateStream(sseChan, anthropic.TranslateStreamOptions{
			Model:           &model,
			RequestMetadata: &metadata,
		})
	} else {
		events = openai.TranslateStream(sseChan, openai.TranslateStreamOptions{
			Model:           &model,
			RequestMetadata: (*openai.ResponsesStreamMetadata)(&metadata),
		})
	}

	pr, pw := io.Pipe()
	go func() {
		defer pw.Close()
		defer resp.Body.Close()

		var lastStats *types.CacheStats

		for ev := range events {
			if ev.Type == "response.completed" {
				if respRaw, ok := ev.Extra["response"].(types.ResponsesResponse); ok {
					if respRaw.Usage != nil {
						stats := extractCacheStats(respRaw.Usage)
						lastStats = &stats
					}
				}
			}
			data := utils.EncodeSseEvent(ev.Type, ev)
			pw.Write([]byte(data))
		}

		pw.Write([]byte("data: [DONE]\n\n"))

		if lastStats != nil && opts.OnCacheStats != nil {
			opts.OnCacheStats(*lastStats)
		}
	}()

	headers := make(http.Header)
	headers.Set("Content-Type", "text/event-stream; charset=utf-8")
	headers.Set("Cache-Control", "no-cache")
	headers.Set("Connection", "keep-alive")

	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     headers,
		Body:       pr,
	}, nil
}

func extractCacheStats(usage *types.ResponsesUsage) types.CacheStats {
	stats := types.CacheStats{
		InputTokens:  usage.InputTokens,
		OutputTokens: usage.OutputTokens,
		TotalTokens:  usage.TotalTokens,
	}
	if usage.InputTokensDetails != nil {
		if usage.InputTokensDetails.CachedTokens != nil {
			stats.CachedTokens = *usage.InputTokensDetails.CachedTokens
		}
		if usage.InputTokensDetails.CacheCreationTokens != nil {
			stats.CacheCreationTokens = *usage.InputTokensDetails.CacheCreationTokens
		}
	}
	return stats
}

func lastUserMessageHasImage(req types.ResponsesRequest) bool {
	input, ok := req.Input.([]any)
	if !ok {
		return false
	}

	for i := len(input) - 1; i >= 0; i-- {
		item, ok := input[i].(map[string]any)
		if !ok || item["role"] != "user" {
			continue
		}

		content, ok := item["content"].([]any)
		if !ok {
			return false
		}

		for _, part := range content {
			p, ok := part.(map[string]any)
			if !ok {
				continue
			}
			t := p["type"]
			if t == "input_image" || t == "image" || t == "image_url" {
				return true
			}
		}
		return false
	}
	return false
}

func IsResponsesEndpoint(r *http.Request) bool {
	return strings.HasSuffix(strings.TrimSuffix(r.URL.Path, "/"), "/v1/responses")
}

func FilterHeaders(h http.Header) http.Header {
	out := make(http.Header)
	for k, v := range h {
		kLower := strings.ToLower(k)
		if droppedHeaders[kLower] || isClientSpecificHeader(kLower) {
			continue
		}
		out[k] = v
	}
	return out
}

func isClientSpecificHeader(key string) bool {
	return strings.HasPrefix(key, "openai-") ||
		strings.HasPrefix(key, "x-stainless") ||
		strings.HasPrefix(key, "x-codex-") ||
		key == "originator" ||
		key == "session_id" ||
		key == "x-client-request-id"
}
