// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// Recording the one qualifying event nothing can derive.
//
// A `business_correspondence` verdict reads the most recent thing on the record
// that makes writing to somebody lawful, and three of the four kinds arrive on
// their own: an inbound message, an inquiry and an open deal are all facts the
// product already holds and the verdict derives them itself.
//
// The fourth happened away from every system. Somebody handed over a card at a
// trade fair, and the only record of it is the memory of the contact who was
// there. This is where they write it down — and the note is required, because
// there is no message to cite and no deal to point at, so the note IS the
// evidence. A basis nobody can check is not accountability.
//
// It grants no marketing consent and could not: §7 UWG asks for express
// consent, and a card is not one. What it settles is the narrower question of
// whether an ordinary business email may be sent at all.

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// kindInPerson is an exchange that happened in a room: a card handed over at a
// trade fair, a conversation at a stand.
//
// Held by: TestRecordingAnExchangeRefusesWhatItCannotStandBehind
// (backend/internal/modules/consent/qualifyingevent_integration_test.go), whose
// first case sends a derived kind and requires the refusal.
const kindInPerson = "in_person"

// KindRequestedBySubject is the contact asking us to write to them, away from
// every system: a phone call, a conversation at a counter, a message on a
// channel this product does not capture.
//
// A SECOND HAND-RECORDED KIND RATHER THAN A WIDER in_person. The rep who took
// the call has to say what happened, and "in person" says they were in a room
// together when they were not. A reader asking later what evidence stands
// behind a send gets the answer in the word.
//
// Exported because the review's context form names it: a rep answering "they
// asked me for this" is recording exactly this kind, and one spelling of the
// word is what stops the form and the store disagreeing about what was written.
const KindRequestedBySubject = "requested_by_subject"

// handRecordedKinds are the kinds a user or their agent may assert. Every other kind is
// derived from records, and a hand-written one would be a second, unbacked
// answer to a question the data already settles.
//
// Both of these need a NOTE and carry no source row, which is the shape the
// table's evidence CHECK requires of them — there is nothing to cite, so the
// note is the evidence.
var handRecordedKinds = map[string]bool{
	kindInPerson:           true,
	KindRequestedBySubject: true,
}

// KindMeeting is the event derived from a meeting the subject was in
// (qualifyingground.go). Exported because the capture ladder reads the same
// fact — a contact we are meeting is a counterparty — and one spelling of the
// word is what stops the two answers drifting apart.
const KindMeeting = "meeting"

// KindInquiry is the event a subject writes by ASKING — a public booking form,
// and the other doors ADR-0098 D2 names beside it. Exported because the door
// that takes the inquiry is not this module, and the word has to be the same on
// both sides of that edge.
const KindInquiry = "inquiry"

// noteMaxRunes bounds what the evidence may be. A sentence saying where a card
// changed hands is the whole need; a page of prose in this column is somebody
// using it as a notes field, and the record has one of those.
const noteMaxRunes = 500

// fieldNote is the field name a refusal about the note carries, which is what
// the surface highlights.
const fieldNote = "note"

// fieldOccurredAt is the same, for the date.
const fieldOccurredAt = "occurred_at"

// clockSkewAllowance is how far ahead of this server a client's clock may be
// before a date reads as a claim about the future rather than a fast watch.
const clockSkewAllowance = 5 * time.Minute

// RecordQualifyingEventInput is one exchange, as the contact who was there
// states it.
type RecordQualifyingEventInput struct {
	Kind       string
	Note       string
	OccurredAt time.Time
	// AsksForQuote says the contact asked for a quote or an offer. Only a
	// request the subject made may say so. Only such a request bears out a
	// quote; a request to be called back does not.
	AsksForQuote bool
	// AboutDeal names the deal the request was about; zero names none. A deal
	// follow-up and a quote on a deal rest only on a request about that deal.
	AboutDeal ids.UUID
}

