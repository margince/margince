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
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/jurisdiction"
)

// KeptBreakdown is the skipped count, by the reason each message survived.
// The four are disjoint and sum to Skipped.
type KeptBreakdown struct {
	// Held is how many an erasure or a controller pinned by hand.
	Held int `json:"held"`
	// UnderStatute is how many sit inside their commercial-retention window.
	UnderStatute int `json:"under_statute"`
	// UnderRequest is how many a data-subject request is still about.
	UnderRequest int `json:"under_request"`
	// UnderUndeterminedFloor is how many were kept because this installation
	// could not say what the law requires of them. Apart from UnderStatute
	// because the shield is the same and the BASIS is not: reported together,
	// the receipt tells an owner a retention window applies when none was
	// measured.
	UnderUndeterminedFloor int `json:"under_undetermined_floor"`
	// StatutoryClass names the retention class that shielded them and for how
	// long — empty when nothing was shielded by it, so a reader is never shown
	// a rule that kept nothing.
	StatutoryClass string `json:"statutory_class,omitempty"`
	// StatutoryYears is how long that class keeps its records, in whole years,
	// and zero when the packs declare a period that is not whole years — a
	// screen that rounded one would misstate how long somebody's mail is held.
	// ISO 8601 deliberately does NOT travel: "P6Y" is a machine's spelling, and
	// this number is read by a colleague.
	StatutoryYears int `json:"statutory_years,omitempty"`
	// StatutoryFromYearEnd says the period counts from the end of the calendar
	// year rather than from the message's own date, which is the difference
	// between "six years" and "up to seven".
	StatutoryFromYearEnd bool `json:"statutory_from_year_end,omitempty"`
}

// keptBreakdown says why a purge left what it left.
//
// The statutory class is named only when it actually kept something. A reader
// shown "commercial correspondence, six years" beside a zero would reasonably
// read it as the rule that applied to their deletion, when in fact nothing of
// theirs was a Handelsbrief.
func keptBreakdown(subject capture.PurgeSubject, policy privacy.FloorPolicy) KeptBreakdown {
	kept := KeptBreakdown{
		Held:                   len(subject.Held),
		UnderStatute:           len(subject.UnderStatute),
		UnderRequest:           len(subject.UnderRequest),
		UnderUndeterminedFloor: len(subject.UnderUndeterminedFloor),
	}
	if kept.UnderStatute > 0 {
		class := policy.StrictestClass(time.Now())
		keep := class.Keep
		kept.StatutoryClass = string(class.Name)
		kept.StatutoryFromYearEnd = class.Anchor == jurisdiction.AnchorCalendarYearEnd
		// Only a whole-year period is reported. A class declared in months or
		// days has no number this copy can say truthfully, and rounding it
		// would tell somebody their mail is held for longer or shorter than it
		// is — so the sentence falls back to naming the class alone.
		if keep.Months == 0 && keep.Days == 0 {
			kept.StatutoryYears = keep.Years
		}
	}
	return kept
}

// auditPurgeReceipt records what the purge did, from the same struct the
// owner is told it from, and returns that struct with the counts the trail
// holds.
//
// There is ONE set of numbers: an operator reading the trail and the colleague
// reading their screen are looking at the same count. A discrepancy between
// them would be worse than either alone, because it would make both
// unbelievable.
//
// Destroyed, Released and Anonymised are counted from the per-message rows the
// cascade wrote, so a message the recheck skipped is not certified as gone.
// Skipped and Kept describe what the purge chose not to touch, which no row
// records, so they stay as selected.
//
// Held by: TestThePurgeReceiptAndTheAuditRowCannotDisagree
// (backend/internal/compose/capturepurge_integration_test.go)
//
// The rule is the subject: a purge is an act performed against one exclusion
// rule, and the per-message rows say what happened to each message. This is the
// summary those rows do not add up to on their own.
func (p *CapturePurger) auditPurgeReceipt(ctx context.Context, ruleID ids.UUID, outcome PurgeOutcome) (PurgeOutcome, error) {
	err := database.WithWorkspaceTx(ctx, p.pool, func(tx pgx.Tx) error {
		tally, err := lockRuleAndTallyTx(ctx, tx, ruleID)
		if err != nil {
			return err
		}
		outcome.Destroyed, outcome.Released, outcome.Anonymised = tally.destroyed, tally.released, tally.anonymised
		_, err = storekit.AuditWithEvidence(ctx, tx, "archive", "capture_exclusion", ruleID, nil, nil,
			map[string]any{
				"destroyed": outcome.Destroyed, "released": outcome.Released,
				"skipped": outcome.Skipped, "anonymised": outcome.Anonymised,
				"kept_held": outcome.Kept.Held, "kept_under_statute": outcome.Kept.UnderStatute,
				"kept_under_request":            outcome.Kept.UnderRequest,
				"kept_under_undetermined_floor": outcome.Kept.UnderUndeterminedFloor,
			})
		return err
	})
	return outcome, err
}

// purgeTally is what the per-message rows of one purge say was done.
type purgeTally struct{ destroyed, released, anonymised int }

// purgeTrailRows selects the rows the cascade writes for one rule (@rule, its
// batch_id) that no receipt covers yet: an erase per destroyed message, an
// archive per released one and an anonymize per stripped contact. A receipt
// covers every row older than itself, and the row lock on the rule keeps a
// second receipt from being written between the count and the write.
const purgeTrailRows = `a.batch_id = @rule AND ` + purgeRowKinds

