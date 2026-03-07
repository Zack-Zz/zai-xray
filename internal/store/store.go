package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/zhouze/zai-xray/internal/trace"
)

type Store interface {
	Migrate(ctx context.Context) error
	CreateTrace(ctx context.Context, t trace.Trace, a *trace.Artifact) error
	GetTrace(ctx context.Context, traceID string) (trace.TraceWithArtifact, error)
	ListTraces(ctx context.Context, filter trace.ListFilter) ([]trace.Trace, error)
	Stats(ctx context.Context, period trace.Period, groupBy string, now time.Time) (trace.StatsSummary, error)
}

type SQLiteStore struct {
	path string
}

func NewSQLite(path string) (*SQLiteStore, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("sqlite path is required")
	}
	return &SQLiteStore{path: path}, nil
}

func (s *SQLiteStore) Close() error { return nil }

func (s *SQLiteStore) Migrate(ctx context.Context) error {
	script := `
CREATE TABLE IF NOT EXISTS schema_migrations (
  version INTEGER PRIMARY KEY,
  applied_at_ms INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS traces (
  trace_id TEXT PRIMARY KEY,
  source TEXT NOT NULL,
  command TEXT NOT NULL,
  provider TEXT,
  model TEXT,
  status TEXT NOT NULL,
  http_status INTEGER,
  error_type TEXT,
  error_message TEXT,
  start_time_ms INTEGER NOT NULL,
  end_time_ms INTEGER NOT NULL,
  latency_ms INTEGER NOT NULL,
  prompt_tokens INTEGER,
  completion_tokens INTEGER,
  total_tokens INTEGER,
  tokens_estimated INTEGER DEFAULT 0,
  cost_usd REAL,
  cost_estimated INTEGER DEFAULT 0,
  retry_count INTEGER DEFAULT 0,
  request_bytes INTEGER,
  response_bytes INTEGER,
  created_at_ms INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_traces_created_at ON traces(created_at_ms);
CREATE INDEX IF NOT EXISTS idx_traces_source ON traces(source);
CREATE INDEX IF NOT EXISTS idx_traces_provider_model ON traces(provider, model);
CREATE TABLE IF NOT EXISTS artifacts (
  trace_id TEXT PRIMARY KEY,
  prompt_preview TEXT,
  response_preview TEXT,
  prompt_sha256 TEXT,
  response_sha256 TEXT
);
INSERT OR IGNORE INTO schema_migrations(version, applied_at_ms)
VALUES(1, ` + strconv.FormatInt(time.Now().UTC().UnixMilli(), 10) + `);
`
	return s.execSQL(ctx, script)
}

func (s *SQLiteStore) CreateTrace(ctx context.Context, t trace.Trace, a *trace.Artifact) error {
	traceSQL := fmt.Sprintf(`INSERT OR REPLACE INTO traces (
trace_id, source, command, provider, model, status, http_status, error_type, error_message,
start_time_ms, end_time_ms, latency_ms,
prompt_tokens, completion_tokens, total_tokens, tokens_estimated,
cost_usd, cost_estimated, retry_count, request_bytes, response_bytes, created_at_ms
) VALUES (
%s, %s, %s, %s, %s, %s, %s, %s, %s,
%d, %d, %d,
%s, %s, %s, %d,
%s, %d, %d, %s, %s, %d
);`,
		sqlText(t.TraceID), sqlText(t.Source), sqlText(t.Command), sqlNullableText(t.Provider), sqlNullableText(t.Model), sqlText(t.Status),
		sqlNullableInt(t.HTTPStatus), sqlNullableText(t.ErrorType), sqlNullableText(t.ErrorMessage),
		t.StartTimeMS, t.EndTimeMS, t.LatencyMS,
		sqlNullableInt(t.PromptTokens), sqlNullableInt(t.CompletionTokens), sqlNullableInt(t.TotalTokens), boolInt(t.TokensEstimated),
		sqlNullableFloat(t.CostUSD), boolInt(t.CostEstimated), t.RetryCount, sqlNullableInt64(t.RequestBytes), sqlNullableInt64(t.ResponseBytes), t.CreatedAtMS,
	)
	if err := s.execSQL(ctx, traceSQL); err != nil {
		return err
	}
	if a == nil {
		return nil
	}
	artifactSQL := fmt.Sprintf(`INSERT OR REPLACE INTO artifacts(trace_id, prompt_preview, response_preview, prompt_sha256, response_sha256)
VALUES (%s, %s, %s, %s, %s);`,
		sqlText(a.TraceID), sqlText(a.PromptPreview), sqlText(a.ResponsePreview), sqlText(a.PromptSHA256), sqlText(a.ResponseSHA256))
	return s.execSQL(ctx, artifactSQL)
}

