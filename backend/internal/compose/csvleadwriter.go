// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The lead object's own create path.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/importkeys"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// errImportReplayed aborts a landing that wrote nothing: the store answered with
// a record that already existed under its natural key. Rolling back keeps the
// identity row out of the map. Adopting a record this run did not create would
// make the next attempt resolve it as already imported.
var errImportReplayed = errors.New("import: the record replayed under its natural key")

// leadImportOrigin is where a contact's owner reads a refused import row came from.
const leadImportOrigin = "a CSV import"

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
	// The row's own transaction has ended, so the owner of a contact the row
	// names is told, as on every other lead door.
	err = w.contacts.TellContactOwnerOfLead(ctx, err, in, leadImportOrigin)
	if _, held := errors.AsType[*contacts.ContactHoldsIdentityError](err); held {
		return skippedFor(contactHoldsLeadWhy), nil
	}
	if _, held := errors.AsType[*contacts.DuplicateLeadError](err); held || errors.Is(err, errImportReplayed) {
		return skippedFor(naturalKeyTakenWhy), nil
	}
	if importkeys.LinkedInClaimed(err) {
		return skippedFor(linkedInClaimedWhy), nil
	}
	if refusedAsErased(err) {
		return skippedFor(addressErasedWhy), nil
	}
	if err != nil {
		return migration.EnsureResult{}, err
	}
	return migration.EnsureResult{Created: true}, nil
}
