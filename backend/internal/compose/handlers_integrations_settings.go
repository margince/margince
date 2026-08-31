// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The installation's provider-lookup posture: read it (every role), change it
// (admin/ops, human-only). Thin transport — the integrations store owns the
// RBAC gate and the audit-only write.

import (
	"net/http"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/integrations"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/httperr"
)

type integrationsSettingsHandlers struct {
	store *integrations.SettingsStore
}

func (h integrationsSettingsHandlers) GetIntegrationsSettings(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		httperr.NotImplemented(w, r, "GetIntegrationsSettings")
		return
	}
	posture, err := h.store.Get(r.Context())
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, toContractIntegrationsSettings(posture))
}

func (h integrationsSettingsHandlers) UpdateIntegrationsSettings(w http.ResponseWriter, r *http.Request) {
	if h.store == nil {
		httperr.NotImplemented(w, r, "UpdateIntegrationsSettings")
		return
	}
	// Human-only (x-agent-access): an agent never decides whether this
	// installation buys data about the people in it.
	if err := auth.RequireHuman(r.Context()); err != nil {
		httperr.Write(w, r, err)
		return
	}
	var req crmcontracts.UpdateIntegrationsSettingsRequest
	if !httperr.Decode(w, r, &req) {
		return
	}
	posture, err := h.store.Update(r.Context(), req.AutomaticLookup)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, toContractIntegrationsSettings(posture))
}

// toContractIntegrationsSettings maps the stored posture onto the wire shape.
func toContractIntegrationsSettings(s integrations.Settings) crmcontracts.IntegrationsSettings {
	return crmcontracts.IntegrationsSettings{AutomaticLookup: s.AutomaticLookup}
}
