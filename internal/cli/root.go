package cli

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/zhouze/zai-xray/internal/app"
	"github.com/zhouze/zai-xray/internal/execution"
	"github.com/zhouze/zai-xray/internal/trace"
)

func Execute() error {
	if len(os.Args) < 2 {
		printRootHelp()
		return nil
	}

	globalArgs, rest, parseErr := extractRootFlags(os.Args[1:])
	if parseErr != nil {
		return parseErr
	}
	if len(rest) == 0 {
		printRootHelp()
		return nil
	}

	cmd := rest[0]
	args := append(globalArgs, rest[1:]...)
	var err error
	switch cmd {
	case "run":
		err = execRun(args)
	case "wrap":
		err = execWrap(args)
	case "trace":
		err = execTrace(args)
	case "stats":
		err = execStats(args)
	case "doctor":
		err = execDoctor(args)
	case "-h", "--help", "help":
		printRootHelp()
		return nil
	default:
		return fmt.Errorf("unknown command: %s", cmd)
	}
	if err != nil {
		return err
	}
	return nil
}

func extractRootFlags(args []string) ([]string, []string, error) {
	global := make([]string, 0, 8)
	i := 0
	for i < len(args) {
		arg := args[i]
		if !strings.HasPrefix(arg, "-") || arg == "-" {
			break
		}
		if arg == "--" {
			i++
			break
		}
		switch {
		case strings.HasPrefix(arg, "--config="),
			arg == "--json",
			arg == "--one-line",
			arg == "--pretty",
			arg == "--no-color":
			global = append(global, arg)
			i++
		case arg == "--config":
			if i+1 >= len(args) {
				return nil, nil, fmt.Errorf("flag needs an argument: --config")
			}
			global = append(global, arg, args[i+1])
			i += 2
		default:
			return nil, nil, fmt.Errorf("unknown command: %s", arg)
		}
	}
	return global, args[i:], nil
}

