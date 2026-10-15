// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

// The installation's jurisdiction at boot. The deployment file seeds it into an
// installation that has none. The boot reads what is stored, to refuse a code
// this build cannot answer.

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
		stored, seedErr = settings.SeedValue(ctx, tx, Country, code, nil)
		return seedErr
	})
	return stored, err
}

// CountryForDecision reads the jurisdiction for a transaction that destroys
// or holds records under its floor. It keeps the country stable until that
// transaction ends. A jurisdiction change waits for it. It waits for a change
// already in flight, and then reads the new value.
func CountryForDecision(ctx context.Context, tx pgx.Tx) (jurisdiction.Code, error) {
	if err := settings.ShareLockForDecision(ctx, tx, Country.Key()); err != nil {
		return "", err
	}
	return CountryOf(ctx, tx)
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
