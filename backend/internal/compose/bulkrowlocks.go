// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// heldRows is what holdBulkRows found: each held row as it stood under its
// lock, and why each refused row is left alone.
type heldRows struct {
	rows    map[ids.UUID]bulkRow
	refused map[ids.UUID]crmcontracts.BulkSkip
}

// Row locks precede every identity lock a per-record writer may take.
func holdBulkRows(ctx context.Context, tx pgx.Tx, target bulkTarget, items []crmcontracts.BulkItem, previewed map[openapi_types.UUID]bool) (heldRows, error) {
	held := heldRows{rows: map[ids.UUID]bulkRow{}, refused: map[ids.UUID]crmcontracts.BulkSkip{}}
	for _, item := range items {
		id := ids.UUID(item.Id)
		if previewed != nil && !previewed[item.Id] {
			continue
		}
		row, err := target.hold(ctx, tx, id)
		if err != nil {
			skip, unexpected := bulkSkipFor(err)
			if unexpected != nil {
				return heldRows{}, unexpected
			}
			skip.Id = item.Id
			held.refused[id] = skip
			continue
		}
		held.rows[id] = row
	}
	return held, nil
}

// The holds take archived rows too, because an undo restores them.

func (t contactBulkTarget) hold(ctx context.Context, tx pgx.Tx, id ids.UUID) (bulkRow, error) {
	row, err := t.store.HoldContactForBulkTx(ctx, tx, ids.From[ids.ContactKind](id))
	return heldContactsRow(row), err
}

func (t companyBulkTarget) hold(ctx context.Context, tx pgx.Tx, id ids.UUID) (bulkRow, error) {
	row, err := t.store.HoldCompanyForBulkTx(ctx, tx, ids.From[ids.CompanyKind](id))
	return heldContactsRow(row), err
}

func (t leadBulkTarget) hold(ctx context.Context, tx pgx.Tx, id ids.UUID) (bulkRow, error) {
	row, err := t.store.HoldLeadForBulkTx(ctx, tx, ids.From[ids.LeadKind](id))
	return heldContactsRow(row), err
}

func heldContactsRow(row contacts.BulkRow) bulkRow {
	return bulkRow{label: row.Label, version: row.Version, ownerID: row.OwnerID, archived: row.Archived}
}

func (t dealBulkTarget) hold(ctx context.Context, tx pgx.Tx, id ids.UUID) (bulkRow, error) {
	row, err := t.store.HoldDealForBulkTx(ctx, tx, ids.From[ids.DealKind](id))
	return bulkRow{label: row.Label, version: row.Version, ownerID: row.OwnerID, archived: row.Archived, lock: row.Lock}, err
}

func (t worklistBulkTarget) hold(ctx context.Context, tx pgx.Tx, id ids.UUID) (bulkRow, error) {
	item, err := t.lockItem(ctx, tx, id)
	return item.row, err
}
