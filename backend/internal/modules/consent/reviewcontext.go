// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// Answering a refusal by saying what happened away from the system.
//
// The engine refuses a send to somebody it has no evidence about, and it is
// right to: nothing on the record connects the workspace to that person. But
// the record is not the world. A customer rang and asked for a quote, somebody
// took a card at a stand — and the rep who was there is the only place that
// fact exists.
//
// This is where they put it on the record, from the review that refused them,
// without leaving the message they are trying to send.
//
// WHAT WENT WRONG THE FIRST TIME, and why this file looks the way it does. The
// reverted attempt wrote the statement to `communication_basis`, which is a
// RECORD of a decision the engine already took and is never read back as
// evidence. The endpoint answered 204 and the very next send was refused
// identically — the rep typed their sentence into a box and nothing changed.
//
// What the verdict actually reads is `consent_qualifying_event`
// (verdict.go, latestQualifyingEvent → recordedQualifyingEvent), so that is
// what this writes, through the store method that already owns that table.
//
// IT ANSWERS ONE REFUSAL AND NOT THE OTHERS. A qualifying event settles whether
// ordinary business correspondence is lawful at all, which is the refusal coded
// no_compatible_evidence. It does not touch a marketing objection, a
// suppression, a bounce or a frequency cap — those are the subject's own
// decisions or facts about the address, and no rep's recollection overrides
// one. Saying so in the refusal is better than writing a row that changes
// nothing.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

// fieldSubjectID is the body field naming the person a statement is about. One
// spelling, because the guard below and the refusal above both name it and a
// typo in either would highlight a box the caller did not fill in.
const fieldSubjectID = "subject_id"

// RecordContextInput is one rep's statement about one person.
//
// THE SUBJECT IS NAMED, and that is a correction rather than a detail. The
// first attempt copied one statement onto every refused recipient, so a
// sentence about a phone call with one person became recorded evidence about
// three others who had nothing to do with it. A statement is about whoever it
// is about.
type RecordContextInput struct {
	// SubjectID is the person the statement is about, which must be one of the
	// people this review was refused for.
	SubjectID ids.PersonID
	// Kind is how the exchange happened: an in-person meeting, or a request the
	// subject made themselves.
	Kind string
	// Note is what happened, in the rep's own words. It IS the evidence, so it
	// is required — see RecordQualifyingEvent.
	Note string
	// OccurredAt is when it happened, not when it was typed in.
	OccurredAt time.Time
}

// ContextNotAnswerableError reports a refusal no statement can answer.
//
// It is a 422 with a field rather than a silent success, because the whole
// defect this slice fixes is an endpoint that accepted a sentence and changed
// nothing. A rep who cannot fix this refusal by talking about it needs to be
// told that, not thanked.
type ContextNotAnswerableError struct {
	// Why names what is actually standing in the way, in the rep's terms.
	Why string
}

func (e *ContextNotAnswerableError) Error() string { return e.Why }

// FieldFault carries the refusal to every surface rather than the HTTP one
// alone, so a tool caller gets the same answer a screen does.
func (e *ContextNotAnswerableError) FieldFault() (field, code, message string) {
	return fieldSubjectID, "context_does_not_answer_this", e.Why
}