// RecordInquiry records that this contact asked for a meeting themselves.
//
// No principal gate, and that is the point of it: the caller is the PUBLIC
// booking door, which has no session at all. What bounds it is the fact it
// records — the door has just committed a meeting this contact booked, and the
// row cites that meeting. It asserts nothing a seat could not see for itself on
// the record.
//
// It is `captured` rather than `human`: nobody typed this claim, the door that
// took the booking wrote it. The hand-recorded verbs beside it stay a named
// user's assertion with no other evidence, made by them or the agent on their
// passport. They keep their RequireHumanOrItsAgent.
func (s *Store) RecordInquiry(
	ctx context.Context, contactID ids.ContactID, activityID ids.UUID,
) error {
	by, err := storekit.CapturedBy(ctx)
	if err != nil {
		return err
	}
	return s.db.Tx(ctx, func(tx pgx.Tx) error {
		holder, err := holdSurvivingContact(ctx, tx, contactID)
		if err != nil {
			return err
		}
		return RecordSubjectInquiry(ctx, tx, holder.String(), QualifyingEvent{
			Kind: KindInquiry,
			// WHEN THEY ASKED, which is now — not when the meeting they booked
			// is scheduled for. A booking is routinely made for a date weeks
			// out, and the verdict reads the most recent event against a reply
			// window: an inquiry dated forward would hold the contact qualified
			// from a moment that has not happened yet. The hand-recorded verb
			// below refuses a future date for the same reason and says so.
			//
			// Taken here rather than from the caller because the caller is
			// observing a fact rather than asserting one: the door knows a
			// booking just arrived, and the moment it arrived is this store's
			// to stamp.
			OccurredAt:       s.now(),
			SourceEntityType: entityActivity,
			SourceEntityID:   activityID.String(),
		}, by)
	})
}

// holdSurvivingContact locks the contact a door named, or the survivor a merge
// has since retired them into, and answers which one it holds. Held to commit,
// so a merge still to come waits for the write and carries it.
//
// One hop, as survivingSubject: a survivor later merged away has its rows
// carried by that merge.
func holdSurvivingContact(ctx context.Context, tx pgx.Tx, contactID ids.ContactID) (ids.ContactID, error) {
	err := auth.LockSubjectLive(ctx, tx, entityContact, contactID.UUID)
	if !errors.Is(err, apperrors.ErrNotFound) {
		return contactID, err
	}
	surviving, err := survivingSubject(ctx, tx, contactID.UUID)
	if err != nil {
		return ids.ContactID{}, err
	}
	if surviving == contactID.UUID {
		return ids.ContactID{}, apperrors.ErrNotFound
	}
	survivor := ids.From[ids.ContactKind](surviving)
	return survivor, auth.LockSubjectLive(ctx, tx, entityContact, survivor.UUID)
}