const purgeRowKinds = `((a.action = 'erase' AND a.entity_type = 'activity')
	  OR (a.action = 'archive' AND a.entity_type = 'activity' AND a.after ? 'released_by')
	  OR (a.action = 'anonymize' AND a.entity_type = 'contact'))
	AND a.occurred_at > ` + lastReceiptAt

// lastReceiptAt is when the rule's latest receipt was written. The evidence
// key tells a receipt from the rule's other archive rows.
const lastReceiptAt = `coalesce((SELECT max(r.occurred_at) FROM audit_log r
	WHERE r.entity_type = 'capture_exclusion' AND r.entity_id = a.batch_id
	  AND r.action = 'archive' AND r.evidence ? 'destroyed'), '-infinity')`

// lockRuleAndTallyTx counts the uncovered rows under a lock on the rule.
// The lock is what makes the live receipt and the sweep exclusive: whichever
// takes it second finds the first one's receipt and counts nothing.
func lockRuleAndTallyTx(ctx context.Context, tx pgx.Tx, ruleID ids.UUID) (purgeTally, error) {
	var locked ids.UUID
	if err := tx.QueryRow(ctx, `SELECT id FROM capture_exclusion WHERE id = @rule FOR UPDATE`,
		pgx.StrictNamedArgs{"rule": ruleID}).Scan(&locked); err != nil {
		return purgeTally{}, fmt.Errorf("capture purge: locking the rule for its receipt: %w", err)
	}
	var t purgeTally
	err := tx.QueryRow(ctx, `
		SELECT count(DISTINCT a.entity_id) FILTER (WHERE a.action = 'erase'),
		       count(DISTINCT a.entity_id) FILTER (WHERE a.action = 'archive'),
		       count(DISTINCT a.entity_id) FILTER (WHERE a.action = 'anonymize')
		  FROM audit_log a WHERE `+purgeTrailRows,
		pgx.StrictNamedArgs{"rule": ruleID}).Scan(&t.destroyed, &t.released, &t.anonymised)
	if err != nil {
		return purgeTally{}, fmt.Errorf("capture purge: counting what the purge did: %w", err)
	}
	return t, nil
}

// receiptlessPurges lists the rules with purge rows no receipt covers, the
// newest of which is older than the grace period. A younger row may belong to
// a purge still running, whose own receipt is about to be written.
func receiptlessPurges(ctx context.Context, tx pgx.Tx, grace time.Duration, limit int) ([]ids.UUID, error) {
	rows, err := tx.Query(ctx, `
		SELECT a.batch_id FROM audit_log a
		  JOIN capture_exclusion e ON e.id = a.batch_id
		 WHERE `+purgeRowKinds+`
		 GROUP BY a.batch_id
		HAVING max(a.occurred_at) < now() - make_interval(secs => @grace_secs)
		 LIMIT @limit`,
		pgx.StrictNamedArgs{"grace_secs": grace.Seconds(), "limit": limit})
	if err != nil {
		return nil, fmt.Errorf("capture purge: finding purges without a receipt: %w", err)
	}
	return pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
}

// SweepMissingReceipts writes the receipt of every purge whose per-message rows
// exist and whose receipt does not, because the process stopped between the
// cascade and the receipt. It counts the rows, so the receipt states what was
// deleted. What the purge chose not to touch left no row, so a recovered
// receipt carries no kept counts and says so with `recovered`.
//
// Running it again writes nothing: the first run's receipt covers the rows.
func (p *CapturePurger) SweepMissingReceipts(ctx context.Context, grace time.Duration) (int, error) {
	ctx = principal.SystemActing(ctx, receiptSweepActor)
	var rules []ids.UUID
	if err := database.WithWorkspaceTx(ctx, p.pool, func(tx pgx.Tx) (err error) {
		rules, err = receiptlessPurges(ctx, tx, grace, personalSweepBatch)
		return err
	}); err != nil {
		return 0, err
	}
	written := 0
	for _, rule := range rules {
		wrote, err := p.writeRecoveredReceipt(ctx, rule)
		if err != nil {
			return written, err
		}
		if wrote {
			written++
		}
	}
	return written, nil
}

// purgeReceiptGrace is how long a purge's last row must have stood before the
// sweep treats the purge as stopped. A running purge commits per message and
// writes its receipt last, well inside this.
const purgeReceiptGrace = time.Hour

// receiptSweepActor is the system principal a recovered receipt is written as.
const receiptSweepActor = "system:capture-purge-receipt"

func (p *CapturePurger) writeRecoveredReceipt(ctx context.Context, ruleID ids.UUID) (bool, error) {
	wrote := false
	err := database.WithWorkspaceTx(ctx, p.pool, func(tx pgx.Tx) error {
		tally, err := lockRuleAndTallyTx(ctx, tx, ruleID)
		if err != nil || tally == (purgeTally{}) {
			return err
		}
		wrote = true
		_, err = storekit.AuditWithEvidence(ctx, tx, "archive", "capture_exclusion", ruleID, nil, nil,
			map[string]any{
				"destroyed": tally.destroyed, "released": tally.released,
				"anonymised": tally.anonymised, "recovered": true,
			})
		return err
	})
	return wrote, err
}
