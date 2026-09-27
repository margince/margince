// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The receipt a purge hands back, and the row that has to agree with it.
//
// The information sheet promises a colleague that captured mail can be deleted
// irrevocably. The cascade behind that promise is thorough; what the person it
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
// The three are disjoint and sum to Skipped.
type KeptBreakdown struct {
	// Held is how many an erasure or a controller pinned by hand.
	Held int `json:"held"`
	// UnderStatute is how many sit inside their commercial-retention window.
	UnderStatute int `json:"under_statute"`
	// UnderRequest is how many a data-subject request is still about.
	UnderRequest int `json:"under_request"`
	// StatutoryClass names the retention class that shielded them and for how
	// long — empty when nothing was shielded by it, so a reader is never shown
	// a rule that kept nothing.
	StatutoryClass string `json:"statutory_class,omitempty"`
	// StatutoryPeriod is that class's period in ISO 8601 (P6Y), and
	// StatutoryFromYearEnd says it counts from the end of the calendar year
	// rather than from the message's own date — which is the difference
	// between "six years" and "up to seven".
	StatutoryPeriod      string `json:"statutory_period,omitempty"`
	StatutoryFromYearEnd bool   `json:"statutory_from_year_end,omitempty"`
}

// keptBreakdown says why a purge left what it left.
//
// The statutory class is named only when it actually kept something. A reader
// shown "commercial correspondence, six years" beside a zero would reasonably
// read it as the rule that applied to their deletion, when in fact nothing of
// theirs was a Handelsbrief.
func keptBreakdown(subject capture.PurgeSubject) KeptBreakdown {
	kept := KeptBreakdown{
		Held:         len(subject.Held),
		UnderStatute: len(subject.UnderStatute),
		UnderRequest: len(subject.UnderRequest),
	}
	if kept.UnderStatute > 0 {
		name, keep, fromYearEnd := privacy.StatutoryFloorClass()
		kept.StatutoryClass = name
		kept.StatutoryPeriod = keep.String()
		kept.StatutoryFromYearEnd = fromYearEnd
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
				"kept_under_request": outcome.Kept.UnderRequest,
			})
		return err
	})
}
