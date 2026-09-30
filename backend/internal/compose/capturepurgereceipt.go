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

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
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
func keptBreakdown(subject capture.PurgeSubject) KeptBreakdown {
	kept := KeptBreakdown{
		Held:                   len(subject.Held),
		UnderStatute:           len(subject.UnderStatute),
		UnderRequest:           len(subject.UnderRequest),
		UnderUndeterminedFloor: len(subject.UnderUndeterminedFloor),
	}
	if kept.UnderStatute > 0 {
		name, keep, fromYearEnd := privacy.StatutoryFloorClass()
		kept.StatutoryClass = name
		kept.StatutoryFromYearEnd = fromYearEnd
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

// auditPurgeReceipt records what the owner was told, from the same struct they
// were told it from.
//
// There is ONE set of numbers: an operator reading the trail and the colleague
// reading their screen are looking at the same count. A discrepancy between
// them would be worse than either alone, because it would make both
// unbelievable.
//
// Held by: TestThePurgeReceiptAndTheAuditRowCannotDisagree
// (backend/internal/compose/capturepurge_integration_test.go)
//
// The rule is the subject: a purge is an act performed against one exclusion
// rule, and the per-activity rows above already say what happened to each
// message. This is the summary those rows do not add up to on their own, since
// nothing writes a row for a message it deliberately did NOT touch.
func (p *CapturePurger) auditPurgeReceipt(ctx context.Context, ruleID ids.UUID, outcome PurgeOutcome) error {
	return database.WithWorkspaceTx(ctx, p.pool, func(tx pgx.Tx) error {
		_, err := storekit.AuditWithEvidence(ctx, tx, "archive", "capture_exclusion", ruleID, nil, nil,
			map[string]any{
				"destroyed": outcome.Destroyed, "released": outcome.Released,
				"skipped": outcome.Skipped, "anonymised": outcome.Anonymised,
				"kept_held": outcome.Kept.Held, "kept_under_statute": outcome.Kept.UnderStatute,
				"kept_under_request":            outcome.Kept.UnderRequest,
				"kept_under_undetermined_floor": outcome.Kept.UnderUndeterminedFloor,
			})
		return err
	})
}
