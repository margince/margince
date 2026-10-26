// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The settings of one composed unit (GET and PUT /v1/extensions/{name}/settings):
// the governance switch the core owns, and the settings the unit declares as
// data. They live in compose beside the inventory because the set of valid
// unit names is the composition root's fact.

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
	unit, err := requireComposedUnit(name)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	h.writeSettings(w, r, unit)
}

// UpdateExtensionSettings (PUT /v1/extensions/{name}/settings). Human and
// admin only; the identity setters check the admin role and audit the change.
func (h extensionSettingsHandlers) UpdateExtensionSettings(w http.ResponseWriter, r *http.Request, name string) {
	if err := auth.RequireHuman(r.Context()); err != nil {
		httperr.Write(w, r, err)
		return
	}
	var req crmcontracts.ExtensionSettings
	if !httperr.Decode(w, r, &req) {
		return
	}
	unit, err := requireComposedUnit(name)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	var sent []crmcontracts.DeclaredExtensionSetting
	if req.Declared != nil {
		sent = *req.Declared
	}
	changes, err := declaredChanges(unit, sent)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	if err := identity.SetExtensionSettings(r.Context(), h.store, name, req.OverwritesEditedFields, changes); err != nil {
		httperr.Write(w, r, err)
		return
	}
	h.writeSettings(w, r, unit)
}

// writeSettings answers with the unit's current settings. The declared ones go
// to an admin only; a reader who is not one gets the governance switch alone.
func (h extensionSettingsHandlers) writeSettings(w http.ResponseWriter, r *http.Request, unit extension.Extension) {
	on, err := identity.ExtensionOverwritesEditedFieldsOf(r.Context(), h.store, string(unit.Name))
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	landsRecords := landsCompaniesOrDeals(unit)
	out := crmcontracts.ExtensionSettings{OverwritesEditedFields: on, LandsRecords: &landsRecords}
	if auth.RequireAdmin(r.Context()) == nil {
		stored, err := identity.ExtensionSettingValuesOf(r.Context(), h.store)
		if err != nil {
			httperr.Write(w, r, err)
			return
		}
		declared := declaredView(unit, stored)
		out.Declared = &declared
	}
	httperr.WriteJSON(w, http.StatusOK, out)
}

// landsCompaniesOrDeals reports whether the unit's ingress lands a kind that has
// fields a user can edit. An activity has none, so a unit that lands only
// activities has nothing for the overwrite switch to decide.
func landsCompaniesOrDeals(unit extension.Extension) bool {
	for _, source := range unit.Ingress {
		if slices.Contains(source.Lands, extension.KindCompany) || slices.Contains(source.Lands, extension.KindDeal) {
			return true
		}
	}
	return false
}

// declaredChanges turns the values a caller sent into the changes to save. A
// key the unit never declared, or a value its kind cannot hold, refuses the
// whole write, so a half-applied form cannot happen.
func declaredChanges(unit extension.Extension, sent []crmcontracts.DeclaredExtensionSetting) (map[string]string, error) {
	changes := make(map[string]string, len(sent))
	for _, in := range sent {
		i := slices.IndexFunc(unit.Settings, func(s extension.Setting) bool { return s.Key == in.Key })
		if i < 0 {
			return nil, fmt.Errorf("the unit declares no setting %q: %w", in.Key, apperrors.ErrInvalidArgument)
		}
		if err := unit.Settings[i].CheckValue(in.Value); err != nil {
			return nil, fmt.Errorf("%w: %w", err, apperrors.ErrInvalidArgument)
		}
		changes[in.Key] = in.Value
	}
	return changes, nil
}

// declaredView lists the unit's declared settings with the saved value of
// each, or its default where an admin never changed it.
func declaredView(unit extension.Extension, stored map[string]string) []crmcontracts.DeclaredExtensionSetting {
	out := make([]crmcontracts.DeclaredExtensionSetting, 0, len(unit.Settings))
	for _, s := range unit.Settings {
		value, ok := stored[string(unit.Name)+"/"+s.Key]
		if !ok {
			value = s.Default()
		}
		kind := crmcontracts.DeclaredExtensionSettingKind(s.Kind)
		out = append(out, crmcontracts.DeclaredExtensionSetting{Key: s.Key, Label: &s.Label, Hint: &s.Hint, Kind: &kind, Value: value})
	}
	return out
}

// requireComposedUnit answers 404 for a name this binary did not compose, so a
// setting is never stored for a unit that cannot read it.
func requireComposedUnit(name string) (extension.Extension, error) {
	units := ComposedExtensions()
	i := slices.IndexFunc(units, func(e extension.Extension) bool { return string(e.Name) == name })
	if i < 0 {
		return extension.Extension{}, fmt.Errorf("no extension unit with that name: %w", apperrors.ErrNotFound)
	}
	return units[i], nil
}
