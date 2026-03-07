package render

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/zhouze/zai-xray/internal/trace"
)

type Mode string

const (
	ModePretty  Mode = "pretty"
	ModeJSON    Mode = "json"
	ModeOneLine Mode = "one-line"
)

type Options struct {
	Mode     Mode
	UseColor bool
}

type Renderer struct {
	w    io.Writer
	opts Options
}

type DoctorCheck struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Message string `json:"message"`
}

func New(w io.Writer, opts Options) *Renderer {
	if opts.Mode == "" {
		opts.Mode = ModePretty
	}
	return &Renderer{w: w, opts: opts}
}

func DefaultUseColor(noColor bool) bool {
	if noColor {
		return false
	}
	if strings.TrimSpace(os.Getenv("NO_COLOR")) != "" {
		return false
	}
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	// Treat non-TTY output as log/pipe mode and disable ANSI colors.
	return (fi.Mode() & os.ModeCharDevice) != 0
}

func (r *Renderer) RenderRun(res map[string]any) error {
	return r.renderGeneric(res, "run")
}

func (r *Renderer) RenderWrap(res map[string]any) error {
	return r.renderGeneric(res, "wrap")
}

func (r *Renderer) RenderTraceList(items []trace.Trace) error {
	switch r.opts.Mode {
	case ModeJSON:
		return writeJSON(r.w, items)
	case ModeOneLine:
		for _, t := range items {
			fmt.Fprintln(r.w, oneLine(t))
		}
		return nil
	default:
		fmt.Fprintln(r.w, title(r.opts.UseColor, "TRACE LIST"))
		tw := tabwriter.NewWriter(r.w, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "TRACE_ID\tSTATUS\tPROVIDER\tMODEL\tLATENCY\tTOKENS\tCOST\tTIME")
		for _, t := range items {
			fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
				shortID(t.TraceID), statusCell(r.opts.UseColor, t.Status), orDash(t.Provider), orDash(t.Model),
				fmtLatency(t.LatencyMS), fmtTokens(t), fmtCost(t.CostUSD), fmtTime(t.CreatedAtMS))
		}
		return tw.Flush()
	}
}

func (r *Renderer) RenderTraceShow(item trace.TraceWithArtifact, full bool) error {
	switch r.opts.Mode {
	case ModeJSON:
		return writeJSON(r.w, item)
	case ModeOneLine:
		fmt.Fprintln(r.w, oneLine(item.Trace))
		return nil
	default:
		t := item.Trace
		fmt.Fprintln(r.w, title(r.opts.UseColor, "TRACE DETAIL"))
		fmt.Fprintf(r.w, "trace_id: %s  status: %s\n", t.TraceID, statusCell(r.opts.UseColor, t.Status))
		fmt.Fprintf(r.w, "source: %s  provider: %s  model: %s\n", t.Source, orDash(t.Provider), orDash(t.Model))
		fmt.Fprintf(r.w, "latency: %s  tokens: %s  cost: %s  retry: %d\n", fmtLatency(t.LatencyMS), fmtTokens(t), fmtCost(t.CostUSD), t.RetryCount)
		fmt.Fprintf(r.w, "http_status: %s  request_bytes: %s  response_bytes: %s\n", fmtHTTPStatus(t.HTTPStatus), fmtI64(t.RequestBytes), fmtI64(t.ResponseBytes))
		if t.ErrorMessage != "" {
			fmt.Fprintf(r.w, "error: (%s) %s\n", orDash(t.ErrorType), t.ErrorMessage)
		}
		fmt.Fprintf(r.w, "time: %s -> %s\n", fmtTime(t.StartTimeMS), fmtTime(t.EndTimeMS))
		if item.Artifact != nil {
			fmt.Fprintf(r.w, "prompt_preview: %s\n", sanitize(item.Artifact.PromptPreview))
			fmt.Fprintf(r.w, "response_preview: %s\n", sanitize(item.Artifact.ResponsePreview))
			if full {
				fmt.Fprintf(r.w, "prompt_sha256: %s\n", item.Artifact.PromptSHA256)
				fmt.Fprintf(r.w, "response_sha256: %s\n", item.Artifact.ResponseSHA256)
			}
		}
		return nil
	}
}

