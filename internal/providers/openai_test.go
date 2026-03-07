package providers

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestOpenAIProvider_Call(t *testing.T) {
	t.Parallel()

	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{
		  "model":"gpt-4o-mini",
		  "choices":[{"message":{"content":"hello"}}],
		  "usage":{"prompt_tokens":12,"completion_tokens":6,"total_tokens":18}
		}`))
	})

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		if strings.Contains(err.Error(), "operation not permitted") {
			t.Skipf("sandbox does not allow local listener in this run: %v", err)
		}
		t.Fatalf("listen failed: %v", err)
	}
	defer ln.Close()

	srv := httptest.NewUnstartedServer(h)
	srv.Listener = ln
	srv.Start()
	defer srv.Close()

	p := NewOpenAIProvider("test-key", srv.URL, 2*time.Second)
	res, err := p.Call(context.Background(), CallRequest{Model: "gpt-4o-mini", Prompt: "hi"})
	if err != nil {
		t.Fatalf("Call failed: %v", err)
	}
	if res.Content != "hello" {
		t.Fatalf("unexpected content: %q", res.Content)
	}
	if res.TotalTokens == nil || *res.TotalTokens != 18 {
		t.Fatalf("unexpected usage: %+v", res)
	}
}

func TestEstimateCost(t *testing.T) {
	t.Parallel()
	p := NewOpenAIProvider("k", "", time.Second)
	cost, ok := p.EstimateCost("gpt-4o-mini", 1000, 500)
	if !ok || cost <= 0 {
		t.Fatalf("EstimateCost failed: ok=%v cost=%f", ok, cost)
	}
}