func execRun(args []string) error {
	fs := newFlagSet("run")
	common := bindCommonFlags(fs)
	var provider string
	var model string
	var useStdin bool
	var timeout string
	fs.StringVar(&provider, "provider", "", "provider name (openai)")
	fs.StringVar(&model, "model", "", "model name")
	fs.BoolVar(&useStdin, "stdin", false, "read additional input from stdin")
	fs.StringVar(&timeout, "timeout", "30s", "request timeout")
	rest, err := parseArgsInterspersed(fs, args)
	if err != nil {
		return err
	}
	if useStdin && len(rest) == 0 {
		return fmt.Errorf("prompt is required")
	}
	if !useStdin && len(rest) != 1 {
		return fmt.Errorf("exactly one prompt argument is required")
	}

	a, err := app.New(common.ConfigPath, common.Output)
	if err != nil {
		return err
	}
	defer a.Close()

	prompt := strings.Join(rest, " ")
	if useStdin {
		stdin, err := readAllStdin()
		if err != nil {
			return err
		}
		if strings.TrimSpace(stdin) != "" {
			prompt = strings.TrimSpace(prompt) + "\n\nInput:\n" + stdin
		}
	}

	runner := execution.NewRunner(a.Store, a.Runner)
	effProvider := app.EffectiveProvider(provider, a.Config)
	effModel := app.EffectiveModel(model, a.Config)
	cmdline := "zai run " + strings.Join(rest, " ")

	td, err := time.ParseDuration(timeout)
	if err != nil {
		return fmt.Errorf("invalid timeout: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), td)
	defer cancel()

	res, runErr := runner.Run(ctx, execution.RunOptions{Provider: effProvider, Model: effModel, Prompt: prompt, Command: cmdline})
	payload := map[string]any{
		"trace_id":    res.Trace.TraceID,
		"source":      res.Trace.Source,
		"provider":    res.Trace.Provider,
		"model":       res.Trace.Model,
		"latency_ms":  res.Trace.LatencyMS,
		"http_status": res.Trace.HTTPStatus,
		"tokens": map[string]any{
			"in":        res.Trace.PromptTokens,
			"out":       res.Trace.CompletionTokens,
			"total":     res.Trace.TotalTokens,
			"estimated": res.Trace.TokensEstimated,
		},
		"cost_usd":       res.Trace.CostUSD,
		"cost_estimated": res.Trace.CostEstimated,
		"status":         res.Trace.Status,
		"retry_count":    res.Trace.RetryCount,
		"output":         res.Output,
	}
	if runErr != nil {
		payload["error"] = runErr.Error()
	}
	if err := a.Render.RenderRun(payload); err != nil {
		return err
	}
	return runErr
}

func execWrap(args []string) error {
	fs := newFlagSet("wrap")
	common := bindCommonFlags(fs)
	var envs strSlice
	var binPath string
	var noProxy bool
	var timeout string
	fs.Var(&envs, "env", "extra env vars in KEY=VALUE format")
	fs.StringVar(&binPath, "bin", "", "explicit binary path")
	fs.BoolVar(&noProxy, "no-proxy", false, "disable proxy injection")
	fs.StringVar(&timeout, "timeout", "2h", "wrap execution timeout")
	rest, err := parseArgsInterspersed(fs, args)
	if err != nil {
		return err
	}
	if len(rest) == 0 {
		return fmt.Errorf("target command is required")
	}

	target := rest[0]
	targetArgs := rest[1:]
	for i, v := range rest {
		if v == "--" {
			target = rest[0]
			targetArgs = rest[i+1:]
			break
		}
	}

	a, err := app.New(common.ConfigPath, common.Output)
	if err != nil {
		return err
	}
	defer a.Close()

	td, err := time.ParseDuration(timeout)
	if err != nil {
		return fmt.Errorf("invalid timeout: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), td)
	defer cancel()

	wrapper := execution.NewWrapper(a.Store)
	commandLine := "zai wrap " + strings.Join(rest, " ")
	res, wrapErr := wrapper.Wrap(ctx, execution.WrapOptions{
		Target:  target,
		BinPath: binPath,
		Args:    targetArgs,
		NoProxy: noProxy,
		Env:     envs,
		Command: commandLine,
	})
	payload := map[string]any{
		"trace_id":    res.Trace.TraceID,
		"source":      res.Trace.Source,
		"provider":    res.Trace.Provider,
		"model":       res.Trace.Model,
		"latency_ms":  res.Trace.LatencyMS,
		"http_status": res.Trace.HTTPStatus,
		"tokens": map[string]any{
			"in":        res.Trace.PromptTokens,
			"out":       res.Trace.CompletionTokens,
			"total":     res.Trace.TotalTokens,
			"estimated": res.Trace.TokensEstimated,
		},
		"cost_usd":       res.Trace.CostUSD,
		"cost_estimated": res.Trace.CostEstimated,
		"status":         res.Trace.Status,
		"retry_count":    res.Trace.RetryCount,
		"proxy_addr":     res.ProxyAddr,
		"exit_code":      res.ExitCode,
	}
	if wrapErr != nil {
		payload["error"] = wrapErr.Error()
	}
	if err := a.Render.RenderWrap(payload); err != nil {
		return err
	}
	return wrapErr
}

func execTrace(args []string) error {
	if len(args) == 0 {
		printTraceHelp()
		return nil
	}
	sub := args[0]
	subArgs := args[1:]
	switch sub {
	case "list":
		return execTraceList(subArgs)
	case "show":
		return execTraceShow(subArgs)
	default:
		return fmt.Errorf("unknown trace subcommand: %s", sub)
	}
}

func execTraceList(args []string) error {
	fs := newFlagSet("trace list")
	common := bindCommonFlags(fs)
	var limit int
	var source string
	var since string
	fs.IntVar(&limit, "limit", 20, "max rows")
	fs.StringVar(&source, "source", "", "filter by source")
	fs.StringVar(&since, "since", "", "filter by duration, e.g. 24h")
	_, err := parseArgsInterspersed(fs, args)
	if err != nil {
		return err
	}
	a, err := app.New(common.ConfigPath, common.Output)
	if err != nil {
		return err
	}
	defer a.Close()

	filter := trace.ListFilter{Limit: limit, Source: source}
	if strings.TrimSpace(since) != "" {
		d, err := time.ParseDuration(since)
		if err != nil {
			return fmt.Errorf("invalid --since: %w", err)
		}
		sinceMS := time.Now().Add(-d).UnixMilli()
		filter.SinceMS = &sinceMS
	}
	items, err := a.Store.ListTraces(context.Background(), filter)
	if err != nil {
		return err
	}
	if len(items) == 0 {
		fmt.Fprintln(os.Stdout, "No traces found")
		return nil
	}
	return a.Render.RenderTraceList(items)
}

func execTraceShow(args []string) error {
	fs := newFlagSet("trace show")
	common := bindCommonFlags(fs)
	var full bool
	fs.BoolVar(&full, "full", false, "show hashes and full metadata")
	rest, err := parseArgsInterspersed(fs, args)
	if err != nil {
		return err
	}
	if len(rest) != 1 {
		return fmt.Errorf("trace id is required")
	}
	a, err := app.New(common.ConfigPath, common.Output)
	if err != nil {
		return err
	}
	defer a.Close()

	item, err := a.Store.GetTrace(context.Background(), rest[0])
	if err != nil {
		return err
	}
	return a.Render.RenderTraceShow(item, full)
}

func execStats(args []string) error {
	fs := newFlagSet("stats")
	common := bindCommonFlags(fs)
	var by string
	fs.StringVar(&by, "by", "", "group by provider|model|source")
	rest, err := parseArgsInterspersed(fs, args)
	if err != nil {
		return err
	}
	period := trace.PeriodToday
	if len(rest) > 0 {
		period = trace.Period(strings.ToLower(rest[0]))
	}
	if len(rest) > 1 {
		return fmt.Errorf("at most one period argument allowed")
	}

	a, err := app.New(common.ConfigPath, common.Output)
	if err != nil {
		return err
	}
	defer a.Close()

	summary, err := a.Store.Stats(context.Background(), period, by, time.Now())
	if err != nil {
		return err
	}
	return a.Render.RenderStats(summary)
}

func execDoctor(args []string) error {
	fs := newFlagSet("doctor")
	common := bindCommonFlags(fs)
	if _, err := parseArgsInterspersed(fs, args); err != nil {
		return err
	}
	a, err := app.New(common.ConfigPath, common.Output)
	if err != nil {
		return err
	}
	defer a.Close()
	checks := app.RunDoctor(context.Background(), a.Config)
	return a.Render.RenderDoctor(checks)
}

type commonFlags struct {
	ConfigPath string
	Output     app.OutputFlags
}

func bindCommonFlags(fs *flag.FlagSet) *commonFlags {
	c := &commonFlags{}
	fs.StringVar(&c.ConfigPath, "config", "", "config file path (default ~/.zai/config.yaml)")
	fs.BoolVar(&c.Output.JSON, "json", false, "output JSON")
	fs.BoolVar(&c.Output.OneLine, "one-line", false, "output one-line summary")
	fs.BoolVar(&c.Output.Pretty, "pretty", false, "output pretty format (default)")
	fs.BoolVar(&c.Output.NoColor, "no-color", false, "disable color output")
	return c
}

func newFlagSet(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	return fs
}

func parseArgsInterspersed(fs *flag.FlagSet, args []string) ([]string, error) {
	flagArgs := make([]string, 0, len(args))
	positionals := make([]string, 0, len(args))

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			positionals = append(positionals, args[i+1:]...)
			break
		}
		if !strings.HasPrefix(arg, "-") || arg == "-" {
			positionals = append(positionals, arg)
			continue
		}

		name := strings.TrimLeft(arg, "-")
		hasInlineValue := strings.Contains(name, "=")
		if hasInlineValue {
			name = strings.SplitN(name, "=", 2)[0]
		}

		f := fs.Lookup(name)
		if f == nil {
			positionals = append(positionals, arg)
			continue
		}

		flagArgs = append(flagArgs, arg)
		if hasInlineValue {
			continue
		}
		if bf, ok := f.Value.(interface{ IsBoolFlag() bool }); ok && bf.IsBoolFlag() {
			continue
		}
		if i+1 >= len(args) {
			return nil, fmt.Errorf("flag needs an argument: %s", arg)
		}
		flagArgs = append(flagArgs, args[i+1])
		i++
	}

	if err := fs.Parse(flagArgs); err != nil {
		return nil, err
	}
	rest := append(fs.Args(), positionals...)
	return rest, nil
}

func readAllStdin() (string, error) {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return "", err
	}
	if (fi.Mode() & os.ModeCharDevice) != 0 {
		return "", nil
	}
	b, err := io.ReadAll(os.Stdin)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

type strSlice []string

func (s *strSlice) String() string {
	return strings.Join(*s, ",")
}

func (s *strSlice) Set(value string) error {
	*s = append(*s, value)
	return nil
}

func printRootHelp() {
	fmt.Println("zai - AI tracing and debugging CLI")
	fmt.Println("Commands: run, wrap, trace, stats, doctor")
}

func printTraceHelp() {
	fmt.Println("zai trace subcommands: list, show")
}