func (r *Renderer) RenderStats(summary trace.StatsSummary) error {
	switch r.opts.Mode {
	case ModeJSON:
		return writeJSON(r.w, summary)
	case ModeOneLine:
		fmt.Fprintf(r.w, "STATS period=%s calls=%d ok=%d error=%d cost=%s avg_latency=%s\n",
			summary.Period, summary.TotalCalls, summary.OKCalls, summary.ErrorCalls, fmtCost(&summary.TotalCost), fmtLatency(int64(summary.AvgLatencyMS)))
		return nil
	default:
		fmt.Fprintln(r.w, title(r.opts.UseColor, "STATS"))
		fmt.Fprintf(r.w, "period: %s  calls: %d  ok: %d  error: %d\n", summary.Period, summary.TotalCalls, summary.OKCalls, summary.ErrorCalls)
		fmt.Fprintf(r.w, "cost: %s  avg_latency: %s  max_latency: %s  total_tokens: %d\n",
			fmtCost(&summary.TotalCost), fmtLatency(int64(summary.AvgLatencyMS)), fmtLatency(summary.MaxLatencyMS), summary.TotalTokens)

		if len(summary.TopSlow) > 0 {
			fmt.Fprintln(r.w, "\nTop Slow")
			tw := tabwriter.NewWriter(r.w, 0, 0, 2, ' ', 0)
			fmt.Fprintln(tw, "TRACE_ID\tMODEL\tLATENCY\tSTATUS")
			for _, t := range summary.TopSlow {
				fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", shortID(t.TraceID), orDash(t.Model), fmtLatency(t.LatencyMS), t.Status)
			}
			_ = tw.Flush()
		}
		if len(summary.TopExpensive) > 0 {
			fmt.Fprintln(r.w, "\nTop Expensive")
			tw := tabwriter.NewWriter(r.w, 0, 0, 2, ' ', 0)
			fmt.Fprintln(tw, "TRACE_ID\tMODEL\tCOST\tSTATUS")
			for _, t := range summary.TopExpensive {
				fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n", shortID(t.TraceID), orDash(t.Model), fmtCost(t.CostUSD), t.Status)
			}
			_ = tw.Flush()
		}
		if len(summary.Groups) > 0 {
			fmt.Fprintln(r.w, "\nBy Group")
			tw := tabwriter.NewWriter(r.w, 0, 0, 2, ' ', 0)
			fmt.Fprintln(tw, "KEY\tCOUNT\tTOTAL_COST\tAVG_LATENCY")
			for _, g := range summary.Groups {
				fmt.Fprintf(tw, "%s\t%d\t%s\t%s\n", g.Key, g.Count, fmtCost(&g.TotalCost), fmtLatency(int64(g.AvgLatMS)))
			}
			_ = tw.Flush()
		}
		return nil
	}
}

func (r *Renderer) RenderDoctor(checks []DoctorCheck) error {
	switch r.opts.Mode {
	case ModeJSON:
		return writeJSON(r.w, checks)
	case ModeOneLine:
		for _, c := range checks {
			fmt.Fprintf(r.w, "DOCTOR %s status=%s message=%q\n", c.Name, c.Status, c.Message)
		}
		return nil
	default:
		fmt.Fprintln(r.w, title(r.opts.UseColor, "DOCTOR"))
		tw := tabwriter.NewWriter(r.w, 0, 0, 2, ' ', 0)
		fmt.Fprintln(tw, "CHECK\tSTATUS\tMESSAGE")
		for _, c := range checks {
			fmt.Fprintf(tw, "%s\t%s\t%s\n", c.Name, statusCell(r.opts.UseColor, strings.ToLower(c.Status)), c.Message)
		}
		return tw.Flush()
	}
}

func (r *Renderer) renderGeneric(res map[string]any, kind string) error {
	switch r.opts.Mode {
	case ModeJSON:
		return writeJSON(r.w, res)
	case ModeOneLine:
		fmt.Fprintf(r.w, "%s trace=%v model=%v latency=%s tokens=%s cost=%s status=%v retry=%v\n",
			strings.ToUpper(kind),
			res["trace_id"],
			emptyToDash(fmt.Sprint(res["model"])),
			fmtLatency(anyToInt64(res["latency_ms"])),
			formatTokenMap(res["tokens"]),
			formatGenericCost(res["cost_usd"]),
			res["status"],
			res["retry_count"],
		)
		return nil
	default:
		fmt.Fprintln(r.w, title(r.opts.UseColor, strings.ToUpper(kind)+" RESULT"))
		fmt.Fprintf(r.w, "trace_id: %v  status: %v\n", res["trace_id"], statusCell(r.opts.UseColor, fmt.Sprint(res["status"])))
		fmt.Fprintf(r.w, "source: %v  provider: %v  model: %v\n", res["source"], emptyToDash(fmt.Sprint(res["provider"])), emptyToDash(fmt.Sprint(res["model"])))
		fmt.Fprintf(r.w, "latency: %s  tokens: %s  cost: %s  retry: %v\n",
			fmtLatency(anyToInt64(res["latency_ms"])), formatTokenMap(res["tokens"]), formatGenericCost(res["cost_usd"]), res["retry_count"])
		if v, ok := res["http_status"]; ok && v != nil {
			fmt.Fprintf(r.w, "http_status: %s\n", formatHTTPAny(v))
		}
		if v, ok := res["proxy_addr"]; ok && v != nil && fmt.Sprint(v) != "" {
			fmt.Fprintf(r.w, "proxy_addr: %v  exit_code: %v\n", v, res["exit_code"])
		}
		if v, ok := res["output"]; ok {
			fmt.Fprintf(r.w, "output:\n%s\n", v)
		}
		if v, ok := res["error"]; ok && v != nil && v != "" {
			fmt.Fprintf(r.w, "error: %v\n", v)
		}
		return nil
	}
}

