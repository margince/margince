// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// Setting the installation's base language, for the tests that assert stored
// copy answers to it.

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/settings"
)

// SetBaseLanguage stores one language code as the installation's base
// language.
//
// Exported from here because the tests that need it sit in two packages. A
// handler reached only from inside package compose cannot be driven from this
// one.
//
// A second copy is how one of them comes to seed a value the other does not.
//
// Not through settings.Set, which resolves the entry through the registry
// compose assembles. Importing compose here is an import cycle, because
// package compose's own tests import this package.
//
// The stored row is deleted first, because Seed writes only where nothing is
// stored. A discarded write fails loudly: a test judging the previous value
// reads as a pass.
func SetBaseLanguage(ctx context.Context, t *testing.T, pool *pgxpool.Pool, code string) {
	t.Helper()
	if err := database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx,
			`DELETE FROM setting WHERE key = $1`, identity.BaseLanguage.Key()); err != nil {
			return err
		}
		stored, err := settings.SeedValue(ctx, tx, identity.BaseLanguage, code, nil)
		if err != nil {
			return err
		}
		if !stored {
			t.Fatalf("the base language %q was discarded rather than stored, "+
				"so the test below would judge the previous value", code)
		}
		return nil
	}); err != nil {
		t.Fatalf("setting the base language to %q: %v", code, err)
	}
}