func (s *SQLiteStore) GetTrace(ctx context.Context, traceID string) (trace.TraceWithArtifact, error) {
	q := `SELECT trace_id, source, command, provider, model, status, http_status, error_type, error_message,
start_time_ms, end_time_ms, latency_ms, prompt_tokens, completion_tokens, total_tokens, tokens_estimated,
cost_usd, cost_estimated, retry_count, request_bytes, response_bytes, created_at_ms
FROM traces WHERE trace_id = ` + sqlText(traceID) + ` LIMIT 1;`
	rows, err := s.queryJSON(ctx, q)
	if err != nil {
		return trace.TraceWithArtifact{}, err
	}
	if len(rows) == 0 {
		return trace.TraceWithArtifact{}, fmt.Errorf("trace not found")
	}
	t, err := mapToTrace(rows[0])
	if err != nil {
		return trace.TraceWithArtifact{}, err
	}

	aq := `SELECT trace_id, prompt_preview, response_preview, prompt_sha256, response_sha256
FROM artifacts WHERE trace_id = ` + sqlText(traceID) + ` LIMIT 1;`
	artRows, err := s.queryJSON(ctx, aq)
	if err != nil {
		return trace.TraceWithArtifact{}, err
	}
	if len(artRows) == 0 {
		return trace.TraceWithArtifact{Trace: t}, nil
	}
	a := &trace.Artifact{
		TraceID:         toString(artRows[0]["trace_id"]),
		PromptPreview:   toString(artRows[0]["prompt_preview"]),
		ResponsePreview: toString(artRows[0]["response_preview"]),
		PromptSHA256:    toString(artRows[0]["prompt_sha256"]),
		ResponseSHA256:  toString(artRows[0]["response_sha256"]),
	}
	return trace.TraceWithArtifact{Trace: t, Artifact: a}, nil
}

