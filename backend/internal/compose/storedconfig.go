// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// A stored setting a boot source sets is locked: it reads as that value and
// refuses a change. Compose builds the stores that read stored settings from a
// pool in many places, at boot and per job. So the program hands its loaded
// configuration over once, and every one of them reads through it.

import (
	"io/fs"
	"sync"
	"sync/atomic"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/platform/settings"
)

var bootConfig atomic.Pointer[config.Config]

// UseConfig hands compose the configuration this program loaded. A program
// that reads stored settings calls it once, after Load and before it composes.
func UseConfig(c *config.Config) { bootConfig.Store(c) }

// loadedConfig is the program's configuration, or for a process that loaded
// none, such as a test composing stores, one where no boot source sets anything.
func loadedConfig() *config.Config {
	if c := bootConfig.Load(); c != nil {
		return c
	}
	return unsetConfig()
}

// StoredConfig reads and saves stored settings through pool, under the locks
// the loaded configuration sets.
func StoredConfig(pool *pgxpool.Pool) *config.Config {
	return loadedConfig().WithDatabase(NewSettingsStore(pool))
}

// unsetConfig holds every stored setting with no boot source. It is also where
// the settings catalog finds their rows, which every program registers alike.
var unsetConfig = sync.OnceValue(func() *config.Config {
	var stored []config.Any
	for _, s := range allSettings() {
		if s.Stored() {
			stored = append(stored, s)
		}
	}
	reg, err := config.NewRegistry(stored)
	if err != nil {
		panic(err)
	}
	cfg, err := config.Load(reg, config.Sources{ReadFile: func(string) ([]byte, error) { return nil, fs.ErrNotExist }})
	if err != nil {
		panic(err)
	}
	return cfg
})

// storedDefinitions are the rows of the stored settings, for the settings catalog.
func storedDefinitions() []settings.Definition { return unsetConfig().StoredDefinitions() }
