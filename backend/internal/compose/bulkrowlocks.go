// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Row locks precede every identity lock a per-record writer may take.
func holdBulkRows(ctx context.Context, tx pgx.Tx, target bulkTarget, items []crmcontracts.BulkItem, previewed map[openapi_types.UUID]bool) (map[ids.UUID]crmcontracts.BulkSkip, error) {
	refused := map[ids.UUID]crmcontracts.BulkSkip{}
	for _, item := range items {
		id := ids.UUID(item.Id)
		if previewed != nil && !previewed[item.Id] {
			continue
		}
		if err := target.hold(ctx, tx, id); err != nil {
			skip, unexpected := bulkSkipFor(err)
			if unexpected != nil {
				return nil, unexpected
			}
			skip.Id = item.Id
			refused[id] = skip
		}
	}
	return refused, nil
}

func holdBulkRecord(ctx context.Context, tx pgx.Tx, table string, id ids.UUID) error {
	if err := auth.EnsureReadable(ctx, tx, table, id); err != nil {
		return err
	}
	// Undo reaches archived rows. Literal tables keep the ownership gate exhaustive.
	var err error
	switch agentRecordType(table) {
	case recordTypeContact:
		_, err = storekit.LockRow(ctx, tx, "contact", id, storekit.IncludeArchived)
	case recordTypeCompany:
		_, err = storekit.LockRow(ctx, tx, "company", id, storekit.IncludeArchived)
	case recordTypeDeal:
		_, err = storekit.LockRow(ctx, tx, "deal", id, storekit.IncludeArchived)
	case recordTypeLead:
		_, err = storekit.LockRow(ctx, tx, "lead", id, storekit.IncludeArchived)
	default:
		return apperrors.ErrInvalidArgument
	}
	return err
}

func (t contactBulkTarget) hold(ctx context.Context, tx pgx.Tx, id ids.UUID) error {
	return holdBulkRecord(ctx, tx, "contact", id)
}

func (t companyBulkTarget) hold(ctx context.Context, tx pgx.Tx, id ids.UUID) error {
	return holdBulkRecord(ctx, tx, "company", id)
}

func (t dealBulkTarget) hold(ctx context.Context, tx pgx.Tx, id ids.UUID) error {
	return holdBulkRecord(ctx, tx, "deal", id)
}

func (t leadBulkTarget) hold(ctx context.Context, tx pgx.Tx, id ids.UUID) error {
	return holdBulkRecord(ctx, tx, "lead", id)
}

func (t worklistBulkTarget) hold(ctx context.Context, tx pgx.Tx, id ids.UUID) error {
	_, err := t.lockItem(ctx, tx, id)
	return err
}
