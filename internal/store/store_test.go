package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/zhouze/zai-xray/internal/trace"
)

func TestSQLiteStore_CRUDAndStats(t *testing.T) {
	t.Parallel()

	dbPath := filepath.Join(t.TempDir(), "traces.db")
	s, err := NewSQLite(dbPath)
	if err != nil {
		t.Fatalf("NewSQLite failed: %v", err)
	}
	defer s.Close()

	ctx := context.Background()
	if err := s.Migrate(ctx); err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}

	now := time.Now().UnixMilli()
	pt, ct, tt := 10, 5, 15
	cost := 0.001
	reqBytes, respBytes := int64(128), int64(512)

	tc := trace.Trace{
		TraceID:          "abc123",
		Source:           "run",
		Command:          "zai run hi",
		Provider:         "openai",
		Model:            "gpt-4o-mini",
		Status:           "ok",
		StartTimeMS:      now - 100,
		EndTimeMS:        now,
		LatencyMS:        100,
		PromptTokens:     &pt,
		CompletionTokens: &ct,
		TotalTokens:      &tt,
		CostUSD:          &cost,
		RequestBytes:     &reqBytes,
		ResponseBytes:    &respBytes,
		CreatedAtMS:      now,
	}

	if err := s.CreateTrace(ctx, tc, trace.BuildArtifact(tc.TraceID, "prompt", "response", 50)); err != nil {
		t.Fatalf("CreateTrace failed: %v", err)
	}

	got, err := s.GetTrace(ctx, tc.TraceID)
	if err != nil {
		t.Fatalf("GetTrace failed: %v", err)
	}
	if got.Trace.TraceID != tc.TraceID {
		t.Fatalf("trace_id mismatch: got=%s want=%s", got.Trace.TraceID, tc.TraceID)
	}
	if got.Artifact == nil || got.Artifact.PromptSHA256 == "" {
		t.Fatalf("artifact not stored")
	}

	list, err := s.ListTraces(ctx, trace.ListFilter{Limit: 10})
	if err != nil {
		t.Fatalf("ListTraces failed: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("unexpected list size: %d", len(list))
	}

	stats, err := s.Stats(ctx, trace.PeriodToday, "provider", time.Now())
	if err != nil {
		t.Fatalf("Stats failed: %v", err)
	}
	if stats.TotalCalls != 1 || stats.OKCalls != 1 {
		t.Fatalf("unexpected stats summary: %+v", stats)
	}
	if len(stats.Groups) != 1 || stats.Groups[0].Key != "openai" {
		t.Fatalf("unexpected grouped stats: %+v", stats.Groups)
	}
}
