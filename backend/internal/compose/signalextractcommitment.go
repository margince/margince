// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// extractKindCommitment is the event kind that goes to the commitment rule.
const extractKindCommitment = "commitment_made"

// maxExtractQuote bounds the quote a reply may carry. A promise is a clause;
// a quote longer than this is the message being copied.
const maxExtractQuote = 300

// validateEventEvidence holds a reply's quote and due day to what the cited
// message supports. A commitment must quote the message it cites, word for
// word: the quote is what a reader checks the promise against, and a quote the
// message does not contain is a promise nobody can check.
func validateEventEvidence(event extractedEvent, thread settledThread) string {
	if reason := validateLifecycleEvidence(event, thread); reason != "" {
		return reason
	}
	if reason := validateOpportunityBuyer(event, thread); reason != "" {
		return reason
	}
	if reason := validateRestatedEvidence(event, thread); reason != "" {
		return reason
	}
	if !validProposedDueDate(event.DueDate) {
		return fmt.Sprintf("an event states the due date %q, and a due date is YYYY-MM-DD or empty",
			clampToken(event.DueDate))
	}
	if event.Kind != extractKindCommitment {
		return ""
	}
	if strings.TrimSpace(event.Quote) == "" {
		return "a commitment carries no quote, so nobody could check what was promised"
	}
	if utf8.RuneCountInString(event.Quote) > maxExtractQuote {
		return fmt.Sprintf("a commitment's quote is longer than %d characters, and a promise is one clause",
			maxExtractQuote)
	}
	for _, message := range thread.Messages {
		if message.ID.String() != event.MessageID {
			continue
		}
		if msg := validateCommitmentRecipients(event.RecipientRefs, message.Parties); msg != "" {
			return msg
		}
		if event.PromiserRef != "" {
			if _, found := referencedPromiser(event.PromiserRef, message.Parties); !found {
				return "a commitment names a promiser outside its verified message participants"
			}
		}
		// The sender's own words, as dispatch reads them: a quote found only
		// in the history the message quotes is not this sender's commitment.
		if _, ok := evidenceSpan(textlang.CurrentMessage(message.Body), event.Quote); !ok {
			return "a commitment quotes words its sender did not write in this message"
		}
	}
	return validateCommitmentMeaning(event.Meaning, event.DueDate, commitmentMeaningSource(event, thread))
}

func commitmentMeaningSource(event extractedEvent, thread settledThread) string {
	id, quote := event.MessageID, event.Quote
	if event.RestatedMessageID != "" {
		id, quote = event.RestatedMessageID, event.RestatedQuote
	}
	for _, message := range thread.Messages {
		if message.ID.String() == id {
			span, found := evidenceSpan(textlang.CurrentMessage(message.Body), quote)
			if found {
				return span
			}
		}
	}
	return ""
}

// evidenceSpan finds the sentence of a body that contains a quote, with runs
// of whitespace folded. The sentence, not the quote, identifies the promise:
// two readings that quote a longer or a shorter piece of one sentence find the
// same one.
func evidenceSpan(body, quote string) (string, bool) {
	text, wanted := strings.Join(strings.Fields(body), " "), strings.Join(strings.Fields(quote), " ")
	at := strings.Index(text, wanted)
	if wanted == "" || at < 0 {
		return "", false
	}
	start := 0
	for i := at - 1; i >= 0; i-- {
		if sentenceEnds(text, i) {
			start = i + 1
			break
		}
	}
	// From the quote's last character, so a quote that ends its sentence ends
	// the span there rather than running into the next one.
	end := len(text)
	for i := at + len(wanted) - 1; i < len(text); i++ {
		if sentenceEnds(text, i) {
			end = i + 1
			break
		}
	}
	return strings.TrimSpace(text[start:end]), true
}

// sentenceEnds reports whether the byte at i ends a sentence. A point between
// two digits, in any script, is a decimal ("2.5 days"), not the end of one.
func sentenceEnds(text string, i int) bool {
	switch text[i] {
	case '!', '?':
		return true
	case '.':
		before, _ := utf8.DecodeLastRuneInString(text[:i])
		after, _ := utf8.DecodeRuneInString(text[i+1:])
		return !unicode.IsDigit(before) || !unicode.IsDigit(after)
	}
	return false
}

// dispatchCommitment files one commitment from a conversation through the
// commitment rule, and reports whether anything new came of it.
func (x *SignalExtractor) dispatchCommitment(
	ctx context.Context, tx pgx.Tx, thread settledThread, event extractedEvent, cited ids.UUID, asOf time.Time,
) (CommitmentOutcome, error) {
	commitment, ok, err := x.emailCommitment(ctx, tx, thread, event, cited, asOf)
	if errors.Is(err, apperrors.ErrNotFound) {
		return CommitmentOutcome(activities.CommitmentSourceUnavailable), nil
	}
	if err != nil || !ok {
		return CommitmentRemembered, err
	}
	var outcome CommitmentOutcome
	err = pgx.BeginFunc(ctx, tx, func(savepoint pgx.Tx) error {
		var dispatchErr error
		outcome, _, dispatchErr = x.dispatch.DispatchTx(ctx, savepoint, commitment, ids.NewV7())
		return dispatchErr
	})
	if errors.Is(err, apperrors.ErrNotFound) {
		return CommitmentOutcome(activities.CommitmentSourceUnavailable), nil
	}
	return outcome, err
}

