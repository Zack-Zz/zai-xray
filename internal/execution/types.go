package execution

import (
	"strings"

	"github.com/zhouze/zai-xray/internal/trace"
)

type RunOptions struct {
	Provider string
	Model    string
	Prompt   string
	Command  string
}

type RunResult struct {
	Trace    trace.Trace
	Artifact *trace.Artifact
	Output   string
}

type WrapOptions struct {
	Target  string
	BinPath string
	Args    []string
	NoProxy bool
	Env     []string
	Command string
}

type WrapResult struct {
	Trace     trace.Trace
	Artifact  *trace.Artifact
	ExitCode  int
	ProxyAddr string
}

func classifyError(err error) string {
	if err == nil {
		return ""
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "timeout"):
		return "timeout"
	case strings.Contains(msg, "429"):
		return "rate_limit"
	case strings.Contains(msg, "401") || strings.Contains(msg, "403"):
		return "auth"
	case strings.Contains(msg, "parse"):
		return "parse"
	default:
		return "unknown"
	}
}
