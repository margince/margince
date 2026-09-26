// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// ResolveMeetingToken expires management capabilities after the meeting retention window.
func (s *Store) ResolveMeetingToken(ctx context.Context, token string) (ids.UUID, ids.UserID, error) {
	if len(token) != 43 {
		return ids.Nil, ids.UserID{}, apperrors.ErrNotFound
	}
	sum := sha256.Sum256([]byte(token))
	var id ids.UUID
	var host ids.UserID
	err := database.WithInfraTx(ctx, s.db.Pool(), func(tx pgx.Tx) error {
		args := schedulingArgs{}
		query := `SELECT i.activity_id,i.host_user_id FROM meeting_invitation i JOIN activity a ON a.id=i.activity_id
   WHERE i.management_hash=` + args.add(hex.EncodeToString(sum[:])) + ` AND a.archived_at IS NULL
   AND a.occurred_at>` + args.add(s.now().Add(-30*24*time.Hour))
		return tx.QueryRow(ctx, query, args...).Scan(&id, &host)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		err = apperrors.ErrNotFound
	}
	return id, host, err
}

// GetPublicSchedulingProfile returns the host-approved public identity and meeting policy.
func (h Handlers) GetPublicSchedulingProfile(w http.ResponseWriter, r *http.Request, slug string) {
	page, err := h.store.ResolveBookingPage(r.Context(), slug)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	profile, err := h.store.hostSchedulingProfile(r.Context(), page.HostUserID)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	out := crmcontracts.PublicSchedulingProfile{
		Title: profile.Title, Location: profile.Location,
		DurationMinutes: profile.DurationMinutes, Enabled: profile.Enabled,
	}
	if profile.HostName != nil {
		out.HostName = *profile.HostName
	}
	if profile.CompanyName != nil {
		out.CompanyName = *profile.CompanyName
	}
	out.LogoUrl = profile.LogoUrl
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
	out.CalendarUrl = nil
	httperr.WriteJSON(w, http.StatusOK, out)
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
	out.CalendarUrl = nil
	httperr.WriteJSON(w, http.StatusAccepted, out)
}

func (h Handlers) bookPublicInvitation(w http.ResponseWriter, r *http.Request, page BookingPage, req crmcontracts.BookPublicMeetingJSONRequestBody, contact ids.UUID, marketing MarketingOutcome, intent *publicBookingIntent) {
	profile, err := h.store.hostSchedulingProfile(r.Context(), page.HostUserID)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	if !profile.Enabled || req.End.Sub(req.Start) != time.Duration(profile.DurationMinutes)*time.Minute {
		httperr.Write(w, r, apperrors.ErrPermissionDenied)
		return
	}
	in := crmcontracts.MeetingInvitationRequest{
		ContactId: crmcontracts.Id(contact), AttendeeEmail: req.Booker.Email,
		Start: req.Start, End: req.End, Subject: profile.Title, Location: profile.Location,
	}
	if req.Subject != nil {
		in.Description = *req.Subject
	}
	if intent != nil {
		intent.Marketing = marketing
	}
	out, err := h.store.reserveAndQueueInvitation(r.Context(), page.HostUserID, in, invitationIntent{Public: intent})
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	if err := h.publicConsent.RecordBookingInquiry(r.Context(), contact, ids.UUID(out.Id)); err != nil {
		slog.WarnContext(r.Context(), "booking: inquiry basis could not be recorded", "activity_id", out.Id, "err", err)
	}
	writePublicInvitation(w, out, marketing)
}

func writePublicInvitation(w http.ResponseWriter, out crmcontracts.MeetingInvitation, marketing MarketingOutcome) {
	httperr.WriteJSON(w, http.StatusCreated, struct {
		Booking    crmcontracts.MeetingInvitationStatus `json:"booking"`
		Start      time.Time                            `json:"start"`
		End        time.Time                            `json:"end"`
		Marketing  MarketingOutcome                     `json:"marketing"`
		Invitation crmcontracts.MeetingInvitation       `json:"invitation"`
	}{out.Status, out.Start, out.End, marketing, out})
}
