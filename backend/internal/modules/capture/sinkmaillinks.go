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

// postCommitLinkReservation is how many link seats this arm leaves for the
// writers after the capture transaction (sink.go). They are the counterparty
// ensure, the project ladder and the deal rule, one each. None reads this budget. So
// the room is kept here, or the 25-link ceiling refuses their links.
const postCommitLinkReservation = 3

// linkResolvedMailParticipants files a captured message under every participant
// who is a live contact or a live lead (participantfiling.go).
//
// It answers nothing, and that is the point. The meeting arm returns a count
// because the audience limiter reads it: a meeting filed under an attendee has
// a record standing behind it, so it is not the link-less mail the limiter
// holds. A cc'd contact or a lead is not that claim — the message may still be
// filed under nobody the ladder judged — so this arm must not feed that
// decision. Returning a count is how a later reader wires it in by accident, so
// there is none to wire.
//
// Held by: TestASuppressedMessageIsFiledUnderNobodyItCopies and
// TestFilingAConfidentialMessageDoesNotWidenWhoMayReadIt
// (backend/internal/compose/integration/capture/capture_participantlink_integration_test.go)
func (s *Sink) linkResolvedMailParticipants(
	ctx context.Context, tx pgx.Tx, activityID ids.ActivityID, existing int,
) error {
	// The budget is what the row can still take rather than a fresh 25: the
	// ceiling is on the ACTIVITY, whatever wrote the links. The seats reserved
	// on top are for the writers that run after this transaction commits
	// without consulting this budget (postCommitLinkReservation).
	_, err := fileUnderParticipants(tx, activityID, participantFiling{
		decider: ctx,
		budget:  max(maxDerivedMeetingLinks-existing-postCommitLinkReservation, 0),
		write:   insertParticipantLink(ctx, tx, activityID),
	})
	return err
}
