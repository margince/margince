// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package configtest

import (
	"io/fs"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/platform/settings"
)

// MustLoadEnv loads owned as a program does from environ alone: no flag, and
// no file, so a margince.yaml beside the test cannot reach it.
func MustLoadEnv(t testing.TB, owned []config.Any, environ ...string) *config.Config {
	t.Helper()
	reg, err := config.NewRegistry(owned)
	if err != nil {
		t.Fatalf("configtest: the settings do not register: %v", err)
	}
	cfg, err := config.Load(reg, config.Sources{
		Environ:  environ,
		ReadFile: func(string) ([]byte, error) { return nil, fs.ErrNotExist },
	})
	if err != nil {
		t.Fatalf("configtest: loading the settings from %v: %v", environ, err)
	}
	return cfg
}

// StoredOver is MustLoadEnv reading and saving its stored settings through
// pool.
func StoredOver(t testing.TB, pool *pgxpool.Pool, owned []config.Any, environ ...string) *config.Config {
	t.Helper()
	_, cfg := StoredOverWith(t, pool, nil, owned, environ...)
	return cfg
}

// StoredOverWith is StoredOver for a package whose settings store also
// registers rows outside the configuration registry, and that needs the store.
func StoredOverWith(
	t testing.TB, pool *pgxpool.Pool, rows []settings.Definition, owned []config.Any, environ ...string,
) (*settings.Store, *config.Config) {
	t.Helper()
	cfg := MustLoadEnv(t, owned, environ...)
	store := settings.New(pool, settings.NewRegistry(slices.Concat(rows, cfg.StoredDefinitions())...))
	return store, cfg.WithDatabase(store)
}

// Bind binds cfg as the program's configuration until the test ends. The
// binding is the process's own, so a test that calls it must not run in
// parallel.
func Bind(t testing.TB, cfg *config.Config) {
	t.Helper()
	before := config.Bound()
	config.Bind(cfg)
	t.Cleanup(func() { config.Bind(before) })
}