// RecordContext writes what the rep says happened, and reports whether the
// message can now go.
//
// IT DOES NOT SEND. Recording evidence and sending a message are two decisions,
// and a call that did both would make a rep who mistyped a date into somebody
// who sent a message they were still deciding about. The caller re-previews and
// presses send.
func (s *Store) RecordContext(
	ctx context.Context, reviewID ids.UUID, in RecordContextInput,
) (Review, error) {
	// AN ABSENT subject_id DECODES TO THE ZERO UUID with no error, and would
	// then match no refusal and answer 404 — telling the caller this review does
	// not name a person they never named.
	//
	// Refused at the STORE rather than in the handler, because that is the door
	// every transport comes through: a check in the HTTP handler alone is what
	// left a tool surface and a REST route answering differently.
	//
	// BEFORE THE PERMISSION CHECK, which is safe here and deliberate: a
	// malformed body reveals nothing about what exists, and this reads nothing.
	// Ordering it after would mean the guard could only ever be reached by a
	// caller who was already entitled, which makes it untestable and leaves the
	// unauthenticated case answering about authentication when the real problem
	// is an empty box.
	if err := httperr.RequireBodyID(fieldSubjectID, in.SubjectID.UUID); err != nil {
		return Review{}, err
	}
	// A PERSON, not merely a principal auth.RequireHuman admits. That check
	// lets connectors through, and a connector runs with the granting human's
	// own grants — so it would hold whatever this is gated on and could assert,
	// in a named employee's name, that a phone call happened. This is a
	// statement about what somebody remembers.
	if err := requireAPersonAtTheKeyboard(ctx); err != nil {
		return Review{}, err
	}
	review, err := s.ReviewForReader(ctx, reviewID)
	if err != nil {
		return Review{}, err
	}
	// A REVIEW THAT IS OVER TAKES NO STATEMENT.
	//
	// ReviewForReader answers terminal rows too — the erasure and the cancel
	// both leave them readable on purpose, so a queue does not develop holes —
	// and without this a rep could answer a review whose message was cancelled
	// weeks ago, writing evidence that then authorizes some unrelated future
	// send. A routed one is refused for a different reason: somebody else is
	// deciding it, and changing the facts under them would have them approve a
	// question that is no longer the one they were asked.
	if review.State != ReviewNeedsContext && review.State != ReviewNeedsRepair {
		return Review{}, &ContextNotAnswerableError{Why: whyThisReviewIsNotOpenToIt(review.State)}
	}
	refusal, found := refusalNaming(review, in.SubjectID)
	if !found {
		// Not found rather than forbidden: from the caller's side there is no
		// such person on this review, and saying which of the two it was would
		// disclose who else the message was refused for.
		return Review{}, apperrors.ErrNotFound
	}
	if refusal.ReasonCode != commsauthz.ReasonNoEvidence {
		return Review{}, &ContextNotAnswerableError{Why: whyAStatementCannotAnswer(refusal.ReasonCode)}
	}
	// THE REFUSALS ON THE ROW ARE A SNAPSHOT, taken when the send was refused.
	// A stop recorded since then is not in them, so a review that still reads
	// "no evidence" can belong to somebody who has since asked us to stop —
	// and accepting a statement there reports success about a message that will
	// be refused anyway, which is this slice's own failure mode on a third axis.
	//
	// Asked of the table directly rather than by re-running the engine: whether
	// a live stop EXISTS is one question, and re-deciding the whole send here
	// would be a second copy of a decision the send path makes properly.
	binding, err := aStopThatBindsTheMessage(ctx, s.db, in.SubjectID, refusal)
	if err != nil {
		return Review{}, err
	}
	if binding != "" {
		return Review{}, &ContextNotAnswerableError{Why: whyAStatementCannotAnswer(binding)}
	}
	// AN EXCHANGE THE VERDICT WOULD IGNORE IS REFUSED HERE.
	//
	// recordedQualifyingEvent bounds its read by the reply window
	// (qualifyingground.go), so a statement about something older than that
	// window is written, stored, and then never read — the endpoint reports
	// success and the very next send is refused identically. That is precisely
	// the defect this slice exists to fix, and it would have come back on the
	// date axis instead of the table axis.
	//
	// Asked of the same resolver the verdict uses, on its own transaction, so a
	// jurisdiction pack that shortens the window shortens this too. Two copies
	// of the bound would drift, and the drift would be silent in exactly the
	// same way.
	if err := s.refuseAnExchangeTooOldToCount(ctx, in.OccurredAt); err != nil {
		return Review{}, err
	}
	if _, err := s.RecordQualifyingEvent(ctx, in.SubjectID, RecordQualifyingEventInput{
		Kind:       in.Kind,
		Note:       in.Note,
		OccurredAt: in.OccurredAt,
	}); err != nil {
		return Review{}, err
	}
	// RE-READ, rather than patching the copy in hand. What the caller needs to
	// know is whether the message can go NOW, and the only honest source for
	// that is the row as it stands after the write — a suppression recorded
	// while the rep was typing is exactly the case a patched copy would miss.
	return s.ReviewForReader(ctx, reviewID)
}

