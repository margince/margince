// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// Taking one of the slots the availability read offered.
//
// Split from that read because the two answer opposite questions about the
// same diary: one reports what is free and promises nothing, and this one
// writes and must refuse a slot somebody else has taken since.

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// BookMeetingInput is one slot a caller is committing: whose calendar it lands
// on, when, what it is called, and the records it is about.
type BookMeetingInput struct {
	Host           ids.UserID
	Start          time.Time
	End            time.Time
	Subject        string
	AttendeeEmails []string
	Links          []ActivityLinkInput
	// Source names the capture surface ("manual" when empty — the
	// authenticated default). The anonymous page passes public_booking
	// so a stranger's submission never masquerades as hand-entered data.
	Source string
}

// BookMeeting commits one slot: the meeting lands as an activity on the
// linked records' timelines, and a taken slot answers slot_taken
// instead of double-booking the host.
func (s *Store) BookMeeting(ctx context.Context, in BookMeetingInput) (crmcontracts.Activity, error) {
	if err := auth.Require(ctx, "activity", principal.ActionCreate); err != nil {
		return crmcontracts.Activity{}, err
	}
	// Booking writes onto the host's calendar, so a caller may commit their OWN
	// slots and only an admin may book on behalf of another host. A
	// per-seat delegation grant would be the finer answer and this build has
	// none, so the admin role carries it.
	//
	// The admin ROLE, not an unbounded row scope: ops, read_only and management
	// all read every row, and none of them is thereby a calendar delegate for
	// everyone in the company.
	actor, ok := principal.Actor(ctx)
	if !ok {
		return crmcontracts.Activity{}, apperrors.ErrPermissionDenied
	}
	if in.Host.UUID != actor.UserID {
		if err := auth.RequireAdmin(ctx); err != nil {
			return crmcontracts.Activity{}, err
		}
	}
	if !in.End.After(in.Start) {
		return crmcontracts.Activity{}, errBookingEndNotAfterStart
	}
	if len(in.Links) == 0 {
		return crmcontracts.Activity{}, errBookingLinksEmpty
	}
	// The conflict probe reads only the calendar the caller may write
	// (their own, or any as admin — gated above) and gives the polite
	// answer; the GUARANTEE is the activity_meeting_no_overlap exclusion
	// constraint (0032) — two racing bookings cannot both commit, the
	// loser's 23P01 maps to the same slot_taken below.
	var taken bool
	err := s.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT EXISTS (
			  SELECT 1 FROM activity
			  WHERE kind = 'meeting' AND archived_at IS NULL AND host_user_id = $1
			    AND occurred_at < $3 AND occurred_at + $4::interval > $2)`,
			in.Host, in.Start, in.End, assumedMeetingDuration.String()).Scan(&taken)
	})
	if err != nil {
		return crmcontracts.Activity{}, err
	}
	if taken {
		return crmcontracts.Activity{}, &SlotTakenError{Start: in.Start}
	}

	subject := in.Subject
	if subject == "" {
		subject = "Meeting"
	}
	source := in.Source
	if source == "" {
		source = sourceManual
	}
	occurred := in.Start
	// Booking a meeting is what `booked` MEANS, and this is the door most
	// meetings arrive through. Leaving the status NULL here left it saying
	// nothing about what had just happened, so "how many did we book this
	// week" answered zero while the calendar filled up.
	booked := string(crmcontracts.ActivityMeetingStatusBooked)
	activity, _, err := s.LogActivity(ctx, LogActivityInput{
		Kind:          "meeting",
		Subject:       &subject,
		OccurredAt:    &occurred,
		HostUserID:    &in.Host,
		MeetingStatus: &booked,
		// This door takes the slot, which is what the overlap guard refuses a
		// second of. Both booking doors arrive here; nothing else sets it.
		ClaimsHostSlot: true,
		Links:          in.Links,
		Source:         source,
	})
	if _, excluded := storekit.ExclusionViolation(err); excluded {
		return crmcontracts.Activity{}, &SlotTakenError{Start: in.Start}
	}
	// NO INVITE IS SENT, and in.AttendeeEmails is not carried anywhere — not
	// onto the activity, not into a queue, not to a transport. This build has
	// none: the calendar integrations are capture-only, and platform/mailer is
	// the password-reset seam.
	//
	// The comment that stood here said delivery "rides the deployment's
	// calendar/mail seam", which read as configuration and was not: no
	// deployment of this build can make it send. The screen and the contract
	// both said an invite was on its way, so a rep watched a client never hear
	// about a meeting with nothing anywhere reporting it.
	//
	// Both now say who has to tell the attendee. Wiring a real transport is a
	// feature and needs one to exist first.
	return activity, err
}

// SlotTakenError maps to the contract's 409 slot_taken.
type SlotTakenError struct{ Start time.Time }

func (e *SlotTakenError) Error() string {
	return fmt.Sprintf("the host is already booked around %s", e.Start.Format(time.RFC3339))
}

// BookMeeting is the REST door onto the store's booking, answering 409 when the
// host was taken between the availability read and this write — which is the
// whole reason the read promises nothing.
func (h Handlers) BookMeeting(w http.ResponseWriter, r *http.Request, _ crmcontracts.BookMeetingParams) {
	var req crmcontracts.BookMeetingJSONRequestBody
	if !httperr.Decode(w, r, &req) {
		return
	}
	actor, ok := principal.Actor(r.Context())
	if !ok {
		httperr.Unauthorized(w, r, "booking needs an authenticated caller")
		return
	}
	in := BookMeetingInput{
		Host:  ids.From[ids.UserKind](actor.UserID),
		Start: req.Start,
		End:   req.End,
	}
	if req.Subject != nil {
		in.Subject = *req.Subject
	}
	if req.AttendeeEmails != nil {
		for _, e := range *req.AttendeeEmails {
			in.AttendeeEmails = append(in.AttendeeEmails, string(e))
		}
	}
	if req.HostUserId != nil {
		in.Host = ids.From[ids.UserKind](ids.UUID(*req.HostUserId))
	}
	for _, l := range req.Links {
		in.Links = append(in.Links, ActivityLinkInput{EntityType: string(l.EntityType), EntityID: ids.UUID(l.EntityId)})
	}

	// The optional CaptureConsent passthrough records the booked subject's
	// consent BEFORE the slot commits, on the same seam the anonymous
	// booking page rides — and with the same stance: a slot_taken 409
	// after the grant leaves the grant standing, because the subject DID
	// give it. Recording it is mandatory once the field is present; a
	// process role composed without the consent seam refuses rather than
	// booking with an unrecorded consent.
	if !h.captureBookingConsent(w, r, req.Consent, in.Links) {
		return
	}

	booked, err := h.store.BookMeeting(r.Context(), in)
	if err != nil {
		var slotTaken *SlotTakenError
		if errors.As(err, &slotTaken) {
			httperr.Write(w, r, httperr.Duplicate("slot_taken", ""))
			return
		}
		writeStoreErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusCreated, booked)
}

// consentSubjectLink resolves which linked record the CaptureConsent
// passthrough attaches to: exactly one linked contact. The ConsentCapturer
// seam is contact-keyed (the compose adapter records against a contact), so
// a lead-only booking cannot carry consent through this endpoint yet —
// that refuses loudly rather than accepting a consent it would not
// record. Returns false after writing the response.
func consentSubjectLink(w http.ResponseWriter, r *http.Request, links []ActivityLinkInput) (ids.UUID, bool) {
	var contacts []ids.UUID
	for _, l := range links {
		if l.EntityType == "contact" {
			contacts = append(contacts, l.EntityID)
		}
	}
	switch len(contacts) {
	case 1:
		return contacts[0], true
	case 0:
		httperr.Write(w, r, httperr.Validation("consent", "subject_required",
			"recording consent with a booking requires a linked contact to attach it to"))
		return ids.UUID{}, false
	default:
		httperr.Write(w, r, httperr.Validation("consent", "subject_ambiguous",
			"recording consent with a booking requires exactly one linked contact"))
		return ids.UUID{}, false
	}
}
