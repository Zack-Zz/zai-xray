package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type Stats struct {
	Provider         string
	Model            string
	HTTPStatus       int
	PromptTokens     *int
	CompletionTokens *int
	TotalTokens      *int
	RetryCount       int
	RequestBytes     int64
	ResponseBytes    int64
}

type Recorder struct {
	server       *http.Server
	listener     net.Listener
	requestBytes atomic.Int64
	respBytes    atomic.Int64
	statusCode   atomic.Int64
	retries      atomic.Int64

	mu       sync.Mutex
	provider string
	model    string
	pt       *int
	ct       *int
	tt       *int
}

func NewRecorder() *Recorder {
	return &Recorder{}
}

func (r *Recorder) Start(ctx context.Context) (string, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", fmt.Errorf("start proxy listener: %w", err)
	}
	r.listener = ln

	r.server = &http.Server{Handler: http.HandlerFunc(r.handleHTTP)}

	go func() {
		<-ctx.Done()
		_ = r.server.Shutdown(context.Background())
	}()
	go func() {
		_ = r.server.Serve(ln)
	}()

	return ln.Addr().String(), nil
}

func (r *Recorder) handleHTTP(w http.ResponseWriter, req *http.Request) {
	if strings.EqualFold(req.Method, http.MethodConnect) {
		r.handleConnect(w, req)
		return
	}

	targetURL := req.URL.String()
	if !strings.HasPrefix(targetURL, "http") {
		host := req.Host
		if host == "" {
			host = req.URL.Host
		}
		targetURL = "http://" + host + req.URL.Path
		if req.URL.RawQuery != "" {
			targetURL += "?" + req.URL.RawQuery
		}
	}

	bodyBytes, err := io.ReadAll(req.Body)
	if err != nil {
		http.Error(w, "read request body failed", http.StatusBadRequest)
		return
	}
	_ = req.Body.Close()
	r.requestBytes.Add(int64(len(bodyBytes)))

	outReq, err := http.NewRequestWithContext(req.Context(), req.Method, targetURL, bytes.NewReader(bodyBytes))
	if err != nil {
		http.Error(w, "build upstream request failed", http.StatusBadGateway)
		return
	}
	outReq.Header = req.Header.Clone()

	resp, err := http.DefaultTransport.RoundTrip(outReq)
	if err != nil {
		http.Error(w, "proxy upstream failed: "+err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		http.Error(w, "read upstream response failed", http.StatusBadGateway)
		return
	}
	r.respBytes.Add(int64(len(respBytes)))
	r.statusCode.Store(int64(resp.StatusCode))
	for key, vals := range resp.Header {
		for _, val := range vals {
			w.Header().Add(key, val)
		}
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(respBytes)

	// Best-effort metadata extraction for provider/model/usage in wrapped calls.
	r.captureMetadata(targetURL, req, resp.StatusCode, bodyBytes, respBytes)
}

func (r *Recorder) handleConnect(w http.ResponseWriter, req *http.Request) {
	host := req.Host
	if host == "" {
		host = req.URL.Host
	}
	if host == "" {
		http.Error(w, "missing CONNECT host", http.StatusBadRequest)
		return
	}
	if !strings.Contains(host, ":") {
		host += ":443"
	}
	upstreamConn, err := net.DialTimeout("tcp", host, 10*time.Second)
	if err != nil {
		http.Error(w, "CONNECT upstream failed: "+err.Error(), http.StatusBadGateway)
		return
	}

	hj, ok := w.(http.Hijacker)
	if !ok {
		_ = upstreamConn.Close()
		http.Error(w, "hijack not supported", http.StatusInternalServerError)
		return
	}
	clientConn, rw, err := hj.Hijack()
	if err != nil {
		_ = upstreamConn.Close()
		http.Error(w, "hijack failed", http.StatusInternalServerError)
		return
	}

	_, _ = clientConn.Write([]byte("HTTP/1.1 200 Connection Established\r\n\r\n"))
	r.statusCode.Store(http.StatusOK)
	if provider := detectProvider(host); provider != "" {
		r.mu.Lock()
		r.provider = provider
		r.mu.Unlock()
	}

	// Flush buffered bytes (if any) from the client-side reader into upstream.
	if rw != nil && rw.Reader.Buffered() > 0 {
		n, _ := io.CopyN(upstreamConn, rw, int64(rw.Reader.Buffered()))
		r.requestBytes.Add(n)
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		n, _ := io.Copy(upstreamConn, clientConn)
		r.requestBytes.Add(n)
		_ = upstreamConn.SetReadDeadline(time.Now())
	}()
	go func() {
		defer wg.Done()
		n, _ := io.Copy(clientConn, upstreamConn)
		r.respBytes.Add(n)
		_ = clientConn.SetReadDeadline(time.Now())
	}()
	wg.Wait()
	_ = upstreamConn.Close()
	_ = clientConn.Close()
}

func (r *Recorder) captureMetadata(targetURL string, req *http.Request, status int, reqBody, respBody []byte) {
	host := req.URL.Host
	if host == "" {
		host = req.Host
	}
	provider := detectProvider(host)
	if provider != "" {
		r.mu.Lock()
		r.provider = provider
		r.mu.Unlock()
	}
	if status == http.StatusTooManyRequests || status >= 500 {
		r.retries.Add(1)
	}
	if provider != "openai" {
		return
	}

	var rq struct {
		Model string `json:"model"`
	}
	if err := json.Unmarshal(reqBody, &rq); err == nil && rq.Model != "" {
		r.mu.Lock()
		r.model = rq.Model
		r.mu.Unlock()
	}

	var rs struct {
		Model string `json:"model"`
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
			TotalTokens      int `json:"total_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(respBody, &rs); err == nil {
		r.mu.Lock()
		if rs.Model != "" {
			r.model = rs.Model
		}
		if rs.Usage.PromptTokens > 0 || rs.Usage.CompletionTokens > 0 || rs.Usage.TotalTokens > 0 {
			pt := rs.Usage.PromptTokens
			ct := rs.Usage.CompletionTokens
			tt := rs.Usage.TotalTokens
			r.pt = &pt
			r.ct = &ct
			r.tt = &tt
		}
		r.mu.Unlock()
	}

	_ = targetURL
}

func (r *Recorder) Stats() Stats {
	r.mu.Lock()
	defer r.mu.Unlock()
	status := int(r.statusCode.Load())
	if status == 0 {
		status = http.StatusOK
	}
	return Stats{
		Provider:         r.provider,
		Model:            r.model,
		HTTPStatus:       status,
		PromptTokens:     r.pt,
		CompletionTokens: r.ct,
		TotalTokens:      r.tt,
		RetryCount:       int(r.retries.Load()),
		RequestBytes:     r.requestBytes.Load(),
		ResponseBytes:    r.respBytes.Load(),
	}
}

func (r *Recorder) ProxyEnv(addr string) []string {
	proxyURL := "http://" + addr
	return []string{
		"HTTP_PROXY=" + proxyURL,
		"HTTPS_PROXY=" + proxyURL,
		"http_proxy=" + proxyURL,
		"https_proxy=" + proxyURL,
		"ZAI_PROXY_ADDR=" + addr,
		"ZAI_PROXY_PORT=" + strconv.Itoa(portFromAddr(addr)),
	}
}

func portFromAddr(addr string) int {
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		return 0
	}
	p, _ := strconv.Atoi(port)
	return p
}

func detectProvider(host string) string {
	host = strings.ToLower(host)
	switch {
	case strings.Contains(host, "openai"):
		return "openai"
	case strings.Contains(host, "anthropic"):
		return "anthropic"
	case strings.Contains(host, "googleapis") || strings.Contains(host, "gemini"):
		return "gemini"
	default:
		return ""
	}
}

func WaitForProxyReady(addr string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 100*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	return fmt.Errorf("proxy did not become ready on %s within %s", addr, timeout)
}
