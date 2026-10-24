// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The governance settings of one composed unit (GET and PUT
// /v1/extensions/{name}/settings). They live in compose beside the inventory
// because the set of valid unit names is the composition root's fact.

import (
	"fmt"
	"net/http"
	"slices"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/platform/settings"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/pkg/extension"
)

type extensionSettingsHandlers struct {
	store *settings.Store
}

// GetExtensionSettings (GET /v1/extensions/{name}/settings). Gated on
// extension_access.read, like the inventory it extends.
func (h extensionSettingsHandlers) GetExtensionSettings(w http.ResponseWriter, r *http.Request, name string) {
	if err := auth.Require(r.Context(), "extension_access", principal.ActionRead); err != nil {
		httperr.Write(w, r, err)
		return
	}
	if err := requireComposedUnit(name); err != nil {
		httperr.Write(w, r, err)
		return
	}
	on, err := identity.ExtensionOverwritesEditedFieldsOf(r.Context(), h.store, name)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, crmcontracts.ExtensionSettings{OverwritesEditedFields: on})
}

// UpdateExtensionSettings (PUT /v1/extensions/{name}/settings). Human and
// admin only; the identity setter checks the admin role and audits the change.
func (h extensionSettingsHandlers) UpdateExtensionSettings(w http.ResponseWriter, r *http.Request, name string) {
	if err := auth.RequireHuman(r.Context()); err != nil {
		httperr.Write(w, r, err)
		return
	}
	var req crmcontracts.ExtensionSettings
	if !httperr.Decode(w, r, &req) {
		return
	}
	if err := requireComposedUnit(name); err != nil {
		httperr.Write(w, r, err)
		return
	}
	if err := identity.SetExtensionOverwritesEditedFields(r.Context(), h.store, name, req.OverwritesEditedFields); err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, req)
}

// requireComposedUnit answers 404 for a name this binary did not compose, so a
// setting is never stored for a unit that cannot read it.
func requireComposedUnit(name string) error {
	known := slices.ContainsFunc(ComposedExtensions(), func(e extension.Extension) bool { return string(e.Name) == name })
	if !known {
		return fmt.Errorf("no extension unit with that name: %w", apperrors.ErrNotFound)
	}
	return nil
}