// refusalNaming finds the refusal this statement is about.
//
// BY SUBJECT ID, never by address. Two recipients can share an address in a
// malformed envelope, and the subject is what the qualifying event is written
// against — matching on anything else would record evidence about one person
// under the authority of a refusal about another.
func refusalNaming(review Review, subject ids.PersonID) (RefusedRecipient, bool) {
	want := subject.String()
	for _, r := range review.Refusals {
		if r.SubjectID == want {
			return r, true
		}
	}
	return RefusedRecipient{}, false
}

// whyAStatementCannotAnswer says what is really in the way, in the words the
// rep needs rather than the engine's code.
//
// NAMED ONE BY ONE rather than answered with "this cannot be fixed here",
// because each of these has a different next move and the rep is about to go
// looking for it. A default that said nothing would send them back to the same
// box to try different words.
func whyAStatementCannotAnswer(reason string) string {
	switch reason {
	case commsauthz.ReasonObjection, commsauthz.ReasonSubjectRequest:
		return "this person asked us to stop, so what you remember does not change it; " +
			"a send that has to go anyway needs a recorded exception"
	case commsauthz.ReasonRestricted:
		return "this record is under a processing restriction, which only lifting the " +
			"restriction changes"
	case commsauthz.ReasonHardBounce:
		return "this address does not accept mail, so the message would not arrive whatever " +
			"the evidence says; correct the address"
	case commsauthz.ReasonConsentWithdrawn:
		return "they took their consent back, and only they can give it again"
	case commsauthz.ReasonNoMarketingConsent, commsauthz.ReasonUnconfirmedDOI:
		return "marketing needs their express consent, which no recollection of a conversation " +
			"can supply"
	case commsauthz.ReasonFrequencyCapReached:
		return "this jurisdiction's limit on how often we may write has been reached; it passes " +
			"with time rather than with evidence"
	case commsauthz.ReasonNoSubject:
		return "this address does not resolve to one person, so there is nobody to record the " +
			"evidence against"
	case commsauthz.ReasonUnknownPurpose:
		return "this message names a purpose this installation does not define, which is a " +
			"problem with the message rather than with what you know about the person"
	case commsauthz.ReasonLegacyTransactionalUnevidenced:
		return "this message was sent under the old transactional key with nothing behind it; " +
			"give it a communication context and the engine can judge it properly"
	}
	return fmt.Sprintf("this refusal (%s) is not one a statement about what happened can answer", reason)
}

// HeldSubjectsForReview lists the people a review was refused for, so a caller
// re-reading a review knows whom it may take a statement about.
//
// It exists because the answer is not simply "every refusal": only the ones
// coded no_compatible_evidence can be answered this way, and offering a form
// for the others is offering a box that cannot work.
func HeldSubjectsForReview(review Review) []ids.PersonID {
	var out []ids.PersonID
	for _, r := range review.Refusals {
		if r.ReasonCode != commsauthz.ReasonNoEvidence || r.SubjectID == "" {
			continue
		}
		id, err := ids.Parse(r.SubjectID)
		if err != nil {
			continue
		}
		out = append(out, ids.From[ids.PersonKind](id))
	}
	return out
}