func anyToInt64(v any) int64 {
	switch x := v.(type) {
	case int64:
		return x
	case int:
		return int64(x)
	case float64:
		return int64(x)
	case string:
		var n int64
		_, _ = fmt.Sscanf(x, "%d", &n)
		return n
	default:
		return 0
	}
}

func formatTokenMap(v any) string {
	m, ok := v.(map[string]any)
	if !ok {
		return "-"
	}
	getPart := func(key string) string {
		raw, exists := m[key]
		if !exists || raw == nil {
			return "-"
		}
		switch val := raw.(type) {
		case *int:
			if val == nil {
				return "-"
			}
			return fmt.Sprintf("%d", *val)
		case int:
			return fmt.Sprintf("%d", val)
		case int64:
			return fmt.Sprintf("%d", val)
		case float64:
			return fmt.Sprintf("%d", int64(val))
		default:
			s := fmt.Sprint(val)
			if s == "" || s == "<nil>" {
				return "-"
			}
			return s
		}
	}
	estimated := false
	if raw, ok := m["estimated"]; ok {
		if b, ok := raw.(bool); ok {
			estimated = b
		}
	}
	suffix := ""
	if estimated {
		suffix = " (est)"
	}
	return fmt.Sprintf("in:%s out:%s total:%s%s", getPart("in"), getPart("out"), getPart("total"), suffix)
}

func formatGenericCost(v any) string {
	switch x := v.(type) {
	case *float64:
		return fmtCost(x)
	case float64:
		return fmt.Sprintf("$%.6f", x)
	default:
		return "-"
	}
}

func formatHTTPAny(v any) string {
	switch x := v.(type) {
	case *int:
		if x == nil {
			return "-"
		}
		return fmt.Sprintf("%d", *x)
	case int:
		return fmt.Sprintf("%d", x)
	case int64:
		return fmt.Sprintf("%d", x)
	case float64:
		return fmt.Sprintf("%d", int64(x))
	default:
		s := fmt.Sprint(v)
		if s == "" || s == "<nil>" {
			return "-"
		}
		return s
	}
}

func emptyToDash(s string) string {
	if s == "" || s == "<nil>" {
		return "-"
	}
	return s
}

func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func oneLine(t trace.Trace) string {
	return fmt.Sprintf("TRACE %s model=%s latency=%s tokens=%s cost=%s status=%s",
		shortID(t.TraceID), orDash(t.Model), fmtLatency(t.LatencyMS), fmtTokens(t), fmtCost(t.CostUSD), t.Status)
}

func fmtLatency(ms int64) string {
	if ms < 0 {
		return "-"
	}
	d := time.Duration(ms) * time.Millisecond
	if d >= time.Second {
		return fmt.Sprintf("%.2fs", d.Seconds())
	}
	return fmt.Sprintf("%dms", ms)
}

func fmtTokens(t trace.Trace) string {
	if t.PromptTokens == nil && t.CompletionTokens == nil && t.TotalTokens == nil {
		return "-"
	}
	return fmt.Sprintf("in:%s out:%s total:%s", fmtInt(t.PromptTokens), fmtInt(t.CompletionTokens), fmtInt(t.TotalTokens))
}

func fmtCost(cost *float64) string {
	if cost == nil {
		return "-"
	}
	return fmt.Sprintf("$%.6f", *cost)
}

func fmtI64(v *int64) string {
	if v == nil {
		return "-"
	}
	return fmt.Sprintf("%d", *v)
}

func fmtInt(v *int) string {
	if v == nil {
		return "-"
	}
	return fmt.Sprintf("%d", *v)
}

func fmtHTTPStatus(v *int) string {
	if v == nil {
		return "-"
	}
	return fmt.Sprintf("%d", *v)
}

func fmtTime(ms int64) string {
	if ms <= 0 {
		return "-"
	}
	return time.UnixMilli(ms).Local().Format("2006-01-02 15:04:05")
}

func shortID(id string) string {
	if len(id) <= 6 {
		return id
	}
	return id[:6]
}

func orDash(s string) string {
	if strings.TrimSpace(s) == "" {
		return "-"
	}
	return s
}

func title(useColor bool, text string) string {
	if !useColor {
		return "== " + text + " =="
	}
	return "\033[1;36m== " + text + " ==\033[0m"
}

func statusCell(useColor bool, status string) string {
	s := strings.ToLower(strings.TrimSpace(status))
	if !useColor {
		return s
	}
	switch s {
	case "ok", "pass":
		return "\033[32m" + s + "\033[0m"
	case "warn":
		return "\033[33m" + s + "\033[0m"
	case "error", "fail":
		return "\033[31m" + s + "\033[0m"
	default:
		return s
	}
}

func sanitize(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.TrimSpace(s)
	if len(s) > 240 {
		return s[:240] + "..."
	}
	return s
}
