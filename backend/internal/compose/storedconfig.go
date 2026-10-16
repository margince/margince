// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// A stored setting a boot source sets is locked: it reads as that value and
// refuses a change. Compose builds the stores that read stored settings from a
// pool in many places, at boot and per job. So the program hands its loaded
// configuration over once, and every one of them reads through it.

import (
	"context"
	"errors"
	"io/fs"
	"log/slog"
	"sync"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/platform/keyvault"
	"github.com/margince/margince/backend/internal/platform/settings"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// UseConfig hands compose the configuration this program loaded. A program
// that reads stored settings calls it once, after Load and before it composes.
// It binds the process's configuration, which the modules' package-level
// readers take their locks from too.
func UseConfig(c *config.Config) { config.Bind(c) }

// loadedConfig is the program's configuration, or for a process that loaded
// none, such as a test composing stores, one where no boot source sets anything.
func loadedConfig() *config.Config {
	if c := config.Bound(); c != nil {
		return c
	}
	return unsetConfig()
}

// StoredConfig reads and saves stored settings through pool, under the locks
// the loaded configuration sets.
func StoredConfig(pool *pgxpool.Pool) *config.Config {
	return loadedConfig().WithDatabase(NewSettingsStore(pool))
}

// WithVault is cfg sealing its secret settings in vault, under the workspace of
// each call's context. A role with no vault keeps cfg, which then refuses to
// seal or open one.
func WithVault(cfg *config.Config, vault keyvault.Vault, log *slog.Logger) *config.Config {
	if vault == nil {
		return cfg
	}
	return cfg.WithSealer(vaultSealer(vault, log))
}

// errNoSealingWorkspace refuses a seal or an open outside a workspace: the
// vault binds each blob to one.
var errNoSealingWorkspace = errors.New("compose: a sealed setting is read or written outside a workspace context")

func sealingWorkspace(ctx context.Context) (ids.WorkspaceID, error) {
	raw, ok := principal.WorkspaceID(ctx)
	if !ok {
		return ids.WorkspaceID{}, errNoSealingWorkspace
	}
	return ids.From[ids.WorkspaceKind](raw), nil
}

func vaultSealer(vault keyvault.Vault, log *slog.Logger) config.Sealer {
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	return config.Sealer{
		Seal: func(ctx context.Context, plaintext string) (string, error) {
			ws, err := sealingWorkspace(ctx)
			if err != nil {
				return "", err
			}
			ref, err := vault.Put(ctx, ws, []byte(plaintext))
			return string(ref), err
		},
		Resolve: func(ctx context.Context, ref string) (string, error) {
			ws, err := sealingWorkspace(ctx)
			if err != nil {
				return "", err
			}
			secret, err := vault.Get(ctx, ws, keyvault.Ref(ref))
			return string(secret), err
		},
		Retire: func(ctx context.Context, ref string) {
			ws, err := sealingWorkspace(ctx)
			if err != nil {
				log.ErrorContext(ctx, "a sealed setting's superseded blob was left in the vault", "credential_ref", ref, "error", err)
				return
			}
			keyvault.DeleteDetached(ctx, vault, log, ws.UUID, keyvault.Ref(ref), "sealed setting replaced")
		},
	}
}

// unsetConfig holds every stored setting with no boot source. It is also where
// the settings catalog finds their rows, which every program registers alike.
var unsetConfig = sync.OnceValue(func() *config.Config {
	reg, err := config.NewRegistry(StoredSettings())
	if err != nil {
		panic(err)
	}
	cfg, err := config.Load(reg, config.Sources{ReadFile: func(string) ([]byte, error) { return nil, fs.ErrNotExist }})
	if err != nil {
		panic(err)
	}
	return cfg
})

// StoredSettings are the settings an administrator may also change at run
// time, for a program lane whose registry must lock them as the worker's does.
func StoredSettings() []config.Any {
	var stored []config.Any
	for _, s := range allSettings() {
		if s.Stored() {
			stored = append(stored, s)
		}
	}
	return stored
}

// storedDefinitions are the rows of the stored settings, for the settings catalog.
func storedDefinitions() []settings.Definition { return unsetConfig().StoredDefinitions() }