// refuseAnExchangeTooOldToCount refuses a statement the verdict would not read.
//
// THE MESSAGE SAYS WHAT TO DO ABOUT IT rather than only that it was refused. A
// rep whose evidence has aged out is not making a mistake — the fact is true
// and no longer supports an unprompted message — and the next move is a
// recorded exception, not a differently worded sentence.
func (s *Store) refuseAnExchangeTooOldToCount(ctx context.Context, occurredAt time.Time) error {
	var window time.Duration
	if err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		rules, err := s.packRulesFor(ctx, tx)
		if err != nil {
			return err
		}
		window = rules.reply
		return nil
	}); err != nil {
		return err
	}
	// NOT-BEFORE rather than after, matching recordedQualifyingEvent's own
	// `occurred_at >= since`. An exclusive bound here would refuse a statement
	// exactly at the edge that the verdict would have read — a refusal the rep
	// cannot act on, about a fact that would have worked.
	if !occurredAt.Before(time.Now().Add(-window)) {
		return nil
	}
	return &ContextNotAnswerableError{
		Why: fmt.Sprintf("this happened more than %d days ago, which is longer than a recorded "+
			"exchange keeps supporting a message the person did not prompt; a send that has to "+
			"go anyway needs a recorded exception", int(window.Hours()/24)),
	}
}

// whyThisReviewIsNotOpenToIt says what happened to a review that cannot take a
// statement, in the words the rep needs.
func whyThisReviewIsNotOpenToIt(state string) string {
	switch state {
	case ReviewAwaitingDecision:
		return "somebody else is deciding this message, so adding evidence now would change the " +
			"question under them; wait for their answer or withdraw the ask"
	case ReviewCancelled:
		return "this message was cancelled, so there is nothing left to send"
	case ReviewResolved:
		return "this message has already gone"
	}
	return "this review is no longer open to new evidence"
}

// aStopThatBindsTheMessage answers which live stop, if any, would refuse the
// message this statement is trying to make lawful. Empty means none does.
//
// THE CATEGORY IS KNOWN HERE — the refusal carries the one the engine resolved
// for this recipient — and that is what makes this narrower than a bare EXISTS.
// The question is not "is there a stop" but "is there a stop that binds THIS
// message", and a marketing objection binds marketing only. An earlier spelling
// refused on any live row, which turned a person who had opted out of the
// newsletter into somebody a rep could not record a phone call about, while the
// send path would have let the ordinary letter through.
//
// THROUGH suppressionBinds RATHER THAN A SECOND RULE. Which categories a kind
// reaches is the engine's judgement and lives one file over; a copy here would
// be a second answer free to drift from the one the send actually applies.
//
// KEYED THE WAY THE SEND PATH KEYS IT (authorizetransmitrecord.go): person,
// lead, or the address itself. A hard bounce recorded against the address alone
// carries no person id, and a check that looked only at the person would accept
// a statement about a mailbox that does not accept mail.
func aStopThatBindsTheMessage(
	ctx context.Context, db *database.DB, subject ids.PersonID, refusal RefusedRecipient,
) (string, error) {
	var kinds []string
	err := db.Tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT DISTINCT kind FROM communication_suppression
			 WHERE revoked_at IS NULL
			   AND (person_id = $1
			        OR lead_id = $1
			        OR (address IS NOT NULL AND $2 <> '' AND lower(address) = lower($2)))`,
			subject.UUID, refusal.Address)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var kind string
			if err := rows.Scan(&kind); err != nil {
				return err
			}
			kinds = append(kinds, kind)
		}
		return rows.Err()
	})
	if err != nil {
		return "", fmt.Errorf("consent: asking whether a stop already refuses this message: %w", err)
	}
	// The category the engine resolved for this recipient, which the refusal
	// carries. An empty one means the engine never got that far, and the safe
	// reading is that any live stop binds — the direction suppressionBinds
	// itself fails in for a kind it does not recognise.
	category := commsauthz.Category(refusal.Category)
	for _, kind := range kinds {
		if category == "" || suppressionBinds(kind, category) {
			return kind, nil
		}
	}
	return "", nil
}
