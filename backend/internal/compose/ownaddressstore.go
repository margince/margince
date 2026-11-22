// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/compose/ownaddress"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// NewOwnerIdentityStore is the store every door that declares a member's own
// address uses. Its un-archive is the root's, because the erasure boundary is.
func NewOwnerIdentityStore(pool *pgxpool.Pool, store *contacts.Store) *capture.OwnerIdentityStore {
	return ownaddress.NewStore(InstallationDB(pool), store, func(ctx context.Context, tx pgx.Tx, id ids.ContactID) error {
		_, err := store.RestoreContactTx(ctx, tx, id, nil, unarchiveWith(nil))
		return err
	})
}
