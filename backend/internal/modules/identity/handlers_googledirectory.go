// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

// The transport slice for the Google directory credential:
// /installation/authentication-policy/google-directory. The key has no read
// path: no GET returns it and no error echoes it. The store owns the gate, the
// check against Google and the sealing.

import (
	"errors"
	"net/http"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/httperr"
)

// GetGoogleDirectory implements (GET /installation/authentication-policy/google-directory).
func (h Handlers) GetGoogleDirectory(w http.ResponseWriter, r *http.Request) {
	if err := auth.RequireHuman(r.Context()); err != nil {
		httperr.Write(w, r, err)
		return
	}
	status, err := h.svc.GoogleDirectoryStatus(r.Context())
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, toContractDirectoryStatus(status))
}

// SetGoogleDirectory implements (PUT /installation/authentication-policy/google-directory).
func (h Handlers) SetGoogleDirectory(w http.ResponseWriter, r *http.Request) {
	if err := auth.RequireHuman(r.Context()); err != nil {
		httperr.Write(w, r, err)
		return
	}
	var body crmcontracts.GoogleDirectoryInput
	if !httperr.Decode(w, r, &body) {
		return
	}
	in := DirectoryInput{}
	if body.ServiceAccountJson != nil {
		in.ServiceAccountJSON = *body.ServiceAccountJson
	}
	if body.AdminSubject != nil {
		in.AdminSubject = string(*body.AdminSubject)
	}
	status, err := h.svc.SetGoogleDirectory(r.Context(), in)
	if err != nil {
		h.writeDirectoryError(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, toContractDirectoryStatus(status))
}

// DeleteGoogleDirectory implements (DELETE /installation/authentication-policy/google-directory).
func (h Handlers) DeleteGoogleDirectory(w http.ResponseWriter, r *http.Request) {
	if err := auth.RequireHuman(r.Context()); err != nil {
		httperr.Write(w, r, err)
		return
	}
	if err := h.svc.RemoveGoogleDirectory(r.Context()); err != nil {
		h.writeDirectoryError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// writeDirectoryError answers a missing vault as 503, the operator's to fix and
// nothing the caller sent. The store has already checked the grant by then, so
// the status is no oracle for the vault's presence.
func (h Handlers) writeDirectoryError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, ErrDirectoryUnavailable) {
		httperr.ServiceUnavailable(w, r, ErrDirectoryUnavailable.Error())
		return
	}
	httperr.Write(w, r, err)
}

func toContractDirectoryStatus(s DirectoryStatus) crmcontracts.GoogleDirectoryStatus {
	out := crmcontracts.GoogleDirectoryStatus{Configured: s.Configured, LastCheckedAt: s.LastCheckedAt}
	if s.LastResult != "" {
		result := crmcontracts.GoogleDirectoryStatusLastResult(s.LastResult)
		out.LastResult = &result
	}
	if s.LastReason != "" {
		reason := crmcontracts.GoogleDirectoryStatusLastReason(s.LastReason)
		out.LastReason = &reason
	}
	return out
}
