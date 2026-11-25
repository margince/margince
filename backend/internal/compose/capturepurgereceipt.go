// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The receipt a purge hands back, and the row that has to agree with it.
//
// The information sheet promises a colleague that captured mail can be deleted
// irrevocably. The cascade behind that promise is thorough; what the colleague it
// was made to got back was nothing, so the one claim about THEIR data was the
// one they had to take on trust. This is the answer, and the half that matters
// most is what was KEPT: a deletion that correctly leaves a Handelsbrief
// standing looks, from the owner's side, exactly like one that silently
// failed.

import (
	"context"

	"github.com/margince/margince/backend/internal/compose/purgerun"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The receipt keys the audit row shares with the purge answer's JSON fields.
const (
	receiptReleased = "released"
	receiptSkipped  = "skipped"
)

// auditPurgeReceipt records what the purge did and returns the struct the owner
// is told it from. The trail and the screen state one set of numbers.
//
// Destroyed, Released and Anonymised are counted from this run's per-message
// rows, so a message the recheck skipped is not certified as gone. Skipped and
// Kept name what the purge left alone, which no row records, so they stay as
// selected.
//
// Held by: TestThePurgeReceiptAndTheAuditRowCannotDisagree
// (backend/internal/compose/capturepurge_integration_test.go)
//
// The rule is the subject: a purge is an act performed against one exclusion
// rule, and the per-message rows say what happened to each message. This is the
// summary those rows do not add up to on their own.
func (p *CapturePurger) auditPurgeReceipt(ctx context.Context, run, ruleID ids.UUID, outcome PurgeOutcome) (PurgeOutcome, error) {
	_, err := purgerun.WriteReceipt(ctx, p.pool, purgerun.Rule(ruleID), run, func(t purgerun.Tally) map[string]any {
		outcome.Destroyed, outcome.Released, outcome.Anonymised = t.Destroyed, t.Released, t.Anonymised
		return map[string]any{
			"destroyed": outcome.Destroyed, receiptReleased: outcome.Released,
			receiptSkipped: outcome.Skipped, "anonymised": outcome.Anonymised,
			"kept_held": outcome.Kept.Held, "kept_under_statute": outcome.Kept.UnderStatute,
			"kept_under_request":            outcome.Kept.UnderRequest,
			"kept_under_undetermined_floor": outcome.Kept.UnderUndeterminedFloor,
		}
	})
	return outcome, err
}
