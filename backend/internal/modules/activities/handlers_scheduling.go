// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"net/http"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// GetSchedulingProfile returns only the acting host’s booking configuration.
func (h Handlers) GetSchedulingProfile(w http.ResponseWriter, r *http.Request) {
	profile, err := h.store.SchedulingProfile(r.Context())
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, profile)
}

// PutSchedulingProfile validates calendar access before enabling the public link.
func (h Handlers) PutSchedulingProfile(w http.ResponseWriter, r *http.Request) {
	var req crmcontracts.SchedulingProfile
	if !httperr.Decode(w, r, &req) {
		return
	}
	profile, err := h.store.SaveSchedulingProfile(r.Context(), req)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, profile)
}

// CreateMeetingInvitation queues delivery through the shared booking engine.
func (h Handlers) CreateMeetingInvitation(w http.ResponseWriter, r *http.Request, _ crmcontracts.CreateMeetingInvitationParams) {
	var req crmcontracts.MeetingInvitationRequest
	if !httperr.Decode(w, r, &req) {
		return
	}
	out, err := h.store.CreateInvitation(r.Context(), req)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	out.ManagementToken = nil
	httperr.WriteJSON(w, http.StatusAccepted, out)
}

// GetMeetingInvitation applies activity visibility before returning delivery status.
func (h Handlers) GetMeetingInvitation(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	out, err := h.store.Invitation(r.Context(), ids.UUID(id))
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, out)
}

// ChangeMeetingInvitation uses version checks to prevent stale lifecycle changes.
func (h Handlers) ChangeMeetingInvitation(w http.ResponseWriter, r *http.Request, id crmcontracts.Id) {
	var req crmcontracts.MeetingInvitationChange
	if !httperr.Decode(w, r, &req) {
		return
	}
	out, err := h.store.ChangeInvitation(r.Context(), ids.UUID(id), req)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusAccepted, out)
}

// GetMeetingAvailability excludes the invitation itself from rescheduling occupancy.
func (h Handlers) GetMeetingAvailability(w http.ResponseWriter, r *http.Request, id crmcontracts.Id, params crmcontracts.GetMeetingAvailabilityParams) {
	host, err := schedulingHost(r.Context())
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	h.writeMeetingAvailability(w, r, host, ids.UUID(id), params.From, params.To)
}

// GetPublicMeetingAvailability offers alternatives only through a valid management capability.
func (h Handlers) GetPublicMeetingAvailability(w http.ResponseWriter, r *http.Request, token string, params crmcontracts.GetPublicMeetingAvailabilityParams) {
	id, host, err := h.store.ResolveMeetingToken(r.Context(), token)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	h.writeMeetingAvailability(w, r, host, id, params.From, params.To)
}

func (h Handlers) writeMeetingAvailability(w http.ResponseWriter, r *http.Request, host ids.UserID, id ids.UUID, from, to time.Time) {
	slots, truncated, err := h.store.invitationAvailability(r.Context(), host, id, from, to)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, struct {
		Slots     []slot `json:"slots"`
		Truncated bool   `json:"truncated"`
	}{slots, truncated})
}

// GetSchedulingCalendars lists calendars available on the acting host’s connection.
func (h Handlers) GetSchedulingCalendars(w http.ResponseWriter, r *http.Request, params crmcontracts.GetSchedulingCalendarsParams) {
	host, err := schedulingHost(r.Context())
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	if h.store.calendar == nil {
		writeStoreErr(w, r, apperrors.ErrPermissionDenied)
		return
	}
	calendars, err := h.store.calendar.List(r.Context(), host, string(params.Provider))
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, calendars)
}
