// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

func validateRestatedEvidence(event extractedEvent, thread settledThread) string {
	if event.RestatedMessageID == "" && event.RestatedQuote == "" {
		return ""
	}
	if event.Kind != extractKindCommitment || strings.TrimSpace(event.RestatedQuote) == "" || utf8.RuneCountInString(event.RestatedQuote) > maxExtractQuote {
		return "a restatement must quote one renewed commitment"
	}
	var original, restated threadMessage
	for _, message := range thread.Messages {
		if message.ID.String() == event.MessageID {
			original = message
		}
		if message.ID.String() == event.RestatedMessageID {
			restated = message
		}
	}
	if original.ID.IsZero() || restated.ID.IsZero() || !restated.At.After(original.At) || restated.Direction != original.Direction {
		return "a restatement must cite a later message from the same side of this conversation"
	}
	if _, ok := evidenceSpan(textlang.CurrentMessage(restated.Body), event.RestatedQuote); !ok {
		return "the restatement quote is not in its sender's own words"
	}
	return ""
}

func (x *SignalExtractor) withRestatedCommitment(ctx context.Context, tx pgx.Tx, thread settledThread, event extractedEvent, commitment Commitment) (Commitment, bool, error) {
	if event.RestatedMessageID == "" || commitment.absentColleague {
		// A promise kept for an absent colleague has no promiser to renew it.
		// It is recorded as it was first made.
		return commitment, true, nil
	}
	id, err := ids.Parse(event.RestatedMessageID)
	if err != nil {
		return Commitment{}, false, err
	}
	parties, err := x.messages.PartiesOf(extractorContext(ctx, signalScanActor), tx, id)
	if err != nil {
		return Commitment{}, false, err
	}
	verified := verifiedCommitmentParties(parties)
	if !samePromiser(commitment, verified) {
		return Commitment{}, false, errRefusedReading
	}
	message, found := citedMessage(thread, id)
	if !found {
		return Commitment{}, false, nil
	}
	if commitment.Seat != nil {
		// The renewal must be the sender's own words too. A later "Robin will
		// send it Friday" does not renew the sender's promise.
		span, _ := evidenceSpan(textlang.CurrentMessage(message.Body), event.RestatedQuote)
		absent, err := senderOwesAnAbsentColleaguesWork(ctx, tx, "seat:"+commitment.Seat.String(), span, verified)
		if err != nil {
			return Commitment{}, false, err
		}
		if absent {
			return commitment, true, nil
		}
	}
	if promisedOnAnothersPrivateThread(commitment) {
		// A renewal is shown with its message's words, and the task is withheld
		// from a holder who cannot open that message. One they cannot open
		// renews nothing for them.
		may, err := auth.SeatMayHoldPrivateWorkTx(ctx, tx, id, commitment.PrivateTo, *commitment.Seat)
		if err != nil {
			return Commitment{}, false, err
		}
		if !may {
			return commitment, true, nil
		}
	}
	commitment.AdmissionSourceID = &id
	commitment.RenewedAt = &message.At
	renewal := commitment.Evidence
	renewal.SourceID, renewal.Snippet = id, event.RestatedQuote
	commitment.RenewalEvidence = &renewal
	return commitment, true, nil
}

func samePromiser(commitment Commitment, parties []commitmentParty) bool {
	var expected string
	if commitment.Theirs != nil {
		expected = "contact:" + commitment.Theirs.String()
	} else if commitment.Seat != nil {
		expected = "seat:" + commitment.Seat.String()
	} else {
		return false
	}
	if commitment.NeedsConfirmation {
		for _, original := range commitment.parties {
			if original.Ref == expected && commitmentPartyHasRole(parties, expected, original.Role) {
				return true
			}
		}
		return false
	}
	found := false
	for _, party := range parties {
		if party.Role != commitmentSenderRole {
			continue
		}
		if party.Ref != expected {
			return false
		}
		found = true
	}
	if !found || commitment.PromisedTo == nil {
		return found
	}
	return commitmentPartyHasRole(parties, "contact:"+commitment.PromisedTo.String(), "to")
}
