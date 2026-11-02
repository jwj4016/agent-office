package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"agent-office/internal/providers"
	"agent-office/internal/storage"
)

// Budget is a project's spending limit (spec §10.3). Limits are checked
// before each new AI step; work already running is not interrupted, and
// provider billing may still lag, so this is not an absolute cap.
//
// Period "day" makes the limits daily: usage is counted from midnight on
// this computer's clock, and held work starts again the next day. Empty
// counts everything the service has used.
type Budget struct {
	MaxTokens  *int64   `json:"maxTokens,omitempty"`
	MaxCostUSD *float64 `json:"maxCostUsd,omitempty"`
	Period     string   `json:"period,omitempty"`
}

// PeriodDay is the daily budget period.
const PeriodDay = "day"

// budgetNow is the clock daily budgets count from (tests set it).
var budgetNow = time.Now

// since is when the budget's counting starts; zero counts everything.
func (b Budget) since() time.Time {
	if b.Period != PeriodDay {
		return time.Time{}
	}
	y, m, d := budgetNow().Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.Local)
}

// Usage sums the latest usage each AI attempt reported in a project.
type Usage struct {
	InputTokens  int64 `json:"inputTokens"`
	OutputTokens int64 `json:"outputTokens"`
	// CostUSD is the sum of reported costs only.
	CostUSD float64 `json:"costUsd"`
	// UnknownCostAttempts counts attempts whose cost was not reported;
	// their spend is not in CostUSD and is never shown as 0.
	UnknownCostAttempts int    `json:"unknownCostAttempts"`
	Attempts            int    `json:"attempts"`
	Budget              Budget `json:"budget"`
	// HoldReason is set when new AI work is held by the budget.
	HoldReason string `json:"holdReason,omitempty"`
	// Since is when a daily budget's counting started; empty when the
	// usage is everything the service has used.
	Since string `json:"since,omitempty"`
}

func (u Usage) tokens() int64 { return u.InputTokens + u.OutputTokens }

func projectUsage(ctx context.Context, q querier, projectID string) (Usage, error) {
	var u Usage
	var budget string
	if err := q.QueryRowContext(ctx, `SELECT budget FROM projects WHERE id = ?`, projectID).Scan(&budget); err != nil {
		return u, err
	}
	json.Unmarshal([]byte(budget), &u.Budget)
	since := u.Budget.since()
	if !since.IsZero() {
		u.Since = since.UTC().Format(time.RFC3339)
	}
	rows, err := q.QueryContext(ctx, `SELECT step_attempt_id, payload, created_at FROM execution_events
		WHERE project_id = ? AND kind = 'provider.usage' AND step_attempt_id IS NOT NULL ORDER BY sequence`, projectID)
	if err != nil {
		return u, err
	}
	var reports []usageReport
	for rows.Next() {
		var r usageReport
		var payload, at string
		if err := rows.Scan(&r.attempt, &payload, &at); err != nil {
			rows.Close()
			return u, err
		}
		var p struct {
			providers.UsagePayload
			Phase string `json:"phase"`
		}
		json.Unmarshal([]byte(payload), &p)
		r.session, r.usage = r.attempt+"|"+p.Phase, p.UsagePayload
		r.at, _ = time.Parse(time.RFC3339Nano, at)
		reports = append(reports, r)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return u, err
	}
	addRunUsage(&u, reports, since)
	// AI work outside runs (work plans, department judgments) is paid
	// from the same budget, failed and retried calls included.
	ai, err := storage.SumAIAttemptUsage(ctx, q, projectID, since)
	if err != nil {
		return u, err
	}
	u.Attempts += ai.Attempts
	u.InputTokens += ai.InputTokens
	u.OutputTokens += ai.OutputTokens
	u.UnknownCostAttempts += ai.UnknownCost
	u.CostUSD += ai.CostUSD
	u.HoldReason = budgetHold(u)
	return u, nil
}

// usageReport is one usage report of a run's AI session.
type usageReport struct {
	attempt, session string
	usage            providers.UsagePayload
	at               time.Time
}

// addRunUsage adds the usage of runs' AI sessions. Providers report
// running totals per session, so each session's latest report counts.
// An attempt's side sessions (meeting turns, answers to other AIs) carry
// a phase to tell them apart. From a non-zero since, a session counts
// what it used after then: its latest total less its last total before.
func addRunUsage(u *Usage, reports []usageReport, since time.Time) {
	latest, before := map[string]providers.UsagePayload{}, map[string]providers.UsagePayload{}
	var order []string
	for _, r := range reports {
		if r.at.Before(since) {
			before[r.session] = r.usage
			continue
		}
		if _, ok := latest[r.session]; !ok {
			order = append(order, r.session)
		}
		latest[r.session] = r.usage
	}
	for _, s := range order {
		p, b := latest[s], before[s]
		u.Attempts++
		u.InputTokens += max(0, p.InputTokens-b.InputTokens)
		u.OutputTokens += max(0, p.OutputTokens-b.OutputTokens)
		if p.CostUSD == nil {
			u.UnknownCostAttempts++
			continue
		}
		cost := *p.CostUSD
		if b.CostUSD != nil {
			cost = max(0, cost-*b.CostUSD)
		}
		u.CostUSD += cost
	}
}

func budgetHold(u Usage) string {
	b := u.Budget
	if b.Period == PeriodDay {
		if b.MaxTokens != nil && u.tokens() >= *b.MaxTokens {
			return fmt.Sprintf("오늘 토큰 예산 %d개 중 %d개를 사용해 새 AI 업무를 보류했습니다. 내일 0시에 다시 시작합니다(서비스 설정에서 예산을 조정할 수 있습니다)", *b.MaxTokens, u.tokens())
		}
		if b.MaxCostUSD != nil && u.CostUSD >= *b.MaxCostUSD {
			return fmt.Sprintf("오늘 비용 예산 $%.2f 중 $%.4f(보고된 비용)를 사용해 새 AI 업무를 보류했습니다. 내일 0시에 다시 시작합니다(서비스 설정에서 예산을 조정할 수 있습니다)", *b.MaxCostUSD, u.CostUSD)
		}
		return ""
	}
	if b.MaxTokens != nil && u.tokens() >= *b.MaxTokens {
		return fmt.Sprintf("토큰 예산 %d개 중 %d개를 사용해 새 AI 업무를 보류했습니다. 서비스 개요에서 예산을 조정하세요", *b.MaxTokens, u.tokens())
	}
	if b.MaxCostUSD != nil && u.CostUSD >= *b.MaxCostUSD {
		return fmt.Sprintf("비용 예산 $%.2f 중 $%.4f(보고된 비용)를 사용해 새 AI 업무를 보류했습니다. 서비스 개요에서 예산을 조정하세요", *b.MaxCostUSD, u.CostUSD)
	}
	return ""
}

// ProjectUsage reports a project's AI usage and budget state.
func (e *Engine) ProjectUsage(ctx context.Context, projectID string) (Usage, error) {
	if err := e.db.CheckScope(ctx, projectID); err != nil {
		return Usage{}, err
	}
	return projectUsage(ctx, e.db.Read(), projectID)
}
