// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

// The installation's jurisdiction at boot: seeding it from the deployment file
// into an installation that has none, and reading what is stored so the boot
// can refuse a code this build cannot answer.

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/settings"
	"github.com/margince/margince/backend/internal/shared/ports/jurisdiction"
)

// SeedCountryOnce stores the configured country when the installation has no
// stored row. An installation that existed before the setting did has none, and
// without this its operator could set it only through the screen. A stored
// value always wins, so a choice made on the screen is never overwritten by
// the file.
func SeedCountryOnce(ctx context.Context, pool *pgxpool.Pool, code string) (stored bool, err error) {
	err = database.WithInfraTx(ctx, pool, func(tx pgx.Tx) error {
		var seedErr error
		stored, seedErr = settings.SeedValue(ctx, tx, Country, code)
		return seedErr
	})
	return stored, err
}

// StoredCountry reads the installation's jurisdiction outside any request, for
// the boot check.
func StoredCountry(ctx context.Context, pool *pgxpool.Pool) (jurisdiction.Code, error) {
	var code jurisdiction.Code
	err := database.WithInfraTx(ctx, pool, func(tx pgx.Tx) error {
		var readErr error
		code, readErr = CountryOf(ctx, tx)
		return readErr
	})
	return code, err
}
