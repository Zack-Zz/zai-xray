package execution

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/zhouze/zai-xray/internal/providers"
	"github.com/zhouze/zai-xray/internal/store"
)

type fakeProvider struct {
	name string
	resp providers.CallResponse
	err  error
}

func (f fakeProvider) Name() string { return f.name }
func (f fakeProvider) Call(ctx context.Context, req providers.CallRequest) (providers.CallResponse, error) {
	return f.resp, f.err
}
func (f fakeProvider) EstimateCost(model string, promptTokens, completionTokens int) (float64, bool) {
	return 0.123, true
}

func TestRunner_RunSuccess(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "traces.db")
	s, err := store.NewSQLite(dbPath)
	if err != nil {
		t.Fatalf("NewSQLite failed: %v", err)
	}
	defer s.Close()
	if err := s.Migrate(context.Background()); err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}

	pt, ct, tt := 10, 10, 20
	r := NewRunner(s, fakeProvider{
		name: "openai",
		resp: providers.CallResponse{Model: "gpt-4o-mini", Content: "ok", PromptTokens: &pt, CompletionTokens: &ct, TotalTokens: &tt, HTTPStatus: 200},
	})
	res, err := r.Run(context.Background(), RunOptions{Provider: "openai", Model: "gpt-4o-mini", Prompt: "hello", Command: "zai run hello"})
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if res.Trace.Status != "ok" || res.Trace.CostUSD == nil {
		t.Fatalf("unexpected result: %+v", res.Trace)
	}
}

func TestRunner_RunError(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "traces.db")
	s, err := store.NewSQLite(dbPath)
	if err != nil {
		t.Fatalf("NewSQLite failed: %v", err)
	}
	defer s.Close()
	if err := s.Migrate(context.Background()); err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}

	r := NewRunner(s, fakeProvider{name: "openai", err: errors.New("timeout")})
	_, err = r.Run(context.Background(), RunOptions{Provider: "openai", Model: "gpt-4o-mini", Prompt: "hello", Command: "zai run hello"})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestRunner_RunEstimatedUsageFallback(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "traces.db")
	s, err := store.NewSQLite(dbPath)
	if err != nil {
		t.Fatalf("NewSQLite failed: %v", err)
	}
	defer s.Close()
	if err := s.Migrate(context.Background()); err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}

	r := NewRunner(s, fakeProvider{
		name: "openai",
		resp: providers.CallResponse{
			Model:      "gpt-4o-mini",
			Content:    "fallback output",
			HTTPStatus: 200,
		},
	})
	res, err := r.Run(context.Background(), RunOptions{
		Provider: "openai",
		Model:    "gpt-4o-mini",
		Prompt:   "fallback prompt",
		Command:  "zai run fallback",
	})
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if !res.Trace.TokensEstimated {
		t.Fatalf("expected TokensEstimated=true")
	}
	if res.Trace.PromptTokens == nil || res.Trace.CompletionTokens == nil || res.Trace.TotalTokens == nil {
		t.Fatalf("expected estimated token fields, got: %+v", res.Trace)
	}
	if res.Trace.CostUSD == nil || !res.Trace.CostEstimated {
		t.Fatalf("expected estimated cost, got cost=%v costEstimated=%v", res.Trace.CostUSD, res.Trace.CostEstimated)
	}
}
