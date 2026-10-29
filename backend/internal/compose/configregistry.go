// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"slices"
	"sync"

	"github.com/margince/margince/backend/internal/compose/programsettings"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/assurance"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/modules/integrations"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/platform/database/dbsettings"
	"github.com/margince/margince/backend/internal/platform/keyvault"
)

// programRoles are the programs that load a configuration.
var programRoles = []config.Role{config.RoleAPI, config.RoleWorker, config.RoleMigrate}

// allSettings gathers the settings the programs read, from the package that owns each.
func allSettings() []config.Any {
	return slices.Concat(
		programsettings.Settings(),
		programsettings.FileSettings(),
		dbsettings.Settings(),
		keyvault.Settings(),
		blobstore.Settings(),
		ai.Settings(),
		[]config.Any{providerModeSetting, LicenseToken, LicenseTokenFile, RatesModelPricing},
		captureSettings(),
		UploadSettings(),
		[]config.Any{CompanyContextRollout, WebSecurityTxt},
		MailSettings(),
		identity.Settings(),
		capture.ConfigSettings(),
		integrations.ConfigSettings(),
		ai.ConfigSettings(),
		activities.ConfigSettings(),
		assurance.ConfigSettings(),
		consent.ConfigSettings(),
		contacts.ConfigSettings(),
		deals.ConfigSettings(),
		privacy.ConfigSettings(),
	)
}

// readBy is whether role reads s. A stored setting with no Roles belongs to
// both long-running programs, because either may read it from the database.
func readBy(s config.Any, role config.Role) bool {
	if len(s.Roles()) == 0 {
		return s.Stored() && role != config.RoleMigrate
	}
	return slices.Contains(s.Roles(), role)
}

// ProgramRegistry is the registry one program loads its configuration with.
func ProgramRegistry(role config.Role) (*config.Registry, error) {
	var own []config.Any
	for _, s := range allSettings() {
		if readBy(s, role) {
			own = append(own, s)
		}
	}
	return config.NewRegistry(own)
}

// ProgramRegistries holds every program's registry, for the gates and docs
// that read them all. It panics on a declaration fault, a wiring mistake that
// ProgramRegistry would refuse at each program's boot anyway.
func ProgramRegistries() map[config.Role]*config.Registry {
	out := make(map[config.Role]*config.Registry, len(programRoles))
	for _, role := range programRoles {
		reg, err := ProgramRegistry(role)
		if err != nil {
			panic(err)
		}
		out[role] = reg
	}
	return out
}

// AllSettingNames is Sources.Known for every program: a margince.yaml shared by
// the api and the worker carries keys only one of them reads. Built once, as
// every Load reads it.
var AllSettingNames = sync.OnceValue(func() config.Names {
	var names config.Names
	for _, s := range allSettings() {
		names.Keys = append(names.Keys, s.Key())
		names.Envs = append(names.Envs, s.Env())
	}
	return names
})
