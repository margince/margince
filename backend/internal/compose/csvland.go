// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// land commits the native record, identity and tags together. It caches the
// binding only after commit, so later pages cannot resolve a rolled-back row.
func (w *csvWriters) land(ctx context.Context, row migration.Row, create func(tx pgx.Tx) (ids.UUID, error)) error {
	ctx = contacts.WithBulkWrite(ctx)
	var id ids.UUID
	if err := database.WithWorkspaceTx(ctx, w.pool, func(tx pgx.Tx) error {
		var err error
		if id, err = create(tx); err != nil {
			return err
		}
		if err := w.identities.RecordIdentityTx(ctx, tx, w.runID, w.sourceSystem, w.object, row.ExternalID, id, migration.OriginCreated); err != nil {
			return err
		}
		if err := w.tagNames.applyAll(ctx, tx, w.object, id, textFields(row.Fields)[csvTargetTags]); err != nil {
			return err
		}
		return w.fileUnderContextTag(ctx, tx, id)
	}); err != nil {
		return err
	}
	w.nativeIDs[row.ExternalID] = id
	return nil
}
