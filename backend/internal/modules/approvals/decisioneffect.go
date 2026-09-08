// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package approvals

// What a COMMITTED decision releases, and how a release that did not happen is
// recovered.
//
// Split from decide.go because it answers a different question. That file
// decides — the authority gate, the pending guard, the status write, the write
// shape, all inside one transaction. This one runs afterwards, outside it, and
// the whole difficulty here is that "afterwards" is where a failure can no
// longer be rolled back: what is left on the row is the only record that the
// work is still owed, and effectIsStillOwed is what reads it.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// effectIsStillOwed reports whether a decision this call could not make was
// already made, and left its work undone.
//
// THE TRAP THIS OPENS. runDecisionEffect runs after the decision commits, so an
// effect that fails leaves the row approved with consumed_at NULL — and the
// obvious retry, deciding again, is refused as already-decided. There is no
// other surface that re-drives an effect, so a transient failure at that moment
// (the store unreachable for a second, a deploy mid-flight) permanently strands
// the approval: the human said yes, the work never ran, and nothing they can do
// makes it run. runPrecheck narrows the window for the kinds that register one
// by refusing BEFORE the decision commits, but it is a preflight, and every
// failure it cannot foresee still lands here.
//
// WHY RE-DRIVING IS SAFE, and it is not "because effects are idempotent" —
// several are emphatically not. It is safe because every effect REDEEMS: it
// consumes the approval inside the same transaction as its write, so an effect
// that got as far as writing anything left consumed_at set, and this predicate
// refuses. An effect that failed left it NULL and wrote nothing. The single-use
// redemption is what makes "did this run" a fact about the row rather than a
// guess, and TestEveryRegisteredEffectRedeemsTheApprovalItRan
// (backend/gates/effectredemption_test.go) is what keeps it true of a kind
// registered later.
//
// The conditions, and what each one excludes:
//
//   - approve — a decline runs INSIDE the decision transaction, so a failed one
//     rolled the decision back and the row is still pending. There is nothing
//     decided to re-drive, and a reject arriving on an approved row is a real
//     conflict.
//   - the row is APPROVED — not rejected, not expired. Expiry in particular is
//     a verdict of its own, and re-driving one would apply what nobody released.
//   - consumed_at IS NULL — the effect never redeemed, which is the whole
//     signal.
//   - serverProposed — an agent-minted row reaches no executor at all, so there
//     is nothing owed. It is also what keeps a step-up out: a staged volume
//     release always carries the passport whose window it widens, and widening
//     is a counter increment with no redemption behind it, so re-driving one
//     would hand out the window twice.
//   - the kind has a registered effect — with none, no work was ever owed and
//     already-decided is the honest answer.
func (s *Service) effectIsStillOwed(a row, approve bool, err error) bool {
	var decided *AlreadyDecidedError
	if !errors.As(err, &decided) || !approve || decided.Status != approvalStatusApproved {
		return false
	}
	if a.ConsumedAt != nil || !serverProposed(a) {
		return false
	}
	_, registered := s.effects[a.Kind]
	return registered
}

// runDecisionEffect runs what a COMMITTED decision releases: a step-up's window
// widening, or the kind's registered follow-on executor.
//
// It is spelled once because two callers release decisions — one approval at a
// time here, a whole bundle at a time in bundle.go — and a second copy of this
// branch is how a bundle member would quietly stop executing what a human
// approved.
//
// The decision is already committed when this runs, so a failure never un-decides
// anything: the approval IS decided either way, and the approved-unredeemed row
// and its audit trail say exactly how far it got. That is also why the error says
// "approved, but …" — a human told only "redis is unreachable" would reasonably
// decide again, and the row would refuse them as already decided.
func (s *Service) runDecisionEffect(ctx context.Context, id ids.ApprovalID, a row, approve bool) error {
	// A step-up's effect is not a write into another module, so it does not run
	// through the effect table — which is closed to agent-minted stagings for
	// the reason serverProposed states, and a step-up is always agent-minted.
	// It widens the window the staging named, from that row's own passport
	// (quotarelease.go).
	if approve && a.Kind == KindVolumeRelease {
		if err := s.applyVolumeRelease(ctx, a); err != nil {
			return s.recordEffectFailure(ctx, id,
				"the agent's window could not be widened, so the approval has not taken effect",
				fmt.Errorf("approved, but widening the agent's window failed: %w", err))
		}
		return nil
	}
	if effect, ok := s.effects[a.Kind]; ok && approve && serverProposed(a) {
		if err := effect(ctx, id, a.ProposedChange, a.DiffHash); err != nil {
			return s.recordEffectFailure(ctx, id,
				"this was approved, but the work it released did not run",
				fmt.Errorf("approved, but executing the %s effect failed: %w", a.Kind, err))
		}
	}
	return nil
}

