// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// ownDomainFoldDataTask writes the folded domain on rows stored before the own-
// domain checks compared it in SQL. Until a row has it, the waiting queue reads
// its sender as an outsider and the sender read arm stays shut for it. The
// small tables go first, so the read arm opens before the long participant pass.
func ownDomainFoldDataTask() datatask.Task {
	return datatask.Task{
		ID: "core/1792909146_own_domains_folded", Version: 1,
		Title: "Fold the mail domains own-domain checks compare", Requires: []string{"core/1792909145"},
		Steps: []datatask.Step{
			foldStep("fold_login_domains", "identity writes the same folded login domain on the seat it inserts, without a separate audit",
				identity.FoldLoginDomainsTx, identity.CountUnfoldedLoginDomains),
			foldStep("fold_company_domains", "contacts writes the same folded domain on the company domain it inserts, without a separate audit",
				contacts.FoldCompanyDomainsTx, contacts.CountUnfoldedCompanyDomains),
			foldStep("fold_participant_domains", "every participant writer stores the same folded domain on the row it inserts, without a separate audit",
				activities.FoldParticipantDomainsTx, activities.CountUnfoldedParticipants),
		},
	}
}

// foldStep runs one owning module's fold as a cursor-paged step.
func foldStep(
	name, unaudited string,
	fold func(context.Context, pgx.Tx, ids.UUID, int) ([]ids.UUID, int, error),
	count func(context.Context, datatask.Querier, int64) (int64, error),
) datatask.Step {
	return datatask.Step{
		Name: name, Limit: 500, MinLimit: 20,
		Audit: datatask.AuditPolicy{Reason: unaudited},
		Estimate: func(ctx context.Context, q datatask.Querier, maximum int64) (int64, bool, error) {
			if err := auth.Require(ctx, "data_task", principal.ActionRead); err != nil {
				return 0, false, err
			}
			total, err := count(ctx, q, maximum)
			return min(total, maximum), total > maximum, err
		},
		Batch: func(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
			if err := auth.RequireSystem(ctx); err != nil {
				return datatask.BatchOut{}, err
			}
			after, err := datatask.UUIDCursor(in.Cursor)
			if err != nil {
				return datatask.BatchOut{}, err
			}
			inspected, changed, err := fold(ctx, tx, after, in.Limit)
			if err != nil {
				return datatask.BatchOut{}, err
			}
			return datatask.AdvanceUUID(in, inspected, changed)
		},
	}
}
