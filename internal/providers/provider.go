package providers

import "context"

type CallRequest struct {
	Model  string
	Prompt string
}

type CallResponse struct {
	Provider         string
	Model            string
	Content          string
	PromptTokens     *int
	CompletionTokens *int
	TotalTokens      *int
	EstimatedTokens  bool
	HTTPStatus       int
	RequestBytes     int64
	ResponseBytes    int64
	RetryCount       int
}

type Provider interface {
	Name() string
	Call(ctx context.Context, req CallRequest) (CallResponse, error)
	EstimateCost(model string, promptTokens, completionTokens int) (float64, bool)
}
