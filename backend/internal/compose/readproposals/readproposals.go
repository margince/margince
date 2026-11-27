// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package readproposals is the data task that keys older approval lists. It
// copies the proposal_ids of each site read and transcript read into the table
// that keys the list to the approval.
package readproposals

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
)

// keyedTablesMigration creates the two tables the task fills.
const keyedTablesMigration = "core/1795756890"

// copyAudit is the audit policy of both steps.
var copyAudit = datatask.AuditPolicy{
	Reason: "a copy restates the list proposal_ids already holds; storeSiteReadProposals and " +
		"storeTranscriptReadProposals write the same rows beside the array with no audit row of their own",
}

// copier is one module's copy and count over the reads it owns.
type copier struct {
	copyTx func(context.Context, pgx.Tx, ids.UUID, int) ([]ids.UUID, int, error)
	count  func(context.Context, datatask.Querier, int64) (int64, error)
}

// DataTask walks site reads, then transcript reads, in key order.
func DataTask() datatask.Task {
	return datatask.Task{
		ID: "core/1795756890_read_proposals_keyed", Version: 1,
		Title:    systemtext.Literal("datatask.title.core.1795756890_read_proposals_keyed"),
		Requires: []string{keyedTablesMigration},
		Steps: []datatask.Step{
			step("copy_site_read_proposals", copier{contacts.CopySiteReadProposalsTx, contacts.CountSiteReadsToCopyProposals}),
			step("copy_transcript_read_proposals", copier{activities.CopyTranscriptReadProposalsTx, activities.CountTranscriptReadsToCopyProposals}),
		},
	}
}

func step(name string, c copier) datatask.Step {
	return datatask.Step{
		Name: name, Limit: 200, MinLimit: 1,
		Audit: copyAudit,
		Estimate: func(ctx context.Context, q datatask.Querier, maximum int64) (int64, bool, error) {
			if err := auth.Require(ctx, "data_task", principal.ActionRead); err != nil {
				return 0, false, err
			}
			reads, err := c.count(ctx, q, maximum)
			return min(reads, maximum), reads > maximum, err
		},
		Batch: func(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
			after, err := datatask.UUIDCursor(in.Cursor)
			if err != nil {
				return datatask.BatchOut{}, err
			}
			inspected, changed, err := c.copyTx(ctx, tx, after, in.Limit)
			if err != nil {
				return datatask.BatchOut{}, err
			}
			return datatask.AdvanceUUID(in, inspected, changed)
		},
	}
}