func (s *SQLiteStore) ListTraces(ctx context.Context, filter trace.ListFilter) ([]trace.Trace, error) {
	limit := filter.Limit
	if limit <= 0 {
		limit = 20
	}
	where := []string{}
	if strings.TrimSpace(filter.Source) != "" {
		where = append(where, "source = "+sqlText(filter.Source))
	}
	if filter.SinceMS != nil {
		where = append(where, fmt.Sprintf("created_at_ms >= %d", *filter.SinceMS))
	}
	q := `SELECT trace_id, source, command, provider, model, status, http_status, error_type, error_message,
start_time_ms, end_time_ms, latency_ms, prompt_tokens, completion_tokens, total_tokens, tokens_estimated,
cost_usd, cost_estimated, retry_count, request_bytes, response_bytes, created_at_ms FROM traces`
	if len(where) > 0 {
		q += " WHERE " + strings.Join(where, " AND ")
	}
	q += fmt.Sprintf(" ORDER BY created_at_ms DESC LIMIT %d;", limit)
	rows, err := s.queryJSON(ctx, q)
	if err != nil {
		return nil, err
	}
	out := make([]trace.Trace, 0, len(rows))
	for _, row := range rows {
		t, err := mapToTrace(row)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, nil
}

func (s *SQLiteStore) Stats(ctx context.Context, period trace.Period, groupBy string, now time.Time) (trace.StatsSummary, error) {
	start, end, err := trace.PeriodRange(period, now)
	if err != nil {
		return trace.StatsSummary{}, err
	}
	startMS := start.UTC().UnixMilli()
	endMS := end.UTC().UnixMilli()

	summary := trace.StatsSummary{Period: period}
	q := fmt.Sprintf(`SELECT
COUNT(*) AS total_calls,
SUM(CASE WHEN status = 'ok' THEN 1 ELSE 0 END) AS ok_calls,
SUM(CASE WHEN status = 'error' THEN 1 ELSE 0 END) AS error_calls,
COALESCE(SUM(cost_usd), 0) AS total_cost,
COALESCE(AVG(latency_ms), 0) AS avg_latency,
COALESCE(MAX(latency_ms), 0) AS max_latency,
COALESCE(SUM(total_tokens), 0) AS total_tokens
FROM traces WHERE created_at_ms >= %d AND created_at_ms < %d;`, startMS, endMS)
	rows, err := s.queryJSON(ctx, q)
	if err != nil {
		return trace.StatsSummary{}, err
	}
	if len(rows) > 0 {
		r := rows[0]
		summary.TotalCalls = toInt64(r["total_calls"])
		summary.OKCalls = toInt64(r["ok_calls"])
		summary.ErrorCalls = toInt64(r["error_calls"])
		summary.TotalCost = toFloat64(r["total_cost"])
		summary.AvgLatencyMS = toFloat64(r["avg_latency"])
		summary.MaxLatencyMS = toInt64(r["max_latency"])
		summary.TotalTokens = toInt64(r["total_tokens"])
	}

	summary.TopSlow, err = s.topTraces(ctx, startMS, endMS, "latency_ms DESC", 5)
	if err != nil {
		return trace.StatsSummary{}, err
	}
	summary.TopExpensive, err = s.topTraces(ctx, startMS, endMS, "cost_usd DESC", 5)
	if err != nil {
		return trace.StatsSummary{}, err
	}
	if groupBy != "" {
		summary.Groups, err = s.groupStats(ctx, startMS, endMS, groupBy)
		if err != nil {
			return trace.StatsSummary{}, err
		}
	}
	return summary, nil
}

func (s *SQLiteStore) topTraces(ctx context.Context, startMS, endMS int64, order string, limit int) ([]trace.Trace, error) {
	q := fmt.Sprintf(`SELECT trace_id, source, command, provider, model, status, http_status, error_type, error_message,
start_time_ms, end_time_ms, latency_ms, prompt_tokens, completion_tokens, total_tokens, tokens_estimated,
cost_usd, cost_estimated, retry_count, request_bytes, response_bytes, created_at_ms
FROM traces WHERE created_at_ms >= %d AND created_at_ms < %d ORDER BY %s LIMIT %d;`, startMS, endMS, order, limit)
	rows, err := s.queryJSON(ctx, q)
	if err != nil {
		return nil, err
	}
	out := make([]trace.Trace, 0, len(rows))
	for _, row := range rows {
		t, err := mapToTrace(row)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, nil
}

func (s *SQLiteStore) groupStats(ctx context.Context, startMS, endMS int64, groupBy string) ([]trace.GroupItem, error) {
	allowed := map[string]string{"provider": "provider", "model": "model", "source": "source"}
	col, ok := allowed[groupBy]
	if !ok {
		return nil, fmt.Errorf("invalid --by value: %s", groupBy)
	}
	q := fmt.Sprintf(`SELECT COALESCE(%s, 'unknown') AS key, COUNT(*) AS cnt,
COALESCE(SUM(cost_usd), 0) AS total_cost, COALESCE(AVG(latency_ms), 0) AS avg_lat
FROM traces WHERE created_at_ms >= %d AND created_at_ms < %d
GROUP BY key ORDER BY cnt DESC LIMIT 20;`, col, startMS, endMS)
	rows, err := s.queryJSON(ctx, q)
	if err != nil {
		return nil, err
	}
	items := make([]trace.GroupItem, 0, len(rows))
	for _, row := range rows {
		items = append(items, trace.GroupItem{
			Key:       toString(row["key"]),
			Count:     toInt64(row["cnt"]),
			TotalCost: toFloat64(row["total_cost"]),
			AvgLatMS:  toFloat64(row["avg_lat"]),
		})
	}
	return items, nil
}

func (s *SQLiteStore) execSQL(ctx context.Context, sql string) error {
	cmd := exec.CommandContext(ctx, "sqlite3", s.path, sql)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("sqlite3 exec failed: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func (s *SQLiteStore) queryJSON(ctx context.Context, sql string) ([]map[string]any, error) {
	// Use sqlite3 built-in JSON mode to keep parsing logic centralized in Go.
	cmd := exec.CommandContext(ctx, "sqlite3", "-json", s.path, sql)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("sqlite3 query failed: %w: %s", err, strings.TrimSpace(string(out)))
	}
	trimmed := strings.TrimSpace(string(out))
	if trimmed == "" {
		return nil, nil
	}
	var rows []map[string]any
	if err := json.Unmarshal(out, &rows); err != nil {
		return nil, fmt.Errorf("parse sqlite json output: %w", err)
	}
	return rows, nil
}

func mapToTrace(row map[string]any) (trace.Trace, error) {
	// sqlite3 -json returns numbers as float64; normalize into typed trace fields.
	t := trace.Trace{
		TraceID:         toString(row["trace_id"]),
		Source:          toString(row["source"]),
		Command:         toString(row["command"]),
		Provider:        toString(row["provider"]),
		Model:           toString(row["model"]),
		Status:          toString(row["status"]),
		ErrorType:       toString(row["error_type"]),
		ErrorMessage:    toString(row["error_message"]),
		StartTimeMS:     toInt64(row["start_time_ms"]),
		EndTimeMS:       toInt64(row["end_time_ms"]),
		LatencyMS:       toInt64(row["latency_ms"]),
		RetryCount:      int(toInt64(row["retry_count"])),
		CreatedAtMS:     toInt64(row["created_at_ms"]),
		TokensEstimated: toInt64(row["tokens_estimated"]) == 1,
		CostEstimated:   toInt64(row["cost_estimated"]) == 1,
	}
	if v := toIntPointer(row["http_status"]); v != nil {
		t.HTTPStatus = v
	}
	t.PromptTokens = toIntPointer(row["prompt_tokens"])
	t.CompletionTokens = toIntPointer(row["completion_tokens"])
	t.TotalTokens = toIntPointer(row["total_tokens"])
	t.CostUSD = toFloatPointer(row["cost_usd"])
	t.RequestBytes = toInt64Pointer(row["request_bytes"])
	t.ResponseBytes = toInt64Pointer(row["response_bytes"])
	if t.TraceID == "" {
		return trace.Trace{}, errors.New("invalid trace row")
	}
	return t, nil
}

func sqlText(s string) string {
	s = strings.ReplaceAll(s, "'", "''")
	return "'" + s + "'"
}

func sqlNullableText(s string) string {
	if strings.TrimSpace(s) == "" {
		return "NULL"
	}
	return sqlText(s)
}

func sqlNullableInt(v *int) string {
	if v == nil {
		return "NULL"
	}
	return strconv.Itoa(*v)
}

func sqlNullableInt64(v *int64) string {
	if v == nil {
		return "NULL"
	}
	return strconv.FormatInt(*v, 10)
}

func sqlNullableFloat(v *float64) string {
	if v == nil {
		return "NULL"
	}
	return strconv.FormatFloat(*v, 'f', -1, 64)
}

func boolInt(v bool) int {
	if v {
		return 1
	}
	return 0
}

func toString(v any) string {
	if v == nil {
		return ""
	}
	s, ok := v.(string)
	if ok {
		return s
	}
	return fmt.Sprintf("%v", v)
}

func toInt64(v any) int64 {
	if v == nil {
		return 0
	}
	switch n := v.(type) {
	case float64:
		return int64(n)
	case int64:
		return n
	case int:
		return int64(n)
	case string:
		i, _ := strconv.ParseInt(n, 10, 64)
		return i
	default:
		return 0
	}
}

func toFloat64(v any) float64 {
	if v == nil {
		return 0
	}
	switch n := v.(type) {
	case float64:
		return n
	case int64:
		return float64(n)
	case string:
		f, _ := strconv.ParseFloat(n, 64)
		return f
	default:
		return 0
	}
}

func toIntPointer(v any) *int {
	if v == nil {
		return nil
	}
	i := int(toInt64(v))
	return &i
}

func toInt64Pointer(v any) *int64 {
	if v == nil {
		return nil
	}
	i := toInt64(v)
	return &i
}

func toFloatPointer(v any) *float64 {
	if v == nil {
		return nil
	}
	f := toFloat64(v)
	return &f
}
