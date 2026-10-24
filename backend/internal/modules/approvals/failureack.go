// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package approvals

// The way off the Worklist for an approved decision whose work did not run.
//
// The failure is shown to the contact who decided it, and to nobody else. They
// can approve it once more, which re-drives the work (decisioneffect.go), or say
// they have seen it. Either one takes the row away; a later failure brings it back.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// stillFailing is the row half of "this failure is still news to its decider".
//
// A run that completes clears the mark (recordEffectRan), so a mark that stands
// is a failure nothing has fixed. The acknowledgement holds a copy of the
// failure it saw, so a newer mark differs from it.
const stillFailing = `effect_failed_at IS NOT NULL
	AND acknowledged_effect_failed_at IS DISTINCT FROM effect_failed_at`

// Absolute stage moves can satisfy an older identical request; relative advances
// and repeatable sends cannot. The later decision must belong to the same decider.
const noCompletedStageReplacement = `NOT (approval.kind = 'progress_deal'
 AND approval.target_entity_type = 'deal'
 AND EXISTS (SELECT 1 FROM approval completed
  WHERE completed.kind = approval.kind
   AND completed.target_entity_type = approval.target_entity_type
   AND completed.target_entity_id = approval.target_entity_id
   AND completed.co_target_entity_type IS NOT DISTINCT FROM approval.co_target_entity_type
   AND completed.co_target_entity_id IS NOT DISTINCT FROM approval.co_target_entity_id
   AND completed.decided_by = approval.decided_by
   AND completed.status = 'approved' AND completed.consumed_at IS NOT NULL
   AND completed.effect_failed_at IS NULL
   AND completed.created_at > approval.created_at
   AND completed.diff_hash = approval.diff_hash
   AND completed.proposed_change = approval.proposed_change))`

// FailedDecision is one of the caller's own approvals whose work did not run.
type FailedDecision struct {
	Approval crmcontracts.Approval
	// Retryable says approving once more would run the work again, for this
	// caller. The row still owes its effect, and they could still decide it.
	Retryable bool
}

// OwnFailedDecisions is the caller's failed decisions, newest first, each with
// whether a retry would be accepted.
//
// The list itself does not ask decidability (see List). Retry does, because the
// approve endpoint asks it again, and a verb that endpoint refuses is a dead button.
func (s *Service) OwnFailedDecisions(ctx context.Context, limit int) ([]FailedDecision, error) {
	rows, _, err := s.List(ctx, ListInput{FailedForDecider: true, Limit: limit})
	if err != nil {
		return nil, err
	}
	p, _ := principal.Actor(ctx)
	now := s.now()
	out := make([]FailedDecision, 0, len(rows))
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		for _, a := range rows {
			retryable := false
			if s.owesItsEffect(a) {
				if retryable, err = decidable(ctx, tx, p, a); err != nil {
					return err
				}
			}
			shown := []crmcontracts.Approval{wire(a, now)}
			withoutOwnersRecords(ctx, shown)
			out = append(out, FailedDecision{Approval: shown[0], Retryable: retryable})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// AcknowledgeEffectFailure takes a failed decision off its decider's Worklist.
//
// Only the decider may do it, and anyone else is answered 404. The failure is
// shown to nobody else, so its existence is not theirs to learn. Acknowledging
// the same failure twice changes nothing and records nothing.
//
// seen is the failure the reader was shown. A retry may have recorded a newer
// one since, and the reader has not seen that one, so it answers 409.
func (s *Service) AcknowledgeEffectFailure(ctx context.Context, id ids.ApprovalID, seen time.Time) error {
	if err := actingForAHuman(ctx); err != nil {
		return err
	}
	// The decider's own list, so an agent acting for them is refused too.
	p, _ := principal.Actor(ctx)
	if p.Type != principal.PrincipalHuman || p.UserID.IsZero() {
		return apperrors.ErrPermissionDenied
	}
	return s.db.Tx(ctx, func(tx pgx.Tx) error {
		var decidedBy *ids.UUID
		var failedAt, acknowledged *time.Time
		var kind string
		err := tx.QueryRow(ctx, `
			SELECT kind, decided_by, effect_failed_at, acknowledged_effect_failed_at
			  FROM approval WHERE id = $1 FOR UPDATE`, id).
			Scan(&kind, &decidedBy, &failedAt, &acknowledged)
		if errors.Is(err, pgx.ErrNoRows) {
			return apperrors.ErrNotFound
		}
		if err != nil {
			return err
		}
		if decidedBy == nil || *decidedBy != p.UserID || failedAt == nil {
			return apperrors.ErrNotFound
		}
		if !failedAt.Equal(seen) {
			return fmt.Errorf("this approval failed again after the failure you saw, so it was not acknowledged: %w",
				apperrors.ErrConflict)
		}
		if acknowledged != nil && acknowledged.Equal(*failedAt) {
			return nil
		}
		if _, err := tx.Exec(ctx,
			`UPDATE approval SET acknowledged_effect_failed_at = effect_failed_at WHERE id = $1`, id); err != nil {
			return err
		}
		auditID, err := storekit.Audit(ctx, tx, "update", entityApproval, id.UUID,
			map[string]any{"acknowledged_effect_failed_at": acknowledged},
			map[string]any{"acknowledged_effect_failed_at": failedAt.UTC()})
		if err != nil {
			return err
		}
		return storekit.EmitEvent(ctx, tx, auditID, id.UUID, crmcontracts.PublicEventApprovalFailureAcknowledged{
			Kind:     kind,
			FailedAt: failedAt.UTC(),
		})
	})
}
