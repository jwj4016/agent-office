package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"agent-office/internal/domain"
)

// JobCollabDecision is the lead AI deciding a reference collaboration
// request for its department in auto mode (plan §4.5).
const JobCollabDecision = "collab_decision"

// AI collaboration decisions.
const (
	CollabDecideAccept   = "accept"
	CollabDecideDecline  = "decline"
	CollabDecideQuestion = "question"
)

// CollabDecisionResult is what the lead AI decided about a request.
type CollabDecisionResult struct {
	Decision     string `json:"decision"`
	Reason       string `json:"reason"`
	Question     string `json:"question"`
	Title        string `json:"title"`
	DoneCriteria string `json:"doneCriteria"`
}

// CollabForDelivery is the request a collaboration delivery is about.
func (db *DB) CollabForDelivery(ctx context.Context, projectID, deliveryID string) (CollabRequest, error) {
	var id string
	if err := db.sql.QueryRowContext(ctx, `SELECT r.id FROM collab_requests r JOIN event_deliveries d
		ON d.event_sequence = r.event_sequence AND d.department_id = r.to_department_id WHERE d.id = ? AND d.project_id = ?`,
		deliveryID, projectID).Scan(&id); err != nil {
		return CollabRequest{}, fmt.Errorf("collaboration of delivery %q: %w", deliveryID, ErrNotInProject)
	}
	return collabTx(ctx, db.sql, projectID, id)
}

// CompleteCollabDecision stores a decision job's result in one
// transaction: the job becomes ready, the request is accepted (its work
// registered and counted in the event's chain), declined or questioned,
// and the delivery finishes or waits for the answer. A request a person
// decided meanwhile, or one that became required, is left as it is.
// Accepting past the chain limits holds the delivery instead.
func (db *DB) CompleteCollabDecision(ctx context.Context, job AIJob, res CollabDecisionResult) error {
	raw, _ := json.Marshal(res)
	_, err := db.Change(ctx, func(c *Change) error {
		tx := c.Tx
		cur, err := aiJobTx(ctx, tx, job.ID)
		if err != nil {
			return err
		}
		if cur.Status != JobWorking {
			return nil // finished meanwhile; the stored result stands
		}
		now := Now()
		if _, err := tx.ExecContext(ctx, `UPDATE work_ai_jobs SET status = 'ready', result = ?, updated_at = ? WHERE id = ?`, string(raw), now, job.ID); err != nil {
			return err
		}
		if err := emitJobUpdate(c, job.ProjectID, job.ID, JobReady); err != nil {
			return err
		}
		finish := func(status, note, hold string) error {
			_, err := tx.ExecContext(ctx, `UPDATE event_deliveries SET status = ?, note = ?, error = '', hold_reason = ?, updated_at = ? WHERE id = ?`,
				status, note, hold, now, job.DeliveryID)
			if err != nil {
				return err
			}
			return c.Emit(job.ProjectID, "", "", "delivery.updated", map[string]string{"deliveryId": job.DeliveryID, "status": status})
		}
		var reqID string
		if err := tx.QueryRowContext(ctx, `SELECT r.id FROM collab_requests r JOIN event_deliveries d
			ON d.event_sequence = r.event_sequence AND d.department_id = r.to_department_id WHERE d.id = ?`, job.DeliveryID).Scan(&reqID); err != nil {
			return err
		}
		r, err := collabTx(ctx, tx, job.ProjectID, reqID)
		if err != nil {
			return err
		}
		if r.Status != CollabRequested || r.Requirement != CollabReference {
			return finish(domain.DeliveryDone, "AI가 결정하기 전에 사람이 결정했거나 필수 협업이 되어 사람이 결정합니다", "")
		}
		who := collabDecider{JobID: job.ID}
		switch res.Decision {
		case CollabDecideAccept:
			if hold, err := chainHoldTx(ctx, tx, r.EventSequence); err != nil {
				return err
			} else if hold != "" {
				return finish(domain.DeliveryHeld, "", hold)
			}
			if err := acceptCollabTx(ctx, c, r, CollabAcceptance{Title: res.Title, DoneCriteria: res.DoneCriteria, Note: res.Reason}, who, now); err != nil {
				return err
			}
			if err := finish(domain.DeliveryDone, "AI가 수락했습니다: "+res.Reason, ""); err != nil {
				return err
			}
		case CollabDecideDecline:
			if _, err := tx.ExecContext(ctx, `UPDATE collab_requests SET status = 'declined', follow_up = '', decided_by = 'ai', decision_job_id = ?,
				decision_reason = ?, decided_at = ?, updated_at = ? WHERE id = ?`, job.ID, res.Reason, now, now, r.ID); err != nil {
				return err
			}
			if err := addCollabHistoryByTx(ctx, tx, r.ProjectID, r.ID, "declined", res.Reason, r.ToDepartmentID, who.author(), now); err != nil {
				return err
			}
			if err := finish(domain.DeliveryDone, "AI가 거절했습니다: "+res.Reason, ""); err != nil {
				return err
			}
		case CollabDecideQuestion:
			if _, err := tx.ExecContext(ctx, `UPDATE collab_requests SET status = 'question', follow_up = ?, decided_by = 'ai', decision_job_id = ?,
				updated_at = ? WHERE id = ?`, res.Question, job.ID, now, r.ID); err != nil {
				return err
			}
			if err := addCollabHistoryByTx(ctx, tx, r.ProjectID, r.ID, "asked", res.Question, r.ToDepartmentID, who.author(), now); err != nil {
				return err
			}
			status, note := domain.DeliveryWaitingAnswer, "AI 질문: "+res.Question
			if job.Round == 2 {
				status, note = domain.DeliveryDone, "AI가 다시 질문해 답한 뒤에는 사람이 결정합니다: "+res.Question
			}
			if err := finish(status, note, ""); err != nil {
				return err
			}
		default:
			return invalid("알 수 없는 협업 결정입니다: %s", res.Decision)
		}
		return c.Emit(job.ProjectID, "", "", "collab.updated", map[string]string{"collabRequestId": r.ID})
	})
	return err
}

