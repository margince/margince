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

	"github.com/margince/margince/backend/internal/platform/settings"
)

// refuseZoneTheDatabaseLacksTx refuses an installation zone Postgres cannot
// read. SQL reads the zone on every activity write, so a zone only Go knows
// would fail each one. It runs in the write's transaction, which a refusal aborts.
func refuseZoneTheDatabaseLacksTx(ctx context.Context, tx pgx.Tx, zone string) error {
	_, err := tx.Exec(ctx, `SELECT now() AT TIME ZONE $1::text`, zone)
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == pgerrcode.InvalidParameterValue {
		return settings.InvalidValue{
			Setting: Timezone.Key(), Code: settings.CodeInvalidValue,
			Reason: fmt.Sprintf("%q is not a zone the database knows, so use one like Europe/Berlin", zone),
		}
	}
	if err != nil {
		return fmt.Errorf("identity: checking the installation zone in the database: %w", err)
	}
	return nil
}
