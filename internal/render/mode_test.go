package render

import (
	"bytes"
	"strings"
	"testing"

	"github.com/zhouze/zai-xray/internal/trace"
)

func TestRenderTraceList_NoColor(t *testing.T) {
	t.Parallel()

	buf := &bytes.Buffer{}
	r := New(buf, Options{Mode: ModePretty, UseColor: false})
	tid := "abcdef123456"
	lat := int64(1200)
	cost := 0.002
	pt, ct, tt := 10, 20, 30
	items := []trace.Trace{{
		TraceID:          tid,
		Status:           "ok",
		Provider:         "openai",
		Model:            "gpt-4o-mini",
		LatencyMS:        lat,
		CostUSD:          &cost,
		PromptTokens:     &pt,
		CompletionTokens: &ct,
		TotalTokens:      &tt,
	}}

	if err := r.RenderTraceList(items); err != nil {
		t.Fatalf("RenderTraceList failed: %v", err)
	}
	out := buf.String()
	if strings.Contains(out, "\x1b[") {
		t.Fatalf("expected no ANSI output, got: %q", out)
	}
	if !strings.Contains(out, "TRACE LIST") {
		t.Fatalf("missing title: %q", out)
	}
}
