// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"errors"
	"net/http"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/platform/httperr"
)

// WithSettings binds the configuration the follow-up window is read and
// written through. Its save gates, refuses a boot-set value, validates and
// audits the write.
func (h Handlers) WithSettings(cfg *config.Config) Handlers {
	h.settings = cfg
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
	if err := config.Save(r.Context(), h.settings, FollowUpAfterDays, req.FollowUpAfterDays); err != nil {
		writeStoreErr(w, r, err)
		return
	}
	// The value just written, not a fresh read: an update grant without a
	// read grant must not commit the change and then be told 403.
	req.Locks = h.settingLocks(ConfigSettings()...)
	httperr.WriteJSON(w, http.StatusOK, req)
}

func (h Handlers) writeFollowUpSettings(w http.ResponseWriter, r *http.Request) {
	if h.settings == nil {
		writeStoreErr(w, r, errors.New("activities: the follow-up window is not wired; the installation cannot read it"))
		return
	}
	days, err := config.StoredVal(r.Context(), h.settings, FollowUpAfterDays)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, crmcontracts.FollowUpSettings{
		FollowUpAfterDays: days, Locks: h.settingLocks(ConfigSettings()...),
	})
}

// WithLocks binds what reports the settings a boot source sets.
func (h Handlers) WithLocks(locks func(surface []config.Any) *[]crmcontracts.SettingLock) Handlers {
	h.locks = locks
	return h
}

// settingLocks are the locks among surface, the settings one response carries.
func (h Handlers) settingLocks(surface ...config.Any) *[]crmcontracts.SettingLock {
	if h.locks == nil {
		return nil
	}
	return h.locks(surface)
}
