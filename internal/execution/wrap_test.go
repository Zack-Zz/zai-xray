package execution

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/zhouze/zai-xray/internal/store"
	"github.com/zhouze/zai-xray/internal/trace"
)

func TestWrapper_NoProxy(t *testing.T) {
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

	w := NewWrapper(s)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	res, err := w.Wrap(ctx, WrapOptions{
		Target:  "sh",
		Args:    []string{"-c", "echo wrapped"},
		NoProxy: true,
		Command: "zai wrap sh -- -c echo wrapped",
	})
	if err != nil {
		t.Fatalf("Wrap failed: %v", err)
	}
	if res.Trace.TraceID == "" || res.Trace.Status != "ok" {
		t.Fatalf("unexpected wrap result: %+v", res)
	}

	item, err := s.GetTrace(context.Background(), res.Trace.TraceID)
	if err != nil {
		t.Fatalf("GetTrace failed: %v", err)
	}
	if item.Trace.Source != "wrap:sh" {
		t.Fatalf("unexpected source: %s", item.Trace.Source)
	}
}

func TestTracePeriodRange(t *testing.T) {
	t.Parallel()
	_, _, err := trace.PeriodRange("today", time.Now())
	if err != nil {
		t.Fatalf("PeriodRange failed: %v", err)
	}
}
