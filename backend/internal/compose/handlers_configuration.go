// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"net/http"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// GetInstallationConfiguration lists every setting the programs read. It is
// for those who change the installation: the values describe the deployment.
func (h installationSettingsHandlers) GetInstallationConfiguration(w http.ResponseWriter, r *http.Request) {
	if h.pool == nil {
		httperr.NotImplemented(w, r, "GetInstallationConfiguration")
		return
	}
	if err := auth.RequireHuman(r.Context()); err != nil {
		httperr.Write(w, r, err)
		return
	}
	if err := auth.Require(r.Context(), "installation_settings", principal.ActionUpdate); err != nil {
		httperr.Write(w, r, err)
		return
	}
	overview, err := Overview(r.Context(), StoredConfig(h.pool))
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, overview)
}
