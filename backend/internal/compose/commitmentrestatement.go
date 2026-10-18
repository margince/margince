// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
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
	if event.RestatedMessageID == "" {
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
	if !samePromiser(commitment, parties) {
		return Commitment{}, false, fmt.Errorf("%w: a renewal does not have the original promiser and recipient", errRefusedReading)
	}
	message, found := citedMessage(thread, id)
	if !found {
		return Commitment{}, false, nil
	}
	commitment.AdmissionSourceID = &id
	commitment.RenewedAt = &message.At
	renewal := commitment.Evidence
	renewal.SourceID, renewal.Snippet = id, event.RestatedQuote
	commitment.RenewalEvidence = &renewal
	return commitment, true, nil
}

func samePromiser(commitment Commitment, parties activities.MessageParties) bool {
	if commitment.Theirs != nil {
		sender, one := onlyOne(parties.SenderContacts)
		return one && sender == commitment.Theirs.UUID
	}
	sender, one := onlyOne(parties.SenderSeats)
	if !one || commitment.Seat == nil || sender != *commitment.Seat {
		return false
	}
	return commitment.PromisedTo == nil || slices.Contains(parties.RecipientContacts, commitment.PromisedTo.UUID)
}
