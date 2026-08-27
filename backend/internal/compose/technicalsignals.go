// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// A change in what a company runs, as a signal on its account.
//
// This is the seam between the people module, which owns the company record and
// notices the change while writing it, and the signals module, which owns the
// row. Neither imports the other; the edge is spelled here, which is the same
// arrangement every other cross-module producer uses.
//
// The event fires on CHANGE, never on observation. A scheduled pass over a
// company that runs exactly what it ran last week must raise nothing — a rep
// opening the account should see "they moved to Microsoft 365 in March", not a
// weekly restatement of a mail provider that never moved.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/people"
	"github.com/margince/margince/backend/internal/modules/signals"
)

// The signal vocabulary for a technical change: one kind, because a rep filters
// for "something about their stack moved" rather than for which field it was,
// and the summary already says which.
const (
	kindTechnicalChange = "technical_change"
	technicalChannel    = "derived"
	technicalSource     = "technical-lookup"
	// technicalPreviousKey names what the record held before, which is what
	// makes a move readable as a move rather than as an arrival.
	technicalPreviousKey = "previous"
)

// technicalChangeRecorder builds the recorder the people module calls inside
// its own write transaction, so the record change and the company event commit
// together or not at all.
func technicalChangeRecorder() people.TechnicalChangeRecorder {
	return func(ctx context.Context, tx pgx.Tx, change people.TechnicalChange, at time.Time) error {
		summary, ok := technicalChangeSummary(change)
		if !ok {
			// A change in a field nobody would act on. The record still holds
			// it; it just does not earn a line on the account's signal list.
			return nil
		}
		_, err := signals.RecordDerived(ctx, tx, signals.DerivedSignal{
			Kind:           kindTechnicalChange,
			OrganizationID: change.OrganizationID.UUID,
			Summary:        summary,
			// Never `warn` or `urgent`: a company changing its own systems is
			// news about the account, not a problem with it.
			Severity: severityInfo,
			Channel:  technicalChannel,
			Source:   technicalSource,
			// The state it arrived at AND the state it left, so a company that
			// moves Google → Microsoft → Google → Microsoft raises an event
			// each time rather than the second Microsoft move colliding with
			// the first. A pass over an UNCHANGED company still raises
			// nothing, because it produces no change to file at all.
			Fingerprint: fingerprintOf(technicalSource, change.OrganizationID.String(),
				change.Field, change.ValueKey, string(change.Kind), change.Previous),
			// The evidence is the public record that proved it — the MX host,
			// the certificate hostname, the matched marker — because "how do
			// you know?" is the first question this claim invites.
			Evidence: []signals.DerivedEvidence{{Snippet: change.Evidence}},
			Audit: map[string]any{
				paramKind:            string(change.Kind),
				extractionFieldKey:   change.Field,
				extractionValueKey:   change.Value,
				technicalPreviousKey: change.Previous,
			},
		}, at)
		if err != nil {
			return fmt.Errorf("filing a technical change signal: %w", err)
		}
		return nil
	}
}

// technicalChangeSummary is the sentence a rep reads on the account.
//
// Written per field rather than from a template, because the four fields are
// four different pieces of news: a mail system moving is an IT decision worth a
// call, a careers page appearing is a hiring signal, and a shared phrasing
// would flatten both into "a technical signal changed".
func technicalChangeSummary(change people.TechnicalChange) (string, bool) {
	switch change.Field {
	case people.FactMailProvider:
		if change.Kind == people.TechnicalMoved {
			return fmt.Sprintf("Mail läuft jetzt über %s (vorher %s)", change.Value, change.Previous), true
		}
		return fmt.Sprintf("Mail läuft über %s", change.Value), true
	case people.FactOperatedService:
		if change.Kind == people.TechnicalGone {
			return fmt.Sprintf("%s ist offline gegangen", change.Value), true
		}
		return fmt.Sprintf("%s ist neu online", change.Value), true
	case people.FactHostingProvider:
		if change.Kind == people.TechnicalMoved {
			return fmt.Sprintf("Hosting jetzt bei %s (vorher %s)", change.Value, change.Previous), true
		}
		return fmt.Sprintf("Hosting bei %s", change.Value), true
	case people.FactTechnology:
		if change.Kind == people.TechnicalGone {
			return fmt.Sprintf("%s wird nicht mehr eingesetzt", change.Value), true
		}
		return fmt.Sprintf("Setzt jetzt %s ein", change.Value), true
	default:
		// email_security lands here on purpose. A DMARC policy tightening is a
		// real fact about the company and belongs on the record, but it is not
		// a reason to pick up the phone, and every account publishing one would
		// put a line on every signal list for nobody to act on.
		return "", false
	}
}