// RecordQualifyingEvent writes the exchange and returns it as it now stands.
//
// Gated on the SUBJECT rather than on a consent object: what this asserts is a
// fact about a contact, it changes what may be sent to them, and the authority
// to make that assertion is the authority to write their record.
func (s *Store) RecordQualifyingEvent(
	ctx context.Context,
	contactID ids.ContactID,
	in RecordQualifyingEventInput,
) (QualifyingEvent, error) {
	if !handRecordedKinds[in.Kind] {
		return QualifyingEvent{}, &InvalidQualifyingEventError{
			Field: "kind",
			Reason: "only an exchange in person or a request the subject made themselves is " +
				"recorded by hand; the rest are derived",
		}
	}
	if in.AsksForQuote && in.Kind != KindRequestedBySubject {
		return QualifyingEvent{}, &InvalidQualifyingEventError{
			Field: "asks_for_quote", Reason: "only a request the contact made can ask for a quote",
		}
	}
	note := strings.TrimSpace(in.Note)
	if note == "" {
		return QualifyingEvent{}, &InvalidQualifyingEventError{
			Field:  fieldNote,
			Reason: "a hand-recorded exchange has no other evidence, so it needs one",
		}
	}
	if len([]rune(note)) > noteMaxRunes {
		return QualifyingEvent{}, &InvalidQualifyingEventError{
			Field: fieldNote, Reason: fmt.Sprintf("keep it under %d characters", noteMaxRunes),
		}
	}
	if in.OccurredAt.IsZero() {
		return QualifyingEvent{}, &InvalidQualifyingEventError{
			Field: fieldOccurredAt, Reason: "say when it happened, not when it was typed in",
		}
	}
	// An exchange somebody remembers happened in the PAST. A future date is not
	// a memory, and the verdict reads the most recent row — so a fabricated one
	// would authorize sending now and shadow the genuine evidence under it. The
	// small skew allowance is for a client clock that runs fast, not for a
	// claim about tomorrow.
	if in.OccurredAt.After(s.now().Add(clockSkewAllowance)) {
		return QualifyingEvent{}, &InvalidQualifyingEventError{
			Field: fieldOccurredAt, Reason: "an exchange you remember happened before now",
		}
	}
	// Here as well as on the route: automatic work never asserts that a contact
	// met somebody, and an in-process caller would otherwise reach the row.
	if err := auth.RequireHumanOrItsAgent(ctx); err != nil {
		return QualifyingEvent{}, err
	}
	if err := auth.Require(ctx, "contact", principal.ActionUpdate); err != nil {
		return QualifyingEvent{}, err
	}
	by, err := storekit.CapturedBy(ctx)
	if err != nil {
		return QualifyingEvent{}, err
	}

	out := QualifyingEvent{Kind: in.Kind, Note: note, OccurredAt: in.OccurredAt, AsksForQuote: in.AsksForQuote}
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		// The row-scope probe first: this changes what may be SENT to a contact,
		// which is a change to their record however little of the row it
		// touches. Held to commit, so a merge of this contact carries the row
		// rather than committing before it lands on the retired record.
		if err := auth.HoldWritableLive(ctx, tx, "contact", contactID.UUID); err != nil {
			return err
		}
		if !in.AboutDeal.IsZero() {
			if err := auth.EnsureLinkTarget(ctx, tx, "deal", in.AboutDeal); err != nil {
				return err
			}
		}
		// The same exchange, re-sent, is one exchange. A retry — a double click,
		// a client that resends on a timeout — must not stack a second row
		// claiming a second meeting happened, because the rows ARE the legal
		// evidence and a duplicated one is a claim nobody made.
		//
		// Matched on the contact, the moment and the words: two genuinely
		// different exchanges with the same contact do not share all three.
		// A retracted row matches too, so replaying the request that recorded
		// it answers the row as it stands and does not bring it back.
		if err := tx.QueryRow(ctx, `
			INSERT INTO consent_qualifying_event
				(contact_id, kind, note, occurred_at, source, captured_by, asks_for_quote, about_deal_id)
			SELECT $1, $2, $3, $4, $7, $5, $6, $8
			 WHERE NOT EXISTS (
				SELECT 1 FROM consent_qualifying_event
				 WHERE contact_id = $1 AND kind = $2 AND note = $3 AND occurred_at = $4
				   AND about_deal_id IS NOT DISTINCT FROM $8)
			RETURNING id::text`,
			contactID, in.Kind, note, in.OccurredAt, by, in.AsksForQuote, handRecordedSource(ctx), zeroUUIDAsNull(in.AboutDeal)).Scan(&out.ID); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				// Already on file. Nothing was written twice, so there is
				// nothing to audit, and the answer is the standing row.
				return standingExchange(ctx, tx, contactID, in.AboutDeal, &out)
			}
			return fmt.Errorf("consent: recording the exchange: %w", err)
		}
		// Audit-only, like every other consent-basis write beside it: the closed
		// public-event catalog carries no type for a lawful basis, and what an
		// auditor asks — who said this happened, and what did they say — is a
		// row question rather than a subscriber's.
		//
		// The entity is `contact`, which is the row the caller was authorized
		// against and the row this changes the sendability of. Naming
		// `contact_consent` would have put a rule in the trail
		// (`contact_consent.create`) that authorized nothing here.
		_, err := storekit.AuditEvent(ctx, tx, "update", "contact", contactID.UUID, map[string]any{
			"qualifying_event": in.Kind,
			fieldNote:          note,
			fieldOccurredAt:    in.OccurredAt.UTC().Format(time.RFC3339),
			"about_deal":       zeroUUIDAsNull(in.AboutDeal),
		})
		return err
	})
	if err != nil {
		return QualifyingEvent{}, err
	}
	return out, nil
}

