// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// ResolveMeetingToken expires management capabilities after the meeting retention window.
func (s *Store) ResolveMeetingToken(ctx context.Context, token string) (ids.UUID, ids.UserID, error) {
	if len(token) != 43 {
		return ids.Nil, ids.UserID{}, apperrors.ErrNotFound
	}
	var id ids.UUID
	var host ids.UserID
	err := database.WithInfraTx(ctx, s.db.Pool(), func(tx pgx.Tx) error {
		args := schedulingArgs{}
		query := `SELECT i.activity_id,i.host_user_id FROM meeting_invitation i JOIN activity a ON a.id=i.activity_id
   WHERE i.management_hash=` + args.add(storekit.TokenHash(token)) + ` AND a.archived_at IS NULL
   AND ` + args.add(s.now().Add(-30*24*time.Hour)) + `<a.occurred_at -- occurred_at raw: the thirty days a guest may manage it
`
		return tx.QueryRow(ctx, query, args...).Scan(&id, &host)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		err = apperrors.ErrNotFound
	}
	return id, host, err
}

// GetPublicSchedulingProfile returns the host-approved public identity and
// meeting policy. A paused page answers only whose page it is.
func (h Handlers) GetPublicSchedulingProfile(w http.ResponseWriter, r *http.Request, slug string, params crmcontracts.GetPublicSchedulingProfileParams) {
	page, err := h.store.ResolveBookingPage(r.Context(), slug)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	profile, open := h.store.openForBooking(r.Context(), page)
	if open.err != nil {
		writeStoreErr(w, r, open.err)
		return
	}
	if open.paused {
		httperr.WriteJSON(w, http.StatusOK, pausedPublicProfile(profile))
		return
	}
	profile = h.store.brandSchedulingProfile(r.Context(), page.HostUserID, profile)
	out := publicProfile(bookingLength(profile, params.L), nil)
	out.OfferedLengths = new(bookingLengths(profile))
	out.PrivacyInformation, err = h.bookingPrivacyInformation(r.Context())
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}

	httperr.WriteJSON(w, http.StatusOK, out)
}

// GetPublicMeetingInvitation withholds provider URLs from the guest projection.
func (h Handlers) GetPublicMeetingInvitation(w http.ResponseWriter, r *http.Request, token string) {
	id, _, err := h.store.ResolveMeetingToken(r.Context(), token)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	out, err := h.store.Invitation(r.Context(), id)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, guestInvitationView(out))
}

// ChangePublicMeetingInvitation limits guest authority to rescheduling and cancellation.
func (h Handlers) ChangePublicMeetingInvitation(w http.ResponseWriter, r *http.Request, token string) {
	id, host, err := h.store.ResolveMeetingToken(r.Context(), token)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	var in crmcontracts.MeetingInvitationChange
	if !httperr.Decode(w, r, &in) {
		return
	}
	if in.Action == "retry" {
		httperr.Write(w, r, apperrors.ErrPermissionDenied)
		return
	}
	out, err := h.store.applyInvitationChange(r.Context(), host, id, in)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusAccepted, guestInvitationView(out))
}

func (h Handlers) bookPublicInvitation(w http.ResponseWriter, r *http.Request, page BookingPage, req crmcontracts.BookPublicMeetingJSONRequestBody, contact ids.UUID, marketing MarketingOutcome, intent *publicBookingIntent, length *int) {
	out, ok := h.reservePublicInvitation(w, r, page, req, contact, invitationIntent{Public: intent, DescriptionFromBooker: true}, marketing, length)
	if !ok {
		return
	}
	if err := h.publicConsent.RecordBookingInquiry(r.Context(), contact, ids.UUID(out.Id)); err != nil {
		slog.WarnContext(r.Context(), "booking: inquiry basis could not be recorded", "activity_id", out.Id, "err", err)
	}
	writePublicInvitation(w, out, marketing)
}

// reservePublicInvitation reserves the booked slot for the contact, or for the
// lead the intent names, and writes the refusal when it cannot.
func (h Handlers) reservePublicInvitation(w http.ResponseWriter, r *http.Request, page BookingPage, req crmcontracts.BookPublicMeetingJSONRequestBody, contact ids.UUID, booking invitationIntent, marketing MarketingOutcome, length *int) (crmcontracts.MeetingInvitation, bool) {
	profile, team, ok := h.openBookingPage(w, r, page)
	if !ok {
		return crmcontracts.MeetingInvitation{}, false
	}
	profile, err := chosenLength(profile, length)
	if err != nil {
		httperr.Write(w, r, err)
		return crmcontracts.MeetingInvitation{}, false
	}
	if req.End.Sub(req.Start) != time.Duration(profile.DurationMinutes)*time.Minute {
		httperr.Write(w, r, apperrors.ErrPermissionDenied)
		return crmcontracts.MeetingInvitation{}, false
	}
	in := crmcontracts.MeetingInvitationRequest{
		ContactId: crmcontracts.Id(contact), AttendeeEmail: req.Booker.Email,
		Start: req.Start, End: req.End, Subject: profile.Title, Location: profile.Location,
	}
	if req.Subject != nil {
		in.Description = *req.Subject
	}
	if booking.Public != nil {
		booking.Public.Marketing = marketing
	}
	booking.Team, booking.Page = team, page.ID
	out, err := h.store.reserveAndQueueInvitation(r.Context(), page.HostUserID, in, booking)
	if err != nil {
		writeStoreErr(w, r, err)
		return crmcontracts.MeetingInvitation{}, false
	}
	return out, true
}

func writePublicInvitation(w http.ResponseWriter, out crmcontracts.MeetingInvitation, marketing MarketingOutcome) {
	httperr.WriteJSON(w, http.StatusCreated, struct {
		Booking    crmcontracts.MeetingInvitationStatus `json:"booking"`
		Start      time.Time                            `json:"start"`
		End        time.Time                            `json:"end"`
		Marketing  MarketingOutcome                     `json:"marketing"`
		Invitation crmcontracts.MeetingInvitation       `json:"invitation"`
	}{out.Status, out.Start, out.End, marketing, guestInvitationView(out)})
}

// openBookingPage reads the profile of a page that takes bookings now. A
// paused page answers as an unknown slug does. The slug is the capability,
// and a paused one must not confirm that it exists.
func (h Handlers) openBookingPage(w http.ResponseWriter, r *http.Request, page BookingPage) (crmcontracts.SchedulingProfile, []teamCalendar, bool) {
	profile, open := h.store.openForBooking(r.Context(), page)
	if open.err != nil {
		writeStoreErr(w, r, open.err)
		return profile, nil, false
	}
	if open.paused {
		httperr.Write(w, r, apperrors.ErrNotFound)
		return profile, nil, false
	}
	return profile, open.team, true
}

// pageOpenness is whether a page takes bookings, or why that is unknown. An
// open page carries the team it was opened with.
type pageOpenness struct {
	paused bool
	team   []teamCalendar
	err    error
}

func (s *Store) openForBooking(ctx context.Context, page BookingPage) (crmcontracts.SchedulingProfile, pageOpenness) {
	profile, err := s.bookingPageProfile(ctx, page)
	if err != nil {
		return profile, pageOpenness{err: err}
	}
	paused, team, err := s.pagePaused(ctx, page.HostUserID, profile)
	return profile, pageOpenness{paused: paused, team: team, err: err}
}
