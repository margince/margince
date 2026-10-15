// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package identity

import (
	"io/fs"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/platform/settings"
)

// unlockedConfig is identity's settings as a server loads them when no boot
// source sets any, read and saved through pool.
func unlockedConfig(t *testing.T, pool *pgxpool.Pool) *config.Config {
	t.Helper()
	reg, err := config.NewRegistry(Settings())
	if err != nil {
		t.Fatalf("identity's settings do not register: %v", err)
	}
	cfg, err := config.Load(reg, config.Sources{ReadFile: func(string) ([]byte, error) { return nil, fs.ErrNotExist }})
	if err != nil {
		t.Fatalf("loading identity's settings with no source: %v", err)
	}
	defs := append(Definitions(), cfg.StoredDefinitions()...)
	return cfg.WithDatabase(settings.New(pool, settings.NewRegistry(defs...)))
}
