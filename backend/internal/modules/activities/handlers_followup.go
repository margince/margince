// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"errors"
	"net/http"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/platform/settings"
)

// WithSettings binds the installation settings store the follow-up window is
// written through, which validates, gates and audits the write.
func (h Handlers) WithSettings(store *settings.Store) Handlers {
	h.settings = store
	return h
}

// GetFollowUpSettings serves GET /activities/follow-up-settings.
func (h Handlers) GetFollowUpSettings(w http.ResponseWriter, r *http.Request) {
	h.writeFollowUpSettings(w, r)
}

// UpdateFollowUpSettings serves PATCH /activities/follow-up-settings (admin/ops, human only).
func (h Handlers) UpdateFollowUpSettings(w http.ResponseWriter, r *http.Request) {
	var req crmcontracts.FollowUpSettings
	if !httperr.Decode(w, r, &req) {
		return
	}
	if h.settings == nil {
		writeStoreErr(w, r, errors.New("activities: the follow-up window is not wired; the installation cannot change it"))
		return
	}
	if err := settings.Set(r.Context(), h.settings, FollowUpAfterDays, req.FollowUpAfterDays); err != nil {
		writeStoreErr(w, r, err)
		return
	}
	// The value just written, not a fresh read: an update grant without a
	// read grant must not commit the change and then be told 403.
	httperr.WriteJSON(w, http.StatusOK, req)
}

func (h Handlers) writeFollowUpSettings(w http.ResponseWriter, r *http.Request) {
	if h.settings == nil {
		writeStoreErr(w, r, errors.New("activities: the follow-up window is not wired; the installation cannot read it"))
		return
	}
	days, err := settings.Get(r.Context(), h.settings, FollowUpAfterDays)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, crmcontracts.FollowUpSettings{FollowUpAfterDays: days})
}
