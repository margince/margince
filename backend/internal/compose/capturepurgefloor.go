// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The statutory floor capture's destructive paths apply, resolved from the
// installation's jurisdiction in the transaction that decides.

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/platform/database"
)

// purgeFloorTx resolves the floor in force and hands capture the shield
// privacy spells once.
//
// Commercial correspondence inside its legal retention window is not the
// owner's to destroy. A purge that ignored the floor would be the one path that
// destroys past it. Capture cannot import privacy,
// so the predicate travels rather than being written a second time. The policy
// is returned too, so a receipt names the floor the selection used rather than
// resolving its own.
func purgeFloorTx(ctx context.Context, tx pgx.Tx) (capture.StatutoryFloor, privacy.FloorPolicy, error) {
	policy, err := privacy.ResolveFloorPolicy(ctx, tx, identity.CountryOf)
	if err != nil {
		return capture.StatutoryFloor{}, privacy.FloorPolicy{}, err
	}
	current, carried := policy.ShieldArgs()
	return capture.StatutoryFloor{Clause: privacy.StatutoryFloorShield, Current: current, Carried: carried}, policy, nil
}

// purgeFloor resolves the floor in a transaction of its own, for a sweep that
// selects pages across several. Each destroying transaction re-resolves it.
func purgeFloor(ctx context.Context, pool *pgxpool.Pool) (capture.StatutoryFloor, error) {
	var floor capture.StatutoryFloor
	err := database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error {
		var err error
		floor, _, err = purgeFloorTx(ctx, tx)
		return err
	})
	return floor, err
}
