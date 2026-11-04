// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/collections"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/datatask"
)

// technicalAddressDataTask retires contacts a technical address made. It
// archives the technical-only ones. It retires the technical address of a
// contact that also holds a real one. It tags for review what it may not
// archive.
func technicalAddressDataTask(pool *pgxpool.Pool) datatask.Task {
	store := newContactsStore(InstallationDB(pool))
	return datatask.Task{
		ID: "core/1793763111_technical_address_contacts", Version: 1,
		Title: "Retire contacts made from a technical address", Requires: []string{"core/1788327300"},
		Steps: []datatask.Step{{
			Name: "retire_technical_address_contacts", Limit: 25, MinLimit: 1,
			Audit:    datatask.AuditPolicy{PerRecord: true},
			Estimate: estimateMachineAddressContacts,
			Batch: func(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
				return retireTechnicalAddressBatch(ctx, tx, store, in)
			},
		}},
	}
}

func retireTechnicalAddressBatch(
	ctx context.Context, tx pgx.Tx, store *contacts.Store, in datatask.BatchIn,
) (datatask.BatchOut, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return datatask.BatchOut{}, err
	}
	after, err := datatask.UUIDCursor(in.Cursor)
	if err != nil {
		return datatask.BatchOut{}, err
	}
	page, err := store.RetireTechnicalAddressContactsTx(ctx, tx, after, in.Limit)
	if err != nil {
		return datatask.BatchOut{}, err
	}
	tagged, err := collections.ApplySystemTagTx(ctx, tx, machineAddressTag, "contact", page.Held,
		map[string]any{machineAddressTaggedBy: in.RunID.String()})
	if err != nil {
		return datatask.BatchOut{}, err
	}
	return datatask.AdvanceUUID(in, page.Scanned, page.Archived+page.Retired+tagged)
}
