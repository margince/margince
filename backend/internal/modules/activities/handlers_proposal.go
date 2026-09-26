// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"log/slog"
	"net/http"
	"strings"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// CreateMeetingProposal binds offered times to one contact before returning a one-use link.
func (h Handlers) CreateMeetingProposal(w http.ResponseWriter, r *http.Request, _ crmcontracts.CreateMeetingProposalParams) {
	var in crmcontracts.MeetingProposalRequest
	if !httperr.Decode(w, r, &in) {
		return
	}
	out, err := h.store.CreateProposal(r.Context(), in)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusCreated, out)
}

func (h Handlers) proposalForRequest(w http.ResponseWriter, r *http.Request, token string) (proposalRow, bool) {
	id, _, err := h.store.ResolveProposalToken(r.Context(), token)
	if err != nil {
		writeStoreErr(w, r, err)
		return proposalRow{}, false
	}
	row, err := h.store.readProposal(r.Context(), id)
	if err != nil {
		writeStoreErr(w, r, err)
		return row, false
	}
	return row, true
}

func publicProfile(profile crmcontracts.SchedulingProfile) crmcontracts.PublicSchedulingProfile {
	out := crmcontracts.PublicSchedulingProfile{Title: profile.Title, Location: profile.Location, DurationMinutes: profile.DurationMinutes, Enabled: profile.Enabled, LogoUrl: profile.LogoUrl}
	if profile.HostName != nil {
		out.HostName = *profile.HostName
	}
	if profile.CompanyName != nil {
		out.CompanyName = *profile.CompanyName
	}
	return out
}

// GetPublicMeetingProposal discloses only the recipient-safe proposal projection.
func (h Handlers) GetPublicMeetingProposal(w http.ResponseWriter, r *http.Request, token string) {
	row, ok := h.proposalForRequest(w, r, token)
	if !ok {
		return
	}
	profile, err := h.store.hostSchedulingProfile(r.Context(), row.Host)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	out := crmcontracts.PublicMeetingProposal{Profile: publicProfile(profile), Description: row.Request.Description, ExpiresAt: row.Expires, Used: row.Used != nil, Options: row.Request.Options}
	out.Profile.Title = row.Request.Subject
	out.Profile.Location = row.Request.Location
	out.Profile.DurationMinutes = row.Request.DurationMinutes
	out.Profile.Enabled = row.Used == nil
	if row.Invitation != nil {
		meeting, err := h.store.recoverProposalInvitation(r.Context(), row)
		if err != nil {
			writeStoreErr(w, r, err)
			return
		}
		out.Meeting = &meeting
	}
	httperr.WriteJSON(w, http.StatusOK, out)
}

// GetPublicProposalAvailability checks live occupancy for the capability-bound host.
func (h Handlers) GetPublicProposalAvailability(w http.ResponseWriter, r *http.Request, token string, params crmcontracts.GetPublicProposalAvailabilityParams) {
	row, ok := h.proposalForRequest(w, r, token)
	if !ok {
		return
	}
	if row.Used != nil {
		httperr.Write(w, r, apperrors.ErrConflict)
		return
	}
	slots, truncated, err := h.store.ReliableAvailability(r.Context(), row.Host, params.From, params.To, time.Duration(row.Request.DurationMinutes)*time.Minute)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, struct {
		Slots     []slot `json:"slots"`
		Truncated bool   `json:"truncated"`
	}{slots, truncated})
}

// AcceptPublicMeetingProposal consumes the recipient capability with the reservation.
func (h Handlers) AcceptPublicMeetingProposal(w http.ResponseWriter, r *http.Request, token string) {
	row, ok := h.proposalForRequest(w, r, token)
	if !ok {
		return
	}
	var req crmcontracts.AcceptPublicMeetingProposalJSONRequestBody
	if !httperr.Decode(w, r, &req) {
		return
	}
	if row.Used != nil {
		httperr.Write(w, r, apperrors.ErrConflict)
		return
	}
	if req.End.Sub(req.Start) != time.Duration(row.Request.DurationMinutes)*time.Minute {
		httperr.Write(w, r, httperr.Validation("end", "duration", "Choose the offered meeting duration"))
		return
	}
	if strings.TrimSpace(req.Consent.Wording) == "" || strings.TrimSpace(req.Consent.PolicyVersion) == "" {
		httperr.Write(w, r, httperr.Validation("consent", "required", "Confirm the booking terms"))
		return
	}
	if h.publicConsent == nil {
		httperr.Write(w, r, apperrors.ErrPermissionDenied)
		return
	}
	if _, err := h.store.validateInvitationTime(r.Context(), row.Host, req.Start, req.End); err != nil {
		writeStoreErr(w, r, err)
		return
	}
	purpose, err := h.publicConsent.ScopedPurpose(r.Context(), nil)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	if err := h.store.calendar.CheckRecipient(r.Context(), row.Host, ids.UUID(row.Request.ContactId), string(row.Request.AttendeeEmail)); err != nil {
		writeStoreErr(w, r, err)
		return
	}
	if _, err := h.publicConsent.CaptureBookingConsent(r.Context(), ids.UUID(row.Request.ContactId), BookingConsent{PurposeID: purpose, Wording: req.Consent.Wording, PolicyVersion: req.Consent.PolicyVersion}); err != nil {
		writeStoreErr(w, r, err)
		return
	}
	in := crmcontracts.MeetingInvitationRequest{ContactId: row.Request.ContactId, AttendeeEmail: row.Request.AttendeeEmail, Subject: row.Request.Subject, Description: row.Request.Description, Location: row.Request.Location, Start: req.Start, End: req.End}
	out, err := h.store.reserveAndQueueInvitation(r.Context(), row.Host, in, invitationIntent{ProposalID: row.ID})
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	if err := h.publicConsent.RecordBookingInquiry(r.Context(), ids.UUID(in.ContactId), ids.UUID(out.Id)); err != nil {
		slog.WarnContext(r.Context(), "booking: inquiry basis could not be recorded", "activity_id", out.Id, "err", err)
	}
	httperr.WriteJSON(w, http.StatusAccepted, out)
}
