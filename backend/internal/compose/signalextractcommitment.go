// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// A commitment read out of an email conversation, handed to the commitment
// rule. Who made it is read from the message, never from the model: an
// outbound message is ours and the colleague who sent it promised; an inbound
// one is the customer's.

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
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
		// The sender's own words, as dispatch reads them: a quote found only
		// in the history the message quotes is not this sender's commitment.
		if _, ok := evidenceSpan(textlang.CurrentMessage(message.Body), event.Quote); !ok {
			return "a commitment quotes words its sender did not write in this message"
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
	ctx context.Context, tx pgx.Tx, thread settledThread, event extractedEvent, cited ids.UUID,
) (bool, error) {
	commitment, ok, err := x.emailCommitment(ctx, tx, thread, event, cited)
	if err != nil || !ok {
		return false, err
	}
	outcome, _, err := x.dispatch.DispatchTx(ctx, tx, commitment, ids.NewV7())
	return outcome != CommitmentRemembered, err
}

// emailCommitment names who made one commitment and to whom. It answers false
// for a commitment nobody can be named for on the customer's side, because a
// promise to watch with no contact to watch it on is filed nowhere.
func (x *SignalExtractor) emailCommitment(
	ctx context.Context, tx pgx.Tx, thread settledThread, event extractedEvent, cited ids.UUID,
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
	if err != nil || !offered {
		return Commitment{}, false, err
	}
	thread.CompanyID, thread.PrivateTo = company, privateTo
	parties, err := x.messages.PartiesOf(extractorContext(ctx, signalScanActor), tx, cited)
	if err != nil {
		return Commitment{}, false, err
	}
	c := Commitment{
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
	var kind, party string
	switch message.Direction {
	case string(crmcontracts.ActivityDirectionInbound):
		sender, one := onlyOne(parties.SenderContacts)
		if !one {
			return Commitment{}, false, nil
		}
		contact := ids.From[ids.ContactKind](sender)
		c.Theirs, kind, party = &contact, claimKindTheirs, "contact:"+sender.String()
	case string(crmcontracts.ActivityDirectionOutbound):
		kind, party = claimKindOurs, "us"
		if seat, one := onlyOne(parties.SenderSeats); one {
			c.Seat, party = &seat, "seat:"+seat.String()
		}
		if recipient, one := onlyOne(parties.RecipientContacts); one {
			promisedTo := ids.From[ids.ContactKind](recipient)
			c.PromisedTo = &promisedTo
			c.Links = append(c.Links, activities.ActivityLinkInput{EntityType: string(recordTypeContact), EntityID: recipient})
		}
	default:
		// Nobody recorded which side wrote it, and a promise attributed to the
		// wrong side is worse than one not filed.
		return Commitment{}, false, nil
	}
	c.Party = emailParty(message.Direction)
	c.Body = fmt.Sprintf("%s in the email %q, sent %s.", c.Party, message.Subject, message.At.Format(time.DateOnly))
	c.Locator = commitmentLocator(cited, kind, party, span)
	return c, true, nil
}

// emailParty names who made a promise in an email, as the task body says it.
func emailParty(direction string) string {
	if direction == string(crmcontracts.ActivityDirectionInbound) {
		return "The customer committed to this"
	}
	return "We committed to this"
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
