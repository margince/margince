// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/settings"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// refuseZoneTheDatabaseLacksTx refuses an installation zone Postgres cannot
// read. SQL reads the zone on every activity write, so a zone only Go knows
// would fail each one.
func refuseZoneTheDatabaseLacksTx(ctx context.Context, tx pgx.Tx, zone string) error {
	known, err := databaseKnowsZoneTx(ctx, tx, zone)
	if err != nil {
		return err
	}
	if !known {
		return settings.InvalidValue{
			Setting: Timezone.Key(), Code: settings.CodeInvalidValue,
			Reason: fmt.Sprintf("%q is not a zone the database knows, so use one like Europe/Berlin", zone),
		}
	}
	return nil
}

// databaseKnowsZoneTx asks Postgres to read the zone. The probe runs in a
// savepoint, because a failed statement would abort the caller's transaction.
func databaseKnowsZoneTx(ctx context.Context, tx pgx.Tx, zone string) (bool, error) {
	known, err := storekit.Savepoint(ctx, tx, func(nested pgx.Tx) (bool, error) {
		_, err := nested.Exec(ctx, `SELECT now() AT TIME ZONE $1::text`, zone)
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == pgerrcode.InvalidParameterValue {
			return false, nil
		}
		return err == nil, err
	})
	if err != nil {
		return false, fmt.Errorf("identity: checking the installation zone in the database: %w", err)
	}
	return known, nil
}

// claimedZone is the zone a claim stores: the one sent, or UTC for a name Go
// or the database does not know.
//
// The claim form sends the zone the browser reports, and a browser can hold
// newer zone data than the server.
func claimedZone(sent string, databaseKnows func(zone string) (bool, error)) (string, error) {
	zone, err := values.ParseTimezone(sent)
	var refusal *values.ParseError
	if errors.As(err, &refusal) && refusal.Code == values.CodeTimezoneUnknown {
		return storekit.FallbackZone, nil
	}
	if err != nil {
		return sent, nil //nolint:nilerr // normalize refuses a malformed name in its own words.
	}
	known, err := databaseKnows(zone.String())
	if err != nil || known {
		return zone.String(), err
	}
	return storekit.FallbackZone, nil
}

// ZoneFallsBackTx reports whether the stored installation zone will not load
// in Go, so background passes count the installation's days in UTC.
func ZoneFallsBackTx(ctx context.Context, tx pgx.Tx) (bool, error) {
	stored, err := settings.ApplyTx(ctx, tx, Timezone)
	if err != nil {
		return false, err
	}
	return !storekit.ZoneLoads(stored), nil
}
