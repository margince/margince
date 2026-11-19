// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"errors"
	"fmt"
	"net/http"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
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
	// Which mail reaches a model is a human's to decide. An agent writing this
	// could be steered by the very mail it would expose.
	if actor, ok := principal.Actor(r.Context()); (req.PrivateMailRequests != nil || req.SharePrivateQuoteRequests != nil) &&
		(!ok || actor.Type != principal.PrincipalHuman) {
		writeStoreErr(w, r, fmt.Errorf("only a human may change %s or %s: %w",
			PrivateMailRequests.Key(), SharePrivateQuoteRequests.Key(), apperrors.ErrPermissionDenied))
		return
	}
	if err := config.Save(r.Context(), h.settings, FollowUpAfterDays, req.FollowUpAfterDays); err != nil {
		writeStoreErr(w, r, err)
		return
	}
	for setting, value := range map[*config.Setting[bool]]*bool{
		PrivateMailRequests: req.PrivateMailRequests, SharePrivateQuoteRequests: req.SharePrivateQuoteRequests,
	} {
		if value == nil {
			continue
		}
		if err := config.Save(r.Context(), h.settings, setting, *value); err != nil {
			writeStoreErr(w, r, err)
			return
		}
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
	private, err := config.StoredVal(r.Context(), h.settings, PrivateMailRequests)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	share, err := config.StoredVal(r.Context(), h.settings, SharePrivateQuoteRequests)
	if err != nil {
		writeStoreErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, crmcontracts.FollowUpSettings{
		FollowUpAfterDays: days, PrivateMailRequests: &private, SharePrivateQuoteRequests: &share,
		Locks: h.settingLocks(ConfigSettings()...),
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
