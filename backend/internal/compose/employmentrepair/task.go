// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package employmentrepair runs bounded corrections through the contact store.
package employmentrepair

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
)

// DataTask bounds corrections through the existing employment writer.
func DataTask(store *contacts.Store) datatask.Task {
	return datatask.Task{
		ID: "core/1794739854_resolve_undated_employment", Version: 1,
		Title: systemtext.Literal("datatask.title.core.1794739854_resolve_undated_employment"), Requires: []string{"core/1789354558"},
		Steps: []datatask.Step{{
			Name: "resolve_newer_roles", Limit: 10, MinLimit: 1, Audit: datatask.AuditPolicy{PerRecord: true}, Estimate: contacts.CountUnresolvedEmploymentContacts,
			Batch: func(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
				after, err := datatask.UUIDCursor(in.Cursor)
				if err != nil {
					return datatask.BatchOut{}, err
				}
				scanned, changed, err := store.RepairUndatedEmploymentTx(ctx, tx, after, in.Limit)
				if err != nil {
					return datatask.BatchOut{}, err
				}
				return datatask.AdvanceUUID(in, scanned, changed)
			},
		}},
	}
}
