// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package automation

// The rule-health read: the automations whose recent firings failed or were
// blocked, newest first. It answers across every live automation — the run
// history endpoint pages ONE instance, and a person who has to open every
// rule to learn one broke is the silence this read ends. Engine workflows
// that are not automations never join (the run row names no automation), and
// the lane this feeds claims only automations for exactly that reason.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// TroubledAutomationRun is one firing that did not do its work: which rule,
// which way it stopped (the contract's failed/blocked vocabulary), and the
// engine's own recorded reason where it left one.
type TroubledAutomationRun struct {
	ID ids.UUID
	// AutomationID is the RULE that failed, as distinct from this one firing
	// of it. Two failures of one rule share it; a rename does not change it,
	// and two rules that happen to share a name do not collide on it.
	AutomationID ids.AutomationID
	Name         string
	Outcome      string
	Reason       *string
	CreatedAt    time.Time
}

// troubledRunsSQL joins each failed or blocked run back to its LIVE, ENABLED
// automation the way ListRuns addresses runs forward: the handler is the
// automation's key and the idempotency key carries "@<automation id>" (a
// UUID, so the LIKE pattern carries no metacharacters). A paused rule's
// failures raise nothing — its owner turned it off, often because of exactly
// those failures, and a card would nag them about their own decision — and
// an archived rule's history stays history. The two spellings stay separate
// because the fragment binds differently — ListRuns parameterizes one
// instance's id, this join correlates the automation column — and runKey's
// own doc (engine_run.go) names both readers so a key-shape change lands on
// everyone who decodes it. An archived automation's history stays
// history — a card for a rule nobody can open would be a dead end.
const troubledRunsSQL = `
SELECT r.id, a.id, a.name, r.status, r.detail, r.created_at
  FROM workflow_run r
  JOIN automation a ON a.archived_at IS NULL AND a.enabled
   AND r.handler = a.key
   AND r.idempotency_key LIKE '%@' || a.id
 WHERE r.status IN ('failed', 'blocked')
   AND r.created_at >= $1
 ORDER BY r.created_at DESC, r.id DESC
 LIMIT $2`

// TroubledRuns answers the recent failed and blocked firings across every
// live automation, newest first, bounded. Gated exactly as the per-instance
// history is (`automation` read), so the lane it feeds is withheld for
// everyone the automation screens refuse.
func (s *AutomationStore) TroubledRuns(ctx context.Context, since time.Time, limit int) ([]TroubledAutomationRun, error) {
	if err := auth.Require(ctx, "automation", principal.ActionRead); err != nil {
		return nil, err
	}
	var troubled []TroubledAutomationRun
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		rows, txErr := tx.Query(ctx, troubledRunsSQL, since, limit)
		if txErr != nil {
			return txErr
		}
		defer rows.Close()
		troubled = []TroubledAutomationRun{}
		for rows.Next() {
			var run TroubledAutomationRun
			var status string
			var rawDetail []byte
			if scanErr := rows.Scan(&run.ID, &run.AutomationID, &run.Name,
				&status, &rawDetail, &run.CreatedAt); scanErr != nil {
				return scanErr
			}
			run.Outcome = runOutcomeByStatus[status]
			var detailErr error
			if run.Reason, detailErr = decodeRunDetail(rawDetail); detailErr != nil {
				return detailErr
			}
			troubled = append(troubled, run)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("automation: listing troubled runs: %w", err)
	}
	return troubled, nil
}
