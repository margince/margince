// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/signals"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// The facts a new_opportunity cite carries under "fields". Deal Scout reads an
// explicit request as enough to open a deal on its own.
const (
	requestKindKey      = "request_kind"
	budgetQuoteKey      = "budget_quote"
	decisionDateKey     = "decision_date"
	requestKindExplicit = "explicit_request"
	requestKindNeed     = "need"
)

// keptFromThread is what a reading may file. A thread with mail private by a
// routine setting gives up only an explicit request, and that request is
// shared. The deal it opens shows what the customer asked for. Every other
// finding stays with that mail.
func keptFromThread(events []extractedEvent, shared bool) []extractedEvent {
	if shared {
		return events
	}
	return slices.DeleteFunc(slices.Clone(events), func(event extractedEvent) bool {
		return event.Kind != extractKindOpportunity || event.RequestKind != requestKindExplicit
	})
}

// requestFacts are a new opportunity's request facts as the cited message
// states them. A budget its sender did not write in it, or a decision day
// that is no date, is dropped rather than filed.
func requestFacts(event extractedEvent, thread settledThread, cited ids.UUID) map[string]any {
	if event.Kind != extractKindOpportunity || event.RequestKind == "" {
		return nil
	}
	facts := map[string]any{requestKindKey: event.RequestKind}
	if event.DecisionDate != "" && validProposedDueDate(event.DecisionDate) {
		facts[decisionDateKey] = event.DecisionDate
	}
	message, found := citedMessage(thread, cited)
	if span, ok := evidenceSpan(textlang.CurrentMessage(message.Body), event.BudgetQuote); found && ok {
		facts[budgetQuoteKey] = span
	}
	return facts
}

// recordEventSignal files one event as a signal on the account.
func recordEventSignal(
	ctx context.Context, tx pgx.Tx, thread settledThread,
	event extractedEvent, cited ids.UUID, now time.Time, outcome CommitmentOutcome,
) (bool, error) {
	audit := map[string]any{
		paramKind: event.Kind, "thread_key": thread.Key,
		extractionConfidenceKey: float64(event.Confidence),
		commitmentDispatchedKey: event.Kind == extractKindCommitment,
	}
	if event.Kind == extractKindCommitment {
		audit["commitment_admission"] = string(outcome)
		audit["commitment_admission_version"] = activities.CommitmentAdmissionVersion
	}
	fingerprint := signalFingerprint(event.Kind, thread.CompanyID, cited)
	buyer, role := opportunityBuyer(event, thread)
	if buyer != nil {
		company, err := opportunityCompany(ctx, tx, buyer, cited)
		if err != nil {
			return false, err
		}
		if company.IsZero() {
			buyer, role = nil, commercialRoleUnknown
		} else {
			thread.CompanyID = company
		}
	}
	evidence := eventSignalEvidence(event, cited, thread)
	evidence.BuyerContactID, evidence.CommercialRole = buyer, role
	evidence.Fields = requestFacts(event, thread, cited)
	return signals.RecordDerived(ctx, tx, signals.DerivedSignal{
		Kind:        event.Kind,
		CompanyID:   thread.CompanyID,
		Summary:     event.Summary,
		Severity:    extractKinds[event.Kind],
		Fingerprint: fingerprint,
		Evidence: []signals.DerivedEvidence{
			evidence,
		},
		// A company attribution cannot widen the source conversation's audience.
		PrivateTo: thread.PrivateTo,
		Audit:     audit,
	}, now)
}
