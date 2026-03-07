package execution

import (
	"context"
	"fmt"
	"time"

	"github.com/zhouze/zai-xray/internal/providers"
	"github.com/zhouze/zai-xray/internal/store"
	"github.com/zhouze/zai-xray/internal/trace"
)

type Runner struct {
	store     store.Store
	providers map[string]providers.Provider
}

func NewRunner(s store.Store, providerList ...providers.Provider) *Runner {
	m := make(map[string]providers.Provider, len(providerList))
	for _, p := range providerList {
		m[p.Name()] = p
	}
	return &Runner{store: s, providers: m}
}

func (r *Runner) Run(ctx context.Context, opts RunOptions) (RunResult, error) {
	p, ok := r.providers[opts.Provider]
	if !ok {
		return RunResult{}, fmt.Errorf("unsupported provider: %s", opts.Provider)
	}
	start := time.Now().UTC()
	callResp, err := p.Call(ctx, providers.CallRequest{Model: opts.Model, Prompt: opts.Prompt})
	end := time.Now().UTC()

	t := trace.Trace{
		TraceID:     trace.NewTraceID(),
		Source:      "run",
		Command:     opts.Command,
		Provider:    opts.Provider,
		Model:       opts.Model,
		StartTimeMS: start.UnixMilli(),
		EndTimeMS:   end.UnixMilli(),
		LatencyMS:   end.Sub(start).Milliseconds(),
		CreatedAtMS: end.UnixMilli(),
	}

	if err != nil {
		t.Status = "error"
		t.ErrorType = classifyError(err)
		t.ErrorMessage = err.Error()
		// Persist failed attempts as first-class traces for debugging and stats accuracy.
		if createErr := r.store.CreateTrace(ctx, t, trace.BuildArtifact(t.TraceID, opts.Prompt, "", 320)); createErr != nil {
			return RunResult{}, fmt.Errorf("store trace after error: %w", createErr)
		}
		return RunResult{Trace: t}, err
	}

	if callResp.Model != "" {
		t.Model = callResp.Model
	}
	t.Status = "ok"
	httpStatus := callResp.HTTPStatus
	t.HTTPStatus = &httpStatus
	t.PromptTokens = callResp.PromptTokens
	t.CompletionTokens = callResp.CompletionTokens
	t.TotalTokens = callResp.TotalTokens
	t.RequestBytes = &callResp.RequestBytes
	t.ResponseBytes = &callResp.ResponseBytes
	t.RetryCount = callResp.RetryCount

	if t.PromptTokens == nil || t.CompletionTokens == nil || t.TotalTokens == nil {
		pt, ct, tt := estimateTokens(opts.Prompt, callResp.Content)
		t.PromptTokens = &pt
		t.CompletionTokens = &ct
		t.TotalTokens = &tt
		t.TokensEstimated = true
	}

	if callResp.PromptTokens != nil && callResp.CompletionTokens != nil {
		// keep branch for readability when provider usage is present
	}
	if t.PromptTokens != nil && t.CompletionTokens != nil {
		cost, ok := p.EstimateCost(t.Model, *t.PromptTokens, *t.CompletionTokens)
		if ok {
			t.CostUSD = &cost
			t.CostEstimated = t.TokensEstimated
		}
	}

	artifact := trace.BuildArtifact(t.TraceID, opts.Prompt, callResp.Content, 320)
	if err := r.store.CreateTrace(ctx, t, artifact); err != nil {
		return RunResult{}, fmt.Errorf("store trace: %w", err)
	}
	return RunResult{Trace: t, Artifact: artifact, Output: callResp.Content}, nil
}
