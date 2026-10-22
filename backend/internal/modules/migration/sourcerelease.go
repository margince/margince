// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package migration

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/storedobjects"
)

// releaseSource records that the run no longer needs its uploaded file, on the
// caller's transaction. The profile read from the file goes with it. The key
// is declared provisional, so the reaper deletes the bytes once no unreleased
// run names them. Releasing twice changes nothing. Only a csv run's source is
// a stored upload; a flip's names its portal.
func releaseSource(ctx context.Context, tx pgx.Tx, id RunID) error {
	var sourceRef string
	err := tx.QueryRow(ctx, `
		UPDATE import_run SET source_released_at = now(), profile = NULL,
			report = report - 'review_plan', updated_at = now()
		 WHERE id = $1 AND connector = $2 AND source_released_at IS NULL
		RETURNING source_ref`, id, ConnectorCSV).Scan(&sourceRef)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("releasing the source of import run %s: %w", id, err)
	}
	return storedobjects.RecordTx(ctx, tx, storedobjects.KindImport, sourceRef)
}
