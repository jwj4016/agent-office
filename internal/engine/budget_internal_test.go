package engine

import (
	"strings"
	"testing"
	"time"

	"agent-office/internal/providers"
)

// A daily budget counts from midnight: a session that ran across it
// counts only what it used after, and earlier sessions not at all.
func TestDailyUsageCountsFromMidnight(t *testing.T) {
	defer func(f func() time.Time) { budgetNow = f }(budgetNow)
	budgetNow = func() time.Time { return time.Date(2026, 11, 3, 15, 0, 0, 0, time.Local) }
	since := Budget{Period: PeriodDay}.since()
	if want := time.Date(2026, 11, 3, 0, 0, 0, 0, time.Local); !since.Equal(want) {
		t.Fatalf("since = %v, want %v", since, want)
	}
	cost := func(v float64) *float64 { return &v }
	at := func(h int) time.Time { return since.Add(time.Duration(h) * time.Hour) }
	reports := []usageReport{
		{session: "yesterday|", usage: providers.UsagePayload{InputTokens: 900, CostUSD: cost(9)}, at: at(-5)},
		{session: "across|", usage: providers.UsagePayload{InputTokens: 100, OutputTokens: 10, CostUSD: cost(1)}, at: at(-1)},
		{session: "across|", usage: providers.UsagePayload{InputTokens: 300, OutputTokens: 30, CostUSD: cost(3)}, at: at(1)},
		{session: "today|", usage: providers.UsagePayload{InputTokens: 50, CostUSD: cost(0.5)}, at: at(2)},
		{session: "codex|", usage: providers.UsagePayload{InputTokens: 70}, at: at(3)},
	}
	var day Usage
	addRunUsage(&day, reports, since)
	if day.Attempts != 3 || day.InputTokens != 320 || day.OutputTokens != 20 || day.CostUSD != 2.5 || day.UnknownCostAttempts != 1 {
		t.Fatalf("today = %+v", day)
	}
	var all Usage
	addRunUsage(&all, reports, time.Time{})
	if all.Attempts != 4 || all.InputTokens != 1320 || all.CostUSD != 12.5 {
		t.Fatalf("all = %+v", all)
	}

	limit := 2.0
	day.Budget = Budget{MaxCostUSD: &limit, Period: PeriodDay}
	if h := budgetHold(day); h == "" || !strings.Contains(h, "오늘 비용 예산") || !strings.Contains(h, "내일 0시") {
		t.Fatalf("hold = %q", h)
	}
}
