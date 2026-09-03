// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package approvals

// An approval a human granted that the agent never came back for.
//
// ADR-0055: an agent-minted staging carries NO server-side executor. The agent
// redeems it by repeating the identical call with the approval token, and
// serverProposed (decide.go) is what keeps a compose executor from running over
// an agent-authored envelope — confusing the two once cost a human a 500, an
// approval that could never be redeemed again, and an audit row asserting a
// redemption for an effect that never ran.
//
// So when the agent does not come back — its session ended, its run was
// cancelled, it errored on the way — the row sits at `approved` until the
// redemption window closes, and then nothing happens at all.
//
// From the approver's side that is indistinguishable from success. They said yes
// in the web app, the row says `approved` with a `decided_at`, and the work they
// authorised was never done. A write that fails loudly is recoverable; one that
// reports success and does nothing is not, because nobody goes back to check.
// margince/margince#2535 is two re-associations approved in the web app where
// neither link was ever written.
//
// This executes NOTHING, which is the point: executing here is exactly what
// serverProposed refuses. It writes the same mark decide.go leaves on a
// server-side effect that failed, so the row reaches the surface that already
// exists for one — compose/attentionseam.go carries a reader their own failed
// decisions — instead of staying silent.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// lapsedRedemptionSentence is what the approver is shown. Written for them, in
// the "approved, but …" shape decide.go uses and for the same reason a failed
// effect's sentence has it: a human told only that something expired would
// reasonably decide again, and the row would refuse them as already decided. So
// it says what did not happen, and what to do instead.
const lapsedRedemptionSentence = "this was approved, but the assistant never came back to carry it out in time, " +
	"so nothing was changed — ask for it again if you still want it"

// lapseSweepBatch bounds one pass, for the reason expirySweepBatch does: an
// unbounded UPDATE over a backlog holds every row it touches until it commits,
// and the inbox reads the same table. A backlog drains across ticks instead —
// the predicate is a clock, and every row this pass did not reach is still
// matched by the next one.
//
// The bound is what makes the lock order below load-bearing rather than
// decorative: a statement that took the whole due set at once would still meet
// a bundle decision on the same rows, and the two would deadlock on any order
// they did not share.
const lapseSweepBatch = 200

// MarkLapsedRedemptions marks approved agent-minted stagings whose redemption
// window closed with nobody spending them, and answers how many it marked.
//
// ONE statement, unlike ExpireDue's row-at-a-time pass. The difference is what
// each writes: an expiry is a DECISION, with a status, a system-actor audit row
// and an event per row, and a batch that half-committed would leave some rows
// audited and others not. This writes bookkeeping — the same UPDATE
// recordEffectFailure makes, and nothing else — so there is no per-row
// transaction to keep honest.
//
// It runs under the expiry sweep's own actor and shares that guard rather than
// re-arguing it here — see onlyTheExpirySweep.
//
// The cutoff comes from the service clock so a test can move it, and it is
// redemptionTTL rather than a number of its own: the window whose closing this
// reports IS the one validateRedemption refuses after, and a second definition
// would drift into marking rows an agent could still spend.
//
// Three predicates carry the meaning, and each excludes a row this must not
// touch:
//
//   - passport_id IS NOT NULL — agent-minted only. A server-proposed staging
//     has an executor that ran inside the decision, and decide.go already marks
//     it when that fails; marking one here would overwrite a real diagnosis with
//     a guess about an agent that was never involved.
//   - consumed_at IS NULL — the agent DID come back. A redeemed approval did its
//     work, whatever happened downstream afterwards.
//   - effect_failed_at IS NULL — makes the pass idempotent, and keeps a row that
//     failed for a real reason from having that sentence replaced by this one.
//
// There is no `decided_at IS NOT NULL` beside the window comparison, and its
// absence is deliberate: NULL < anything is unknown rather than true, so the
// comparison already excludes a row nobody decided. Spelling it as well read
// as a second guard and was one nothing could fail without.
//
// A LOCKING READ under lockOrder, then a write to the ids it returned — rather
// than one UPDATE with the predicate inline. A bare UPDATE locks every row it
// matches in whatever order the planner chose, and a bundle decision locking
// the same rows under the canonical one would eventually deadlock: PostgreSQL
// resolves that by aborting one of them, which would be somebody's decision
// taking a 500 so that this pass could annotate a row it could have annotated
// on the next tick. Ordering by decided_at read perfectly naturally and was
// exactly that divergence. The write then acquires no new lock, because the
// read already holds every row it names.
func (s *Service) MarkLapsedRedemptions(ctx context.Context) (int64, error) {
	// The same guard ExpireDue takes, and for the same reason. This writes a
	// sentence a human reads on their own approvals, in bulk, and an open bulk
	// write would let any authenticated caller tell every approver in the
	// workspace that their decisions were never carried out — each one looking
	// exactly like a finding this sweep had made. It is the sweep's tick or
	// nobody.
	if err := onlyTheExpirySweep(ctx); err != nil {
		return 0, err
	}
	var marked int64
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT id FROM approval
			 WHERE status = $1
			   AND passport_id IS NOT NULL
			   AND consumed_at IS NULL
			   AND effect_failed_at IS NULL
			   AND decided_at < $2
			 `+lockOrder+`
			 LIMIT $3
			   FOR UPDATE`,
			approvalStatusApproved, s.now().UTC().Add(-redemptionTTL), lapseSweepBatch)
		if err != nil {
			return fmt.Errorf("crmapprovals: reading the approvals nobody came back for: %w", err)
		}
		lapsed, err := pgx.CollectRows(rows, pgx.RowTo[ids.ApprovalID])
		if err != nil {
			return fmt.Errorf("crmapprovals: reading the approvals nobody came back for: %w", err)
		}
		if len(lapsed) == 0 {
			return nil
		}
		tag, err := tx.Exec(ctx, `
			UPDATE approval
			   SET effect_failed_at = now(), effect_failure = $1
			 WHERE id = ANY($2)`, lapsedRedemptionSentence, lapsed)
		if err != nil {
			return fmt.Errorf("crmapprovals: marking the approvals nobody came back for: %w", err)
		}
		marked = tag.RowsAffected()
		return nil
	})
	return marked, err
}
