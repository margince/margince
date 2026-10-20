// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package identity

import (
	"context"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/platform/settings"
)

// unlockedConfig is identity's settings as a server loads them when no boot
// source sets any, read and saved through pool.
func unlockedConfig(t *testing.T, pool *pgxpool.Pool) *config.Config {
	t.Helper()
	return identityConfig(t, environConfig(t), pool)
}

func identityConfig(t *testing.T, cfg *config.Config, pool *pgxpool.Pool) *config.Config {
	t.Helper()
	defs := append(Definitions(), cfg.StoredDefinitions()...)
	return cfg.WithDatabase(settings.New(pool, settings.NewRegistry(defs...)))
}

// A setting a boot source sets is neither seeded nor reported as discarded:
// the operator's value is the one in force.
func TestBootstrapSeedsNoSettingABootSourceSets(t *testing.T) {
	owner, pool := setupIdentityDB(t)
	ctx := context.Background()
	tx, err := owner.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := tx.Rollback(context.Background()); err != nil {
			t.Errorf("rolling back the seed: %v", err)
		}
	})
	if _, err := tx.Exec(ctx, `DELETE FROM setting WHERE key = ANY($1)`,
		[]string{Name.Key(), BaseLanguage.Key()}); err != nil {
		t.Fatal(err)
	}
	cfg := identityConfig(t, environConfig(t, "MARGINCE_INSTALLATION_BASE_LANGUAGE=de"), pool)
	discarded, err := seedInstallationIdentity(ctx, tx, cfg, installationIdentity{
		name: "Seeded Co", zone: "UTC", currency: "EUR", language: "en",
	})
	if err != nil {
		t.Fatal(err)
	}
	if slices.Contains(discarded, BaseLanguage.Key()) {
		t.Errorf("the boot-set base language was reported discarded: %v", discarded)
	}
	var rows int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM setting WHERE key = $1`, BaseLanguage.Key()).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 0 {
		t.Errorf("the boot-set base language was seeded (%d rows)", rows)
	}
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM setting WHERE key = $1`, Name.Key()).Scan(&rows); err != nil || rows != 1 {
		t.Errorf("the name no boot source sets was not seeded: %d rows, %v", rows, err)
	}
}
