// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package identity

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/settings"
)

// A zone the database cannot read is refused as an invalid installation zone,
// so it never reaches the SQL every activity write runs. One it reads passes.
func TestAZoneTheDatabaseLacksIsRefused(t *testing.T) {
	_, pool := setupIdentityDB(t)
	ctx := context.Background()
	for zone, refused := range map[string]bool{"Margince/Nowhere": true, "Pacific/Kiritimati": false} {
		err := pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error { return refuseZoneTheDatabaseLacksTx(ctx, tx, zone) })
		var invalid settings.InvalidValue
		switch {
		case refused && (!errors.As(err, &invalid) || invalid.Setting != Timezone.Key()):
			t.Errorf("%s: the check answered %v, want a refusal naming %s", zone, err, Timezone.Key())
		case !refused && err != nil:
			t.Errorf("%s: the check refused a zone the database reads: %v", zone, err)
		}
	}
}
