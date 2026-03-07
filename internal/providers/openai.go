package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type OpenAIProvider struct {
	apiKey  string
	baseURL string
	client  *http.Client
}

func NewOpenAIProvider(apiKey, baseURL string, timeout time.Duration) *OpenAIProvider {
	if strings.TrimSpace(baseURL) == "" {
		baseURL = "https://api.openai.com"
	}
	return &OpenAIProvider{
		apiKey:  strings.TrimSpace(apiKey),
		baseURL: strings.TrimRight(baseURL, "/"),
		client: &http.Client{
			Timeout: timeout,
		},
	}
}

func (p *OpenAIProvider) Name() string {
	return "openai"
}

func (p *OpenAIProvider) EstimateCost(model string, promptTokens, completionTokens int) (float64, bool) {
	return estimateCostWithTable(openAIPrices, model, promptTokens, completionTokens)
}

func (p *OpenAIProvider) Call(ctx context.Context, req CallRequest) (CallResponse, error) {
	if p.apiKey == "" {
		return CallResponse{}, fmt.Errorf("OPENAI_API_KEY is required")
	}
	model := strings.TrimSpace(req.Model)
	if model == "" {
		model = "gpt-4o-mini"
	}

	bodyObj := map[string]any{
		"model": model,
		"messages": []map[string]string{{
			"role":    "user",
			"content": req.Prompt,
		}},
	}
	payload, err := json.Marshal(bodyObj)
	if err != nil {
		return CallResponse{}, fmt.Errorf("marshal openai request: %w", err)
	}

	var lastErr error
	var retryCount int
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			retryCount++
			select {
			case <-ctx.Done():
				return CallResponse{}, ctx.Err()
			case <-time.After(time.Duration(attempt*200) * time.Millisecond):
			}
		}

		resp, err := p.doRequest(ctx, payload)
		if err == nil {
			resp.RetryCount = retryCount
			return resp, nil
		}
		lastErr = err
		var apiErr *APIError
		if !isRetryable(err, &apiErr) {
			break
		}
	}
	return CallResponse{}, lastErr
}

func (p *OpenAIProvider) doRequest(ctx context.Context, payload []byte) (CallResponse, error) {
	url := p.baseURL + "/v1/chat/completions"
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return CallResponse{}, fmt.Errorf("build openai request: %w", err)
	}
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)
	httpReq.Header.Set("Content-Type", "application/json")

	httpResp, err := p.client.Do(httpReq)
	if err != nil {
		return CallResponse{}, fmt.Errorf("send openai request: %w", err)
	}
	defer httpResp.Body.Close()

	respBody, err := io.ReadAll(httpResp.Body)
	if err != nil {
		return CallResponse{}, fmt.Errorf("read openai response: %w", err)
	}
	if httpResp.StatusCode >= 300 {
		return CallResponse{}, &APIError{StatusCode: httpResp.StatusCode, Body: string(respBody)}
	}

	var parsed struct {
		Model   string `json:"model"`
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
			TotalTokens      int `json:"total_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return CallResponse{}, fmt.Errorf("parse openai response: %w", err)
	}

	result := CallResponse{
		Provider:      p.Name(),
		Model:         parsed.Model,
		HTTPStatus:    httpResp.StatusCode,
		Content:       "",
		RequestBytes:  int64(len(payload)),
		ResponseBytes: int64(len(respBody)),
	}
	if len(parsed.Choices) > 0 {
		result.Content = parsed.Choices[0].Message.Content
	}
	if parsed.Usage.TotalTokens > 0 || parsed.Usage.PromptTokens > 0 || parsed.Usage.CompletionTokens > 0 {
		pt := parsed.Usage.PromptTokens
		ct := parsed.Usage.CompletionTokens
		tt := parsed.Usage.TotalTokens
		result.PromptTokens = &pt
		result.CompletionTokens = &ct
		result.TotalTokens = &tt
	}
	return result, nil
}

func isRetryable(err error, target **APIError) bool {
	if !errorAs(err, target) {
		return false
	}
	return (*target).StatusCode == http.StatusTooManyRequests || (*target).StatusCode >= 500
}

// errorAs is a minimal local replacement to keep call sites simple.
func errorAs(err error, target **APIError) bool {
	if err == nil {
		return false
	}
	apiErr, ok := err.(*APIError)
	if ok {
		*target = apiErr
		return true
	}
	type unwrapper interface{ Unwrap() error }
	u, ok := err.(unwrapper)
	if !ok {
		return false
	}
	return errorAs(u.Unwrap(), target)
}
