// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package approvals

// The transport for a rep's own answer to "how much of this queue should answer
// itself". Apart from handlers.go because it is a different surface: that file
// carries the inbox — a list of things other contacts and machines proposed — and
// this one carries a setting the reader holds about themselves.

import (
	"net/http"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/platform/httperr"
)

// GetAutonomy answers with the reader's own settings, one row per eligible kind.
func (h Handlers) GetAutonomy(w http.ResponseWriter, r *http.Request) {
	// Human-only, matching the contract. An agent asking what its principal has
	// automated is asking a question about the contact rather than about the work,
	// and the answer would tell it which proposals it can expect to skip review.
	if err := auth.RequireHuman(r.Context()); err != nil {
		httperr.Write(w, r, err)
		return
	}
	settings, err := h.svc.AutoApplySettings(r.Context())
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	h.writeAutonomy(w, r, settings)
}

// writeAutonomy answers with the rep's rows and the installation switch above them.
func (h Handlers) writeAutonomy(w http.ResponseWriter, r *http.Request, settings []KindAutonomy) {
	on, err := h.svc.autoApplyInstallationOn(r.Context())
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	wire := autonomyWire(settings)
	wire.AutoApplyEnabled = on
	wire.Locks = h.installationLocks()
	httperr.WriteJSON(w, http.StatusOK, wire)
}

// UpdateAutoApplyInstallation switches automatic application for everyone. The
// admin check and the audit row belong to SetAutoApplyEnabled.
func (h Handlers) UpdateAutoApplyInstallation(w http.ResponseWriter, r *http.Request) {
	if h.settings == nil {
		httperr.NotImplemented(w, r, "UpdateAutoApplyInstallation")
		return
	}
	if err := auth.RequireHuman(r.Context()); err != nil {
		httperr.Write(w, r, err)
		return
	}
	var req crmcontracts.AutoApplyInstallation
	if !httperr.Decode(w, r, &req) {
		return
	}
	if err := SetAutoApplyEnabled(r.Context(), h.settings, req.Enabled); err != nil {
		httperr.Write(w, r, err)
		return
	}
	req.Locks = h.installationLocks()
	httperr.WriteJSON(w, http.StatusOK, req)
}

// installationLocks names the boot source that sets the switch, if one does.
func (h Handlers) installationLocks() *[]crmcontracts.SettingLock {
	if h.locks == nil {
		return nil
	}
	return h.locks([]config.Any{AutoApplyEnabled})
}

// WithSettings binds the stored configuration the installation switch lives in.
func (h Handlers) WithSettings(cfg *config.Config) Handlers {
	h.settings = cfg
	return h
}

// WithLocks binds what reports the settings a boot source sets.
func (h Handlers) WithLocks(locks func(surface []config.Any) *[]crmcontracts.SettingLock) Handlers {
	h.locks = locks
	return h
}

// UpdateAutonomy turns one kind on or off for the reader, then answers with the
// whole set.
//
// It returns everything rather than the row it changed so the client has no
// second read to get wrong: a screen that patched one row and re-rendered from
// its own optimistic copy would show a stale track record beside the switch the
// reader just moved. The service reads it back inside the writing transaction,
// so an answer that arrives at all describes what was committed.
func (h Handlers) UpdateAutonomy(w http.ResponseWriter, r *http.Request) {
	if err := auth.RequireHuman(r.Context()); err != nil {
		httperr.Write(w, r, err)
		return
	}
	var req crmcontracts.UpdateAutonomyRequest
	if !httperr.Decode(w, r, &req) {
		return
	}
	// An absent `kind` names no setting to change.
	//
	// The lookup answered 404 about an empty one, which reads as "no such
	// kind" where the caller named none.
	if raw, sent := httperr.PresentField(r, "kind"); !sent || raw == nil {
		httperr.Write(w, r, httperr.Validation("kind", "required",
			"name the kind of change to set, as GET /autonomy lists them"))
		return
	}
	// An absent `auto` is a client that forgot the field, not one asking to be
	// asked again.
	//
	// The contract requires it, so it decodes into a plain bool and absent is
	// indistinguishable from false. Read off the struct alone, forgetting the
	// field opted the installation out of applying this kind on sight.
	if raw, sent := httperr.PresentField(r, "auto"); !sent || raw == nil {
		httperr.Write(w, r, httperr.Validation("auto", "required",
			"send auto as true to apply this kind on sight, or false to be asked"))
		return
	}
	settings, err := h.svc.SetAutoApply(r.Context(), req.Kind, req.Auto)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	h.writeAutonomy(w, r, settings)
}

// autonomyWire renders the service's rows onto the contract's shape.
func autonomyWire(settings []KindAutonomy) crmcontracts.AutonomySettings {
	data := make([]crmcontracts.KindAutonomy, 0, len(settings))
	for _, s := range settings {
		data = append(data, crmcontracts.KindAutonomy{
			Kind:           s.Kind,
			Mode:           crmcontracts.KindAutonomyMode(s.Mode),
			ApprovedClean:  s.ApprovedClean,
			ApprovedEdited: s.ApprovedEdited,
			Rejected:       s.Rejected,
		})
	}
	return crmcontracts.AutonomySettings{Data: data}
}
