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

// ValidatePurpose confirms the purpose exists AND is the booking-scoped
// `transactional` purpose BEFORE the surface writes anything — a public
// capture may not create a person it cannot attach a recordable consent
// to, and may not reach beyond its own consent lane.
func (a bookingConsentAdapter) ValidatePurpose(ctx context.Context, purposeID ids.UUID) error {
	purposes, err := a.store.ListPurposes(ctx)
	if err != nil {
		return err
	}
	return admitBookingPurpose(purposes, purposeID)
}

// admitBookingPurpose is the pure admission decision: the id must resolve
// within the tracked catalog and be the booking-scoped purpose. An
// unknown id and an out-of-scope purpose are both a 422 — neither leaks
// which of the two it was beyond what the caller already supplied.
func admitBookingPurpose(purposes []consent.Purpose, purposeID ids.UUID) error {
	for _, p := range purposes {
		if p.ID.UUID == purposeID {
			if p.Key != bookingScopedPurposeKey {
				return httperr.Validation("consent.purpose_id", "invalid",
					"public booking may only record consent for the transactional purpose")
			}
			return nil
		}
	}
	return httperr.Validation("consent.purpose_id", "invalid", "not a tracked consent purpose")
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
// not that check repeated for its own sake: it runs before EnsurePersonByEmail
// commits a person row, so a form naming a bad purpose refuses without leaving
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

func (a bookingConsentAdapter) CaptureBookingConsent(ctx context.Context, personID ids.UUID, c activities.BookingConsent) error {
	source := "public_booking"
	_, err := a.store.Record(ctx, consent.RecordInput{
		PersonID:      ids.From[ids.PersonKind](personID),
		PurposeID:     ids.From[ids.PurposeKind](c.PurposeID),
		NewState:      "granted",
		Source:        &source,
		PolicyText:    c.Wording,
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
		return httperr.Validation(invalid.Field, "invalid", invalid.Reason)
	}
	if err != nil {
		return err
	}
	return a.askMarketing(ctx, personID, c.Marketing)
}

// askMarketing mails the confirmation link an affirmative tick earns, and it
// runs AFTER the operational grant on purpose: the meeting the subject actually
// booked must not fail because a newsletter question could not be asked.
//
// What it mails is a question, not a subscription. IssueConsentLink mints
// against the person's OWN live primary address — read from the record, never
// taken from the request — so the mail cannot be aimed at a third party, and
// the grant that follows exists only if that mailbox answers.
//
// TickedFrom is passed so the mint can refuse a mismatch. A booking email
// resolves to any existing person holding it, including on a NON-primary
// address, and the link would then go to that person's primary — asking them to
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
func (a bookingConsentAdapter) askMarketing(ctx context.Context, personID ids.UUID, m *activities.BookingMarketing) error {
	if m == nil {
		return nil
	}
	_, err := a.store.IssueConsentLink(ctx,
		ids.From[ids.PersonKind](personID), ids.From[ids.PurposeKind](m.PurposeID), m.TickedFrom)
	var invalid *consent.ValidationError
	if errors.As(err, &invalid) {
		return httperr.Validation(invalid.Field, "invalid", invalid.Reason)
	}
	return err
}