// standingExchange fills out with the row a retried record matched, retracted
// or not.
func standingExchange(ctx context.Context, tx pgx.Tx, contactID ids.ContactID, deal ids.UUID, out *QualifyingEvent) error {
	if err := tx.QueryRow(ctx, `
		SELECT id::text, asks_for_quote, retracted_at IS NOT NULL FROM consent_qualifying_event
		 WHERE contact_id = $1 AND kind = $2 AND note = $3 AND occurred_at = $4
		   AND about_deal_id IS NOT DISTINCT FROM $5
		 ORDER BY created_at LIMIT 1`, contactID, out.Kind, out.Note, out.OccurredAt, zeroUUIDAsNull(deal)).
		Scan(&out.ID, &out.AsksForQuote, &out.Retracted); err != nil {
		return fmt.Errorf("consent: reading the exchange already on file: %w", err)
	}
	return nil
}

// RetractQualifyingEvent withdraws an exchange recorded by hand. The row stays
// as the record of what was said and when it was taken back; no send and no
// verdict reads it any more.
//
// Gated like recording, because it is the same statement taken back. A derived exchange is not retracted here: it stands on a record the
// product holds, and archiving that record is what ends it.
func (s *Store) RetractQualifyingEvent(ctx context.Context, contactID ids.ContactID, eventID ids.UUID) error {
	if err := auth.RequireHumanOrItsAgent(ctx); err != nil {
		return err
	}
	if err := auth.Require(ctx, "contact", principal.ActionUpdate); err != nil {
		return err
	}
	by, err := storekit.CapturedBy(ctx)
	if err != nil {
		return err
	}
	return s.db.Tx(ctx, func(tx pgx.Tx) error {
		if err := auth.HoldWritableLive(ctx, tx, "contact", contactID.UUID); err != nil {
			return err
		}
		var kind string
		var occurred time.Time
		err := tx.QueryRow(ctx, `
			UPDATE consent_qualifying_event
			   SET retracted_at = now(), retracted_by = $3
			 WHERE id = $1 AND contact_id = $2 AND kind = ANY($4::text[]) AND retracted_at IS NULL
			RETURNING kind, occurred_at`, eventID, contactID, by, handRecordedKindList()).Scan(&kind, &occurred)
		if errors.Is(err, pgx.ErrNoRows) {
			return apperrors.ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("consent: withdrawing the exchange: %w", err)
		}
		// Audit-only, as recording is: the public-event catalog has no type for
		// a lawful basis, and the trail answers who withdrew what.
		_, err = storekit.AuditEvent(ctx, tx, "update", "contact", contactID.UUID, map[string]any{
			"qualifying_event_retracted": kind,
			fieldOccurredAt:              occurred.UTC().Format(time.RFC3339),
		})
		return err
	})
}

// handRecordedKindList is handRecordedKinds as a statement argument.
func handRecordedKindList() []string {
	out := make([]string, 0, len(handRecordedKinds))
	for kind := range handRecordedKinds {
		out = append(out, kind)
	}
	return out
}

// handRecordedSource is the source column of a hand-recorded exchange. An agent's
// row says so, because this row is the legal evidence and an agent was not there.
func handRecordedSource(ctx context.Context) string {
	if actor, ok := principal.Actor(ctx); ok && actor.Type == principal.PrincipalAgent {
		return sourceKindAgent
	}
	return sourceKindHuman
}

// InvalidQualifyingEventError refuses a claim the record could not stand
// behind, naming the field so the surface can point at it.
type InvalidQualifyingEventError struct {
	Field  string
	Reason string
}

func (e *InvalidQualifyingEventError) Error() string { return e.Field + ": " + e.Reason }

// FieldFault carries the verdict to every surface, as this module's other
// refusals do.
func (e *InvalidQualifyingEventError) FieldFault() (field, code, message string) {
	return e.Field, "invalid", e.Reason
}