// ReviewCollabDecision records that the person looked at the lead AI's
// decision and keeps it.
func (db *DB) ReviewCollabDecision(ctx context.Context, projectID, id string) (CollabRequest, error) {
	return db.changeCollab(ctx, projectID, id, func(c *Change, r CollabRequest, now string) error {
		if r.DecidedBy != "ai" || r.ReviewedAt != "" {
			return invalid("확인할 AI 결정이 없습니다")
		}
		_, err := c.Tx.ExecContext(ctx, `UPDATE collab_requests SET reviewed_at = ?, updated_at = ? WHERE id = ?`, now, now, r.ID)
		return err
	})
}

// RevertCollabDecision undoes the lead AI's decision with the person's
// reason: work the AI accepted is cancelled (not while it runs), and the
// request waits for the person to decide. The decision stays in the
// history.
func (db *DB) RevertCollabDecision(ctx context.Context, projectID, id, reason string) (CollabRequest, error) {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return CollabRequest{}, invalid("되돌리는 이유가 필요합니다")
	}
	return db.changeCollab(ctx, projectID, id, func(c *Change, r CollabRequest, now string) error {
		tx := c.Tx
		if r.DecidedBy != "ai" || (r.Status != CollabAccepted && r.Status != CollabDeclined && r.Status != CollabQuestion) {
			return invalid("AI가 결정한 협업만 되돌릴 수 있습니다")
		}
		if r.Status == CollabAccepted && r.ResultWorkItemID != "" {
			var active int
			tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM runs WHERE work_item_id = ? AND status IN ('running', 'waiting', 'paused')`, r.ResultWorkItemID).Scan(&active)
			if active > 0 {
				return invalid("수행 업무가 실행 중입니다. 실행을 취소한 뒤 되돌리세요")
			}
			w, err := workTx(ctx, tx, projectID, r.ResultWorkItemID)
			if err != nil {
				return err
			}
			if w.Status != domain.WorkCancelled {
				if err := setWorkStatusTx(ctx, c, w, domain.WorkCancelled, "AI의 협업 수락을 되돌렸습니다: "+reason, domain.LocalOwner); err != nil {
					return err
				}
			}
			if _, err := tx.ExecContext(ctx, `UPDATE auto_starts SET status = 'cancelled', reason = '', note = ?, updated_at = ?
				WHERE work_item_id = ? AND status IN ('pending', 'planning', 'approved', 'review')`, "AI의 협업 수락을 되돌렸습니다", now, w.ID); err != nil {
				return err
			}
		}
		if r.Status == CollabQuestion {
			if _, err := tx.ExecContext(ctx, `UPDATE event_deliveries SET status = 'done', note = ?, updated_at = ?
				WHERE event_sequence = ? AND department_id = ? AND purpose = 'collab_request' AND status = 'waiting_answer'`,
				"사람이 AI의 질문을 되돌렸습니다", now, r.EventSequence, r.ToDepartmentID); err != nil {
				return err
			}
		}
		if _, err := tx.ExecContext(ctx, `UPDATE collab_requests SET status = 'requested', follow_up = '', decided_by = '', decision_reason = '',
			decided_at = NULL, result_work_item_id = NULL, reverted_at = ?, revert_reason = ?, reviewed_at = ?, updated_at = ? WHERE id = ?`,
			now, reason, now, now, r.ID); err != nil {
			return err
		}
		return addCollabHistoryTx(ctx, tx, projectID, r.ID, "reverted", reason, "", now)
	})
}
