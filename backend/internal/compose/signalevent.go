// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/signals"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

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
