package trace

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

type Trace struct {
	TraceID          string
	Source           string
	Command          string
	Provider         string
	Model            string
	Status           string
	HTTPStatus       *int
	ErrorType        string
	ErrorMessage     string
	StartTimeMS      int64
	EndTimeMS        int64
	LatencyMS        int64
	PromptTokens     *int
	CompletionTokens *int
	TotalTokens      *int
	TokensEstimated  bool
	CostUSD          *float64
	CostEstimated    bool
	RetryCount       int
	RequestBytes     *int64
	ResponseBytes    *int64
	CreatedAtMS      int64
}

type Artifact struct {
	TraceID         string
	PromptPreview   string
	ResponsePreview string
	PromptSHA256    string
	ResponseSHA256  string
}

type TraceWithArtifact struct {
	Trace    Trace
	Artifact *Artifact
}

type ListFilter struct {
	Limit   int
	Source  string
	SinceMS *int64
}

type Period string

const (
	PeriodToday     Period = "today"
	PeriodYesterday Period = "yesterday"
	PeriodLast7D    Period = "last7d"
)

type GroupItem struct {
	Key       string
	Count     int64
	TotalCost float64
	AvgLatMS  float64
}

type StatsSummary struct {
	Period       Period
	TotalCalls   int64
	OKCalls      int64
	ErrorCalls   int64
	TotalCost    float64
	AvgLatencyMS float64
	MaxLatencyMS int64
	TotalTokens  int64
	TopSlow      []Trace
	TopExpensive []Trace
	Groups       []GroupItem
}

func NewTraceID() string {
	now := time.Now().UTC().UnixNano()
	raw := sha256.Sum256([]byte(fmt.Sprintf("%d", now)))
	return hex.EncodeToString(raw[:])[:12]
}

func UnixMillis(t time.Time) int64 {
	return t.UTC().UnixMilli()
}

func BuildArtifact(traceID, prompt, response string, limit int) *Artifact {
	if limit <= 0 {
		limit = 320
	}
	return &Artifact{
		TraceID:         traceID,
		PromptPreview:   preview(prompt, limit),
		ResponsePreview: preview(response, limit),
		PromptSHA256:    sha(prompt),
		ResponseSHA256:  sha(response),
	}
}

func preview(s string, limit int) string {
	s = strings.TrimSpace(s)
	if len(s) <= limit {
		return s
	}
	return s[:limit] + "..."
}

func sha(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}

func PeriodRange(period Period, now time.Time) (time.Time, time.Time, error) {
	loc := now.Location()
	todayStart := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	switch period {
	case PeriodToday:
		return todayStart, now, nil
	case PeriodYesterday:
		start := todayStart.Add(-24 * time.Hour)
		return start, todayStart, nil
	case PeriodLast7D:
		return now.Add(-7 * 24 * time.Hour), now, nil
	default:
		return time.Time{}, time.Time{}, fmt.Errorf("unknown period: %s", period)
	}
}
