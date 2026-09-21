// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// bookingConsentAdapter satisfies activities.ConsentCapturer over the
// consent module — the cross-module edge of the public booking capture
// path, injected here so activities never imports its sibling. The
// grant rides the normal consent write shape (proof row + audit +
// consent.changed), carrying the CaptureConsent passthrough verbatim.

import (
	"context"
	"errors"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

type bookingConsentAdapter struct {
	store *consent.Store
}

// bookingScopedPurposeKey is the ONE purpose an anonymous public booking
// form may assert consent for: the always-seeded `transactional` lane
// (the lawful channel for operational mail about the meeting). Admitting
// any tracked purpose let an unauthenticated caller who knows only a
// victim's email plant an effective first-party grant under an operator's
// non-DOI marketing purpose — forged consent authorizing outbound sends
// the address's owner never opted into. The edge is scoped to its own
// purpose so a stranger's submission can only ever touch the transactional
// lane, never escalate into a marketing grant.
const bookingScopedPurposeKey = "transactional"

// ScopedPurpose answers WHICH purpose this booking's grant lands on, BEFORE
// the surface writes anything — a public capture may not create a contact it
// cannot attach a recordable consent to, and may not reach beyond its own
// consent lane.
//
// It answers the purpose rather than merely admitting one so that the id the
// handler writes is the id this decision reached. Validating an id and then
// carrying the caller's own value forward were two statements that had to
// agree; now there is one.
func (a bookingConsentAdapter) ScopedPurpose(ctx context.Context, requested *ids.UUID) (ids.UUID, error) {
	purposes, err := a.store.ListPurposes(ctx)
	if err != nil {
		return ids.Nil, err
	}
	return admitBookingPurpose(purposes, requested)
}

// admitBookingPurpose is the pure decision.
//
// A caller that names NO purpose gets the booking lane's own id, looked up by
// key. That is the answer a published page needs and not a convenience: purpose
// ids are per-installation uuids minted at seed time, the contract exposes no
// anonymous read of them, and the door admits exactly one purpose anyway — so
// an anonymous form that names an id is naming a value nobody ever gave it, and
// a form shipped with a stand-in refuses on every installation there is.
//
// A caller that DOES name one still has to have named the lane. An unknown id
// and an out-of-scope purpose are both a 422 — neither leaks which of the two
// it was beyond what the caller already supplied.
func admitBookingPurpose(purposes []consent.Purpose, requested *ids.UUID) (ids.UUID, error) {
	if requested == nil {
		for _, p := range purposes {
			if p.Key == bookingScopedPurposeKey {
				return p.ID.UUID, nil
			}
		}
		// Said plainly, because an operator whose catalog is missing the lane
		// can act on it and no subject data is disclosed by saying so.
		return ids.Nil, httperr.Validation("consent.purpose_id", "invalid",
			"this installation tracks no transactional consent purpose for a booking to record against")
	}
	for _, p := range purposes {
		if p.ID.UUID == *requested {
			if p.Key != bookingScopedPurposeKey {
				return ids.Nil, httperr.Validation("consent.purpose_id", "invalid",
					"public booking may only record consent for the transactional purpose")
			}
			return p.ID.UUID, nil
		}
	}
	return ids.Nil, httperr.Validation("consent.purpose_id", "invalid", "not a tracked consent purpose")
}

// ValidateMarketingPurpose admits the purpose a booking form's marketing tick
// may ask about, BEFORE the surface writes anything.
//
// The admission is the OPPOSITE of the operational one beside it, and
// deliberately so. The booking lane is confined to one named purpose because a
// grant lands there immediately and a stranger must not choose which purpose
// they grant. The marketing tick lands NO grant: it mails a confirmation link,
// and the only thing that turns it into consent is the address's own holder
// spending it. So the question the form may ask is bounded by a PROPERTY —
// the purpose must require double opt-in — rather than by a single key, and
// an installation may offer more than one newsletter.
//
// requires_double_opt_in is what makes that safe. It is the flag that makes
// consentproof.go refuse a grant with no mailbox behind it, so a purpose
// carrying it cannot be granted by this path even if everything else here were
// wrong. A purpose without it would be grantable on assertion alone, which is
// exactly what an anonymous door may not reach.
//
// The store re-checks the same property inside the minting transaction. This is
// not that check repeated for its own sake: it runs before EnsureContactByEmail
// commits a contact row, so a form naming a bad purpose refuses without leaving
// one behind.
func (a bookingConsentAdapter) ValidateMarketingPurpose(ctx context.Context, purposeID ids.UUID) error {
	purposes, err := a.store.ListPurposes(ctx)
	if err != nil {
		return err
	}
	return admitBookingMarketingPurpose(purposes, purposeID)
}

// admitBookingMarketingPurpose is the pure admission decision. An unknown id
// and a purpose that does not require double opt-in are both a 422; the second
// says why, because an operator who pointed their form at the wrong purpose can
// act on that and no subject data is disclosed by saying it.
func admitBookingMarketingPurpose(purposes []consent.Purpose, purposeID ids.UUID) error {
	for _, p := range purposes {
		if p.ID.UUID == purposeID {
			if !p.RequiresDoubleOptIn {
				return httperr.Validation("consent.marketing.purpose_id", "invalid",
					"a booking form may only ask about a purpose confirmed by double opt-in")
			}
			return nil
		}
	}
	return httperr.Validation("consent.marketing.purpose_id", "invalid", "not a tracked consent purpose")
}

func (a bookingConsentAdapter) CaptureBookingConsent(ctx context.Context, contactID ids.UUID, c activities.BookingConsent) (activities.MarketingOutcome, error) {
	source := "public_booking"
	_, err := a.store.Record(ctx, consent.RecordInput{
		ContactID:     ids.From[ids.ContactKind](contactID),
		PurposeID:     ids.From[ids.PurposeKind](c.PurposeID),
		NewState:      "granted",
		Source:        &source,
		PolicyText:    &c.Wording,
		PolicyVersion: &c.PolicyVersion,
		// Anyone knowing an email can post this form: a decision already
		// on record — above all a withdrawal — must stand.
		NeverOverrideExisting: true,
	})
	// The consent module's client-fault type is its own; the booking
	// transport only knows the platform vocabulary — translate here so a
	// bad DOI token reads as the 422 it is, not a 500.
	var invalid *consent.ValidationError
	if errors.As(err, &invalid) {
		return activities.MarketingNotRequested, httperr.Validation(invalid.Field, "invalid", invalid.Reason)
	}
	if err != nil {
		return activities.MarketingNotRequested, err
	}
	outcome, err := a.askMarketing(ctx, contactID, c.Marketing)
	return outcome, err
}

// RecordBookingInquiry stamps the qualifying event a public booking IS: the
// subject asked for a meeting, which ADR-0098 D2 counts as them initiating
// correspondence exactly as an inbound message does.
//
// The cross-module edge, here for the reason every other one in this file is:
// `activities` owns the booking door and `consent` owns the basis, and neither
// imports the other.
//
// Its own transaction. The booking is already committed by the time this runs —
// it has to be, since the row cites the meeting as its evidence — so there is no
// transaction left to join, and nothing here may take the meeting back.
func (a bookingConsentAdapter) RecordBookingInquiry(ctx context.Context, contactID, activityID ids.UUID) error {
	return a.store.RecordInquiry(ctx, ids.From[ids.ContactKind](contactID), activityID)
}

// askMarketing mails the confirmation link an affirmative tick earns, and it
// runs AFTER the operational grant on purpose: the meeting the subject actually
// booked must not fail because a newsletter question could not be asked.
//
// What it mails is a question, not a subscription. IssueConsentLink mints
// against the contact's OWN live primary address — read from the record, never
// taken from the request — so the mail cannot be aimed at a third party, and
// the grant that follows exists only if that mailbox answers.
//
// TickedFrom is passed so the mint can refuse a mismatch. A booking email
// resolves to any existing contact holding it, including on a NON-primary
// address, and the link would then go to that contact's primary — asking them to
// confirm a subscription requested from an address they did not use.
//
// The wording and version the subject was shown on the FORM reach no row, and
// that is a real gap rather than a design choice. The grant is recorded later by
// recordLinkedPurposeTx, which sets PolicyText from the confirmation page's own
// wording and sets NO PolicyVersion at all. So the tick collects a version, this
// door refuses a tick that omits it, and nothing stores it.
//
// They are collected anyway because the refusal is the honest one to keep: a
// form asking for a subscription must be able to say what it showed, and an
// installation that has not wired that text has a consent problem whether or
// not this code reads it. Carrying both through to the grant needs columns on
// confirm_token, which is its own change.
func (a bookingConsentAdapter) askMarketing(ctx context.Context, contactID ids.UUID,
	m *activities.BookingMarketing,
) (activities.MarketingOutcome, error) {
	if m == nil {
		return activities.MarketingNotRequested, nil
	}
	issued, err := a.store.IssueConsentLink(ctx,
		ids.From[ids.ContactKind](contactID), ids.From[ids.PurposeKind](m.PurposeID), m.TickedFrom)
	if err == nil {
		// STAGED, not merely minted. issueLink answers a token it could not
		// send rather than failing — an installation with no relay gets a link
		// it can see was not posted — so a nil error alone says the row exists,
		// never that anybody will receive it.
		//
		// This adapter builds its own consent store, and the confirmation lane
		// is rewired onto the handlers' store rather than that one. So on the
		// production wiring today the mail is never staged, and reporting the
		// nil error as pending_confirmation would tell every booker a question
		// is coming when none is.
		if !issued.Staged {
			return activities.MarketingNotAsked, nil
		}
		return activities.MarketingPendingConfirmation, nil
	}
	// NO REFUSAL OF THE QUESTION COSTS THE MEETING, whatever the reason.
	//
	// The tick was optional and the meeting was not: the subject can be asked
	// again from the preference centre or the next mail, and the slot they
	// booked cannot be handed back. Reporting not_asked is what the response
	// field exists for, so a booker who ticked the box is not left waiting for
	// a mail that is not coming.
	//
	// THE TWO NAMED REFUSALS ARE STILL ENFORCED, and they are enforced where
	// they matter: inside the mint, which writes no token and stages no mail.
	// A misdirected link would have gone to a mailbox other than the one that
	// asked; a re-solicitation would have gone to somebody who explicitly
	// withdrew. Neither is sent either way. What used to happen as well was
	// that the booking died with them — a visitor who typed the second address
	// they hold, or who unsubscribed from the newsletter last year, lost the
	// meeting they came for over a checkbox they could have left empty, and
	// the transactional grant recorded a moment earlier then stood for a
	// booking that did not exist. Refusing to ask does not require refusing
	// the meeting, and a booking form is not a way around a withdrawal when
	// nothing is mailed and nothing is granted.
	//
	// The refusals keep their FieldFault types for the operator's own door:
	// handlers.go's confirm-link verb raises them as the 422 naming the field,
	// where a seat pressing the button HAS made a mistake it can act on.
	// Answering the same to an anonymous booking form would be an oracle
	// besides — it would say which addresses a contact holds and what they
	// have withdrawn, to anyone who can guess an email.
	//
	// AN AUTHORIZATION REFUSAL IS THE ONE EXCEPTION, and it is not a refusal of
	// the question at all: it says something about the CALLER rather than about
	// this installation's ability to ask. The mint takes auth.Require before it
	// opens a transaction, and the contact probe inside it can answer a denial
	// or a not-found for a subject the caller may not write.
	//
	// In the booking path the operational grant one call earlier runs
	// EnsureWritableLive on the same contact and is fatal, so a refusal here
	// means authority changed between two consecutive writes. That is rare and
	// it is exactly why it must not be reported as "we could not ask": a
	// booking form is not the place to discover a permission failure silently.
	if errors.Is(err, apperrors.ErrPermissionDenied) || errors.Is(err, apperrors.ErrNotFound) {
		return activities.MarketingNotRequested, err
	}
	return activities.MarketingNotAsked, nil
}
