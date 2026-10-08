package engine

import (
	"context"
	"encoding/json"
	"fmt"

	"agent-office/internal/providers"
)

// Budget is a project's spending limit (spec §10.3). Limits are checked
// before each new AI step; work already running is not interrupted, and
// provider billing may still lag, so this is not an absolute cap.
type Budget struct {
	MaxTokens  *int64   `json:"maxTokens,omitempty"`
	MaxCostUSD *float64 `json:"maxCostUsd,omitempty"`
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
}

func (u Usage) tokens() int64 { return u.InputTokens + u.OutputTokens }

func projectUsage(ctx context.Context, q querier, projectID string) (Usage, error) {
	var u Usage
	var budget string
	if err := q.QueryRowContext(ctx, `SELECT budget FROM projects WHERE id = ?`, projectID).Scan(&budget); err != nil {
		return u, err
	}
	json.Unmarshal([]byte(budget), &u.Budget)
	// Providers report running totals per session; keep each session's
	// latest report. An attempt's side sessions (meeting turns, answers
	// to other AIs) carry a phase to tell them apart.
	rows, err := q.QueryContext(ctx, `SELECT step_attempt_id, payload FROM execution_events
		WHERE project_id = ? AND kind = 'provider.usage' AND step_attempt_id IS NOT NULL ORDER BY sequence`, projectID)
	if err != nil {
		return u, err
	}
	defer rows.Close()
	latest := map[string]providers.UsagePayload{}
	for rows.Next() {
		var id, payload string
		rows.Scan(&id, &payload)
		var p struct {
			providers.UsagePayload
			Phase string `json:"phase"`
		}
		json.Unmarshal([]byte(payload), &p)
		latest[id+"|"+p.Phase] = p.UsagePayload
	}
	for _, p := range latest {
		u.Attempts++
		u.InputTokens += p.InputTokens
		u.OutputTokens += p.OutputTokens
		if p.CostUSD == nil {
			u.UnknownCostAttempts++
		} else {
			u.CostUSD += *p.CostUSD
		}
	}
	u.HoldReason = budgetHold(u)
	return u, rows.Err()
}

func budgetHold(u Usage) string {
	b := u.Budget
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
