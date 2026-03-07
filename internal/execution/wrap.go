package execution

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/zhouze/zai-xray/internal/providers"
	"github.com/zhouze/zai-xray/internal/proxy"
	"github.com/zhouze/zai-xray/internal/store"
	"github.com/zhouze/zai-xray/internal/trace"
)

type Wrapper struct {
	store         store.Store
	openAIPricing providers.Provider
}

func NewWrapper(s store.Store) *Wrapper {
	return &Wrapper{
		store:         s,
		openAIPricing: providers.NewOpenAIProvider("placeholder", "", time.Second),
	}
}

func (w *Wrapper) Wrap(ctx context.Context, opts WrapOptions) (WrapResult, error) {
	traceID := trace.NewTraceID()
	start := time.Now().UTC()

	proxyCtx, cancelProxy := context.WithCancel(context.Background())
	defer cancelProxy()

	var rec *proxy.Recorder
	proxyAddr := ""
	if !opts.NoProxy {
		rec = proxy.NewRecorder()
		addr, err := rec.Start(proxyCtx)
		if err != nil {
			return WrapResult{}, err
		}
		proxyAddr = addr
		if err := proxy.WaitForProxyReady(addr, 2*time.Second); err != nil {
			return WrapResult{}, err
		}
	}

	cmdPath := opts.BinPath
	if strings.TrimSpace(cmdPath) == "" {
		cmdPath = opts.Target
	}
	cmd := exec.CommandContext(ctx, cmdPath, opts.Args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin

	env := append([]string{}, os.Environ()...)
	env = append(env, "ZAI_TRACE_ID="+traceID)
	if rec != nil {
		env = append(env, rec.ProxyEnv(proxyAddr)...)
	}
	env = append(env, opts.Env...)
	cmd.Env = env

	runErr := cmd.Run()
	end := time.Now().UTC()

	t := trace.Trace{
		TraceID:     traceID,
		Source:      "wrap:" + opts.Target,
		Command:     opts.Command,
		Status:      "ok",
		StartTimeMS: start.UnixMilli(),
		EndTimeMS:   end.UnixMilli(),
		LatencyMS:   end.Sub(start).Milliseconds(),
		CreatedAtMS: end.UnixMilli(),
	}
	exitCode := 0

	if runErr != nil {
		t.Status = "error"
		t.ErrorType = classifyError(runErr)
		t.ErrorMessage = runErr.Error()
		var ee *exec.ExitError
		if errors.As(runErr, &ee) {
			exitCode = ee.ExitCode()
		}
	}

	if rec != nil {
		st := rec.Stats()
		t.Provider = st.Provider
		t.Model = st.Model
		if st.HTTPStatus > 0 {
			h := st.HTTPStatus
			t.HTTPStatus = &h
		}
		t.PromptTokens = st.PromptTokens
		t.CompletionTokens = st.CompletionTokens
		t.TotalTokens = st.TotalTokens
		t.RetryCount = st.RetryCount
		t.RequestBytes = &st.RequestBytes
		t.ResponseBytes = &st.ResponseBytes
		if t.PromptTokens == nil || t.CompletionTokens == nil || t.TotalTokens == nil {
			pt, ct, tt := estimateTokensFromBytes(st.RequestBytes, st.ResponseBytes)
			t.PromptTokens = &pt
			t.CompletionTokens = &ct
			t.TotalTokens = &tt
			t.TokensEstimated = true
		}
		if t.Provider == "openai" && t.Model != "" && t.PromptTokens != nil && t.CompletionTokens != nil {
			if cost, ok := w.openAIPricing.EstimateCost(t.Model, *t.PromptTokens, *t.CompletionTokens); ok {
				t.CostUSD = &cost
				t.CostEstimated = t.TokensEstimated
			}
		}
	}

	if err := w.store.CreateTrace(ctx, t, trace.BuildArtifact(traceID, opts.Command, "", 320)); err != nil {
		return WrapResult{}, fmt.Errorf("store wrap trace: %w", err)
	}
	if runErr != nil {
		return WrapResult{Trace: t, ExitCode: exitCode, ProxyAddr: proxyAddr}, runErr
	}
	return WrapResult{Trace: t, ExitCode: 0, ProxyAddr: proxyAddr}, nil
}
