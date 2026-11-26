// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// Filing a captured message under the contacts who were on it, beyond the one
// the ladder judged.
//
// Mail reaches its records through the COUNTERPARTY: one address, one ensure,
// one link. That is right for the party the message is with, and it is the
// whole filing a message ever got — so a contact who was merely cc'd was
// stamped as a participant and filed nowhere. The message never reached their
// timeline, and nothing that reads through activity_link could see it.
//
// It cost more than a missing row. Ordinary business correspondence is lawful
// under Art 6(1)(f) when the contact has written to us, and consent reads that
// from an INBOUND activity linked to them (consent.inboundQualifyingEvent). A
// cc'd contact's inbound mail therefore qualified nobody: the send gate refused
// mail to a contact whose replies were sitting in the workspace, unlinked. The
// two repair sweeps could not reach it either — one keys on counterparty_email,
// which names somebody else on these messages, and the other is gated on
// meetings.
//
// So this is the meeting arm's rule (sinkmeetinglinks.go) applied to the
// transports that carry a counterparty, and it is deliberately the SAME rule:
// contacts already resolved on the participant rows, nobody created, the merge
// redirect settled, the visibility probe made, the 25-link ceiling respected.
//
// TWO DIFFERENCES, both load-bearing:
//
// A SUPPRESSED message is filed under nobody. The ladder judged its sender
// infrastructure or noise, and a newsletter that happens to name a colleague is
// not correspondence with them. Filing it would put a bulk sender's mail on a
// contact's timeline and hand it a qualifying event the judgement just refused.
//
// The count is NOT reported to the audience limiter — see the caller for what
// that would cost. These links say where a message is FILED; they say nothing
// about who may read it.

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// laterLinkReservation is how many link seats this arm leaves for the writers
// that run after it (sink.go), one each. They are the counterparty ensure later
// in the capture transaction, then the project ladder and the deal rule. None
// reads this budget, so the room is kept here, or the 25-link ceiling refuses
// their links.
const laterLinkReservation = 3

// fileMailUnderParticipants locks the mail's whole reach, then files it under
// the contacts and leads on it. A suppressed mail is filed under nobody.
func (s *Sink) fileMailUnderParticipants(
	ctx context.Context, tx pgx.Tx, activityID ids.ActivityID, existing int, decision counterpartyDecision, private bool,
) error {
	named, err := lockMailReachTx(ctx, tx, activityID, decision)
	if err != nil || decision.traceOutcome == TraceSuppressed {
		return err
	}
	return s.linkResolvedMailParticipants(ctx, tx, activityID, named, existing, private)
}

// linkResolvedMailParticipants files a captured message under every participant
// who is a live contact or a live lead (participantfiling.go).
//
// It answers no count, unlike the meeting arm. The audience limiter reads that
// count, and a copied contact or a lead does not make a message filed. A count
// here would let a later reader wire it into the limiter and open held mail.
//
// Held by: TestASuppressedMessageIsFiledUnderNobodyItCopies and
// TestFilingAConfidentialMessageDoesNotWidenWhoMayReadIt
// (backend/internal/compose/integration/capture/capture_participantlink_integration_test.go)
func (s *Sink) linkResolvedMailParticipants(
	ctx context.Context, tx pgx.Tx, activityID ids.ActivityID, named []participantRecord, existing int, private bool,
) error {
	// The ceiling is on the activity. The budget is what the row can still
	// take, less the seats kept for the writers after this arm.
	_, err := fileUnderRecords(tx, named, participantFiling{
		decider: ctx,
		budget:  max(maxDerivedMeetingLinks-existing-laterLinkReservation, 0),
		private: private,
		write:   insertParticipantLink(ctx, tx, activityID),
	})
	return err
}
