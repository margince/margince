// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The lead object's own create path.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// errImportReplayed aborts a landing that wrote nothing: the store answered with
// a record that already existed under its natural key. Rolling back keeps the
// identity row out of the map. Adopting a record this run did not create would
// make the next attempt resolve it as already imported.
var errImportReplayed = errors.New("import: the record replayed under its natural key")

func (w *csvWriters) createLead(ctx context.Context, row migration.Row, typed map[string]any) (migration.EnsureResult, error) {
	in := leadCreateFrom(textFields(row.Fields), typed, w.sourceSystem, row.ExternalID, w.provenanceOf(row.ExternalID))
	err := w.land(ctx, row, func(tx pgx.Tx) (ids.UUID, error) {
		if err := w.authors.resolve(ctx, tx, &in.Author); err != nil {
			return ids.UUID{}, err
		}
		lead, created, err := w.contacts.CreateLeadTx(ctx, tx, in, w.cols.store)
		if err != nil {
			return ids.UUID{}, fmt.Errorf("import: creating lead %s: %w", row.ExternalID, err)
		}
		if !created {
			return ids.UUID{}, errImportReplayed
		}
		return ids.UUID(lead.Id), nil
	})
	if errors.Is(err, errImportReplayed) {
		return skippedFor(naturalKeyTakenWhy), nil
	}
	if linkedInClaimed(err) {
		return skippedFor(linkedInClaimedWhy), nil
	}
	if err != nil {
		return migration.EnsureResult{}, err
	}
	return migration.EnsureResult{Created: true}, nil
}
