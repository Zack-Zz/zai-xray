package store

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/zhouze/zai-xray/internal/trace"
)

func BenchmarkSQLiteStore_ListAndStats_10k(b *testing.B) {
	dbPath := filepath.Join(b.TempDir(), "perf.db")
	s, err := NewSQLite(dbPath)
	if err != nil {
		b.Fatalf("NewSQLite failed: %v", err)
	}
	defer s.Close()
	ctx := context.Background()
	if err := s.Migrate(ctx); err != nil {
		b.Fatalf("Migrate failed: %v", err)
	}
	seedTraces(b, s, 10_000)

	b.Run("trace_list_limit_50", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if _, err := s.ListTraces(ctx, trace.ListFilter{Limit: 50}); err != nil {
				b.Fatalf("ListTraces failed: %v", err)
			}
		}
	})

	b.Run("stats_today", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if _, err := s.Stats(ctx, trace.PeriodToday, "", time.Now()); err != nil {
				b.Fatalf("Stats failed: %v", err)
			}
		}
	})
}

func TestSQLiteStore_Baseline_10k(t *testing.T) {
	if testing.Short() {
		t.Skip("skip baseline in short mode")
	}
	dbPath := filepath.Join(t.TempDir(), "baseline.db")
	s, err := NewSQLite(dbPath)
	if err != nil {
		t.Fatalf("NewSQLite failed: %v", err)
	}
	defer s.Close()
	ctx := context.Background()
	if err := s.Migrate(ctx); err != nil {
		t.Fatalf("Migrate failed: %v", err)
	}
	seedTraces(t, s, 10_000)

	startList := time.Now()
	if _, err := s.ListTraces(ctx, trace.ListFilter{Limit: 50}); err != nil {
		t.Fatalf("ListTraces failed: %v", err)
	}
	listDur := time.Since(startList)

	startStats := time.Now()
	if _, err := s.Stats(ctx, trace.PeriodToday, "", time.Now()); err != nil {
		t.Fatalf("Stats failed: %v", err)
	}
	statsDur := time.Since(startStats)

	// Keep this as a warning threshold to avoid flaky CI across machines.
	if listDur > 400*time.Millisecond {
		t.Logf("warning: trace list baseline slower than target: %s", listDur)
	}
	if statsDur > 600*time.Millisecond {
		t.Logf("warning: stats baseline slower than target: %s", statsDur)
	}
}

type seedTB interface {
	Fatalf(string, ...any)
	Helper()
}

func seedTraces(tb seedTB, s *SQLiteStore, total int) {
	tb.Helper()
	now := time.Now().UnixMilli()
	for i := 0; i < total; i++ {
		traceID := fmt.Sprintf("seed-%06d", i)
		pt, ct, tt := 120+i%30, 80+i%20, 200+i%40
		cost := 0.0004 + float64(i%9)/10000
		tItem := trace.Trace{
			TraceID:          traceID,
			Source:           "run",
			Command:          "zai run seed",
			Provider:         "openai",
			Model:            "gpt-4o-mini",
			Status:           "ok",
			StartTimeMS:      now - int64(100+i%30),
			EndTimeMS:        now - int64(i%20),
			LatencyMS:        int64(100 + i%40),
			PromptTokens:     &pt,
			CompletionTokens: &ct,
			TotalTokens:      &tt,
			CostUSD:          &cost,
			CreatedAtMS:      now - int64(i*5),
		}
		if err := s.CreateTrace(context.Background(), tItem, nil); err != nil {
			tb.Fatalf("seed trace failed at %d: %v", i, err)
		}
	}
}
