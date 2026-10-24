// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// unsplitAddressDataTask splits the addresses older writers stored whole in
// line 1, with the splitter today's writers use. Text the splitter refuses
// stays as it is, so the task runs without an operator's review.
func unsplitAddressDataTask() datatask.Task {
	return datatask.Task{
		ID: "core/1792852411_unsplit_addresses", Version: 1,
		Title: "Splitting addresses stored in one line into street, postal code and city",
		Steps: []datatask.Step{
			unsplitAddressStep("companies", contacts.CompanyAddressRepair),
			unsplitAddressStep("contacts", contacts.ContactAddressRepair),
		},
	}
}

// unsplitAddressStep walks every record of one kind by id, a page at a time.
// A batch inspects at most its limit, however few records need a split.
func unsplitAddressStep(name string, kind contacts.AddressRepairKind) datatask.Step {
	return datatask.Step{
		Name: name, Limit: 200, MinLimit: 10,
		Audit: datatask.AuditPolicy{PerRecord: true},
		Estimate: func(ctx context.Context, q datatask.Querier, maximum int64) (int64, bool, error) {
			if err := auth.Require(ctx, "data_task", principal.ActionRead); err != nil {
				return 0, false, err
			}
			count, err := contacts.CountAddressRepairRows(ctx, q, kind, maximum)
			return min(count, maximum), count > maximum, err
		},
		Batch: func(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
			if err := auth.RequireSystem(ctx); err != nil {
				return datatask.BatchOut{}, err
			}
			after, err := datatask.UUIDCursor(in.Cursor)
			if err != nil {
				return datatask.BatchOut{}, err
			}
			page, err := contacts.RepairUnsplitAddressesTx(ctx, tx, kind, after, in.Limit)
			if err != nil {
				return datatask.BatchOut{}, err
			}
			return datatask.AdvanceUUID(in, page.Scanned, page.Changed)
		},
	}
}