// emailCommitment names who made one commitment and to whom. It answers false
// for a commitment nobody can be named for on the customer's side, because a
// promise to watch with no contact to watch it on is filed nowhere.
func (x *SignalExtractor) emailCommitment(
	ctx context.Context, tx pgx.Tx, thread settledThread, event extractedEvent, cited ids.UUID, asOf time.Time,
) (Commitment, bool, error) {
	message, found := citedMessage(thread, cited)
	// Only the words the sender wrote: a reply quoting our own promise back to
	// us must not file that promise as theirs.
	span, spanned := evidenceSpan(textlang.CurrentMessage(message.Body), event.Quote)
	if !found || !spanned {
		return Commitment{}, false, nil
	}
	// Who may read the conversation is asked again here, in the transaction
	// that files: the reading ran outside it, and a conversation made private
	// in between must not be filed as a shared one.
	company, privateTo, offered, err := threadReaderNow(ctx, tx, thread.Key)
	if err != nil {
		return Commitment{}, false, err
	}
	if !offered {
		return Commitment{}, false, apperrors.ErrNotFound
	}
	thread.CompanyID, thread.PrivateTo = company, privateTo
	parties, err := x.messages.PartiesOf(extractorContext(ctx, signalScanActor), tx, cited)
	if err != nil {
		return Commitment{}, false, err
	}
	c := Commitment{
		Meaning:          event.Meaning,
		AsOf:             asOf,
		ReadMessages:     fullyReadMessages(thread.Messages),
		Extractor:        signalScanActor,
		SourceActivityID: cited,
		Summary:          event.Summary,
		Quote:            event.Quote,
		DueDate:          event.DueDate,
		Confidence:       float64(event.Confidence),
		Links:            []activities.ActivityLinkInput{{EntityType: string(recordTypeCompany), EntityID: thread.CompanyID}},
		Evidence: approvals.Evidence{
			Snippet: event.Quote, SourceType: string(recordTypeActivity), SourceID: cited,
		},
	}
	if !thread.PrivateTo.IsZero() {
		owner := thread.PrivateTo
		c.PrivateTo = &owner
	}
	verified := verifiedCommitmentParties(parties)
	promiser := event.PromiserRef
	// The reply's promiser is the model's reading. The sentence and the seat
	// roster can still show the work is a colleague's who is not on the mail.
	if absent, err := senderOwesAnAbsentColleaguesWork(ctx, tx, promiser, span, verified); err != nil {
		return Commitment{}, false, err
	} else if absent {
		promiser = ""
	}
	c.absentColleague = promiser != event.PromiserRef
	kind, party := attributeCommitment(&c, promiser, verified)
	if err := attributeCommitmentRecipients(&c, event.RecipientRefs, verified); err != nil {
		return Commitment{}, false, err
	}
	c.NeedsConfirmation = message.Direction != connector.DirectionOutbound || !commitmentPartyHasRole(verified, promiser, commitmentSenderRole)
	c.Body = fmt.Sprintf("Promiser: %s. Commitment in the email %q, sent %s.", c.Party, message.Subject, message.At.Format(time.DateOnly))
	c.Span = span
	c.Locator = commitmentLocator(cited, kind, party, span)
	if promisedOnAnothersPrivateThread(c) {
		// The model sums up a window of messages, and the promiser may open only
		// the one they wrote. Their own sentence is all the task may say.
		c.Summary = span
	}
	return x.withRestatedCommitment(ctx, tx, thread, event, c)
}

// promisedOnAnothersPrivateThread is a colleague's promise on a thread private
// to another member, such as mail they sent that only that member's mailbox
// captured.
func promisedOnAnothersPrivateThread(c Commitment) bool {
	return c.Theirs == nil && c.Seat != nil && heldByAnotherThanOwner(c.PrivateTo, *c.Seat)
}

// citedMessage is the message of the conversation an event cites.
func citedMessage(thread settledThread, cited ids.UUID) (threadMessage, bool) {
	for _, message := range thread.Messages {
		if message.ID == cited {
			return message, true
		}
	}
	return threadMessage{}, false
}

// onlyOne is the single id of a list, when it holds exactly one.
func onlyOne(list []ids.UUID) (ids.UUID, bool) {
	if len(list) != 1 {
		return ids.UUID{}, false
	}
	return list[0], true
}

func fullyReadMessages(messages []threadMessage) []ids.UUID {
	read := make([]ids.UUID, 0, len(messages))
	for _, message := range messages {
		// A body the parser did not read in full is never read in full here.
		if message.Incomplete {
			continue
		}
		if message.UnreadRunes == 0 || (message.QuotedRemainder && textlang.CurrentMessage(message.Body) != strings.TrimSpace(message.Body)) {
			read = append(read, message.ID)
		}
	}
	return read
}
