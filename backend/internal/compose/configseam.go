// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"cmp"
	"context"
	"slices"
	"sync"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/config"
)

// Locks lists the settings of surface that a boot source sets. It is never
// nil, so a response says "nothing is locked" as [].
func Locks(cfg *config.Config, surface []config.Any) []crmcontracts.SettingLock {
	locks := []crmcontracts.SettingLock{}
	for _, s := range surface {
		if origin, locked := cfg.Locked(s.Key()); locked {
			locks = append(locks, crmcontracts.SettingLock{Key: s.Key(), Origin: settingOrigin(origin)})
		}
	}
	return locks
}

// loadedLocks is Locks over the loaded configuration, as a response field.
// Modules take it through their WithLocks and name their own surfaces.
func loadedLocks(surface []config.Any) *[]crmcontracts.SettingLock {
	locks := Locks(loadedConfig(), surface)
	return &locks
}

// settingOrigin is the wire form of an origin. The API names one YAML source,
// because which file held the value is what `file` says.
func settingOrigin(o config.Origin) crmcontracts.SettingOrigin {
	source := o.Source
	if source == config.SourceBaseFile {
		source = config.SourceConfigFile
	}
	out := crmcontracts.SettingOrigin{Source: crmcontracts.SettingOriginSource(source), Name: o.Name}
	if o.File != "" {
		file := o.File
		out.File = &file
	}
	return out
}

// programRegistries is built once: the overview asks which programs read
// each setting on every request.
var programRegistries = sync.OnceValue(ProgramRegistries)

// Overview lists every setting of every program, sorted by key. Values and
// origins are cfg's; a setting cfg's program does not read has neither, since
// another program's sources are not visible here. It takes the read gate of
// every stored setting, as StoredEntries does.
func Overview(ctx context.Context, cfg *config.Config) ([]crmcontracts.ConfigurationEntry, error) {
	entries, err := cfg.StoredEntries(ctx)
	if err != nil {
		return nil, err
	}
	listed := map[string]bool{}
	for _, e := range entries {
		listed[e.Key] = true
	}
	registries := programRegistries()
	for _, role := range programRoles {
		for _, s := range registries[role].All() {
			if !listed[s.Key()] {
				listed[s.Key()] = true
				entries = append(entries, config.Unread(s))
			}
		}
	}
	out := make([]crmcontracts.ConfigurationEntry, 0, len(entries))
	for _, e := range entries {
		out = append(out, configurationEntry(e, readers(registries, e.Key)))
	}
	slices.SortFunc(out, func(a, b crmcontracts.ConfigurationEntry) int { return cmp.Compare(a.Key, b.Key) })
	return out, nil
}

// readers are the programs whose registry holds key, in programRoles order.
func readers(registries map[config.Role]*config.Registry, key string) []crmcontracts.ConfigurationEntryRoles {
	roles := []crmcontracts.ConfigurationEntryRoles{}
	for _, role := range programRoles {
		if _, ok := registries[role].Lookup(key); ok {
			roles = append(roles, crmcontracts.ConfigurationEntryRoles(role))
		}
	}
	return roles
}

func configurationEntry(e config.Entry, roles []crmcontracts.ConfigurationEntryRoles) crmcontracts.ConfigurationEntry {
	out := crmcontracts.ConfigurationEntry{
		Key: e.Key, Env: e.Env, Type: e.Type, Doc: e.Doc, Roles: roles,
		Secret: e.Secret, Stored: e.Stored, Value: e.Value, Default: e.Default,
		Origin: settingOrigin(e.Origin),
	}
	if e.Flag != "" {
		flag := e.Flag
		out.Flag = &flag
	}
	return out
}