// recordEffectFailure marks an approved row whose effect did not run, and
// returns the caller's own error unchanged.
//
// Without the mark the row is unreachable: it is not pending, so the decision
// lane skips it, and it names a human decider, so the receipts lane does too. A
// person approved something, was told it was approved, and the work never
// happened — with the only trace an error on one request nobody may have read.
//
// The stored sentence is written HERE rather than from the executor's error,
// which carries whatever the failing module said and can name a table, a
// statement or a host. What reaches a reader says what happened and what it
// means for them.
//
// A failure to record the failure is logged and swallowed on purpose, and it is
// the one place in this file that swallows anything: the caller is already
// returning an error about the effect, and replacing it with a bookkeeping
// error would tell the human who approved the row the wrong thing about what
// went wrong.
func (s *Service) recordEffectFailure(ctx context.Context, id ids.ApprovalID, reader string, cause error) error {
	// Detached from the request's cancellation: an effect that failed BECAUSE
	// the request was cancelled or timed out is exactly a failure this mark
	// exists to keep, and writing it through the dead context would lose it.
	ctx = context.WithoutCancel(ctx)
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		// The IS NULL arm is the CAS: two failures racing on one row keep the
		// FIRST mark, because that is the one whose timestamp says when the
		// work was actually lost. Zero rows affected is that race resolved,
		// not an error.
		tag, err := tx.Exec(ctx,
			`UPDATE approval SET effect_failed_at = now(), effect_failure = $2
			  WHERE id = $1 AND effect_failed_at IS NULL`, id, reader)
		if err == nil && tag.RowsAffected() == 0 {
			s.logger().InfoContext(ctx, "approvals: effect failure already marked", "approval_id", id.String())
		}
		return err
	})
	if err != nil {
		s.logger().ErrorContext(ctx, "approvals: an approved effect failed and the row could not be marked",
			"approval_id", id.String(), "error", err)
	}
	return cause
}

// serverProposed reports whether this staging was minted by a SERVER-SIDE
// proposal flow rather than by an agent asserting a passport.
//
// The effect table is keyed by the kind string alone, and a kind is not a
// namespace: the REST admission gate stages under the operation's TOOL name,
// so an agent could mint a staging whose kind matched a kind some compose
// proposal flow had registered an executor for — "enrich" names both the
// scrape proposal and the tool behind three agent-reachable routes. A human
// approving that staging then invoked the compose executor over an
// agent-authored REST envelope, which consumed the approval in its own
// committed transaction and only then failed to parse: the human got a 500,
// the approval could never be redeemed again, and the audit row asserted a
// redemption for an effect that never ran.
//
// Provenance is the discriminator, because it is the thing that actually
// differs: a server-side proposal is staged by the system or by a human, and
// carries no passport. An agent-minted staging is redeemed the way ADR-0055
// says — by repeating the identical call with the approval token — and needs
// no server-side executor at all.
func serverProposed(a row) bool { return a.PassportID == nil }

// decideInTx runs the decision inside the caller's transaction: the
// decide-authority + row-scope gate, the pending guard, the optional
// modify-then-approve edit, the status write, and the write shape. It
// returns the re-read row so the follow-on effect runs against committed
// state.
