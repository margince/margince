// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package archivedclaims is the data task that deletes the duplicate claims
// the evidence index archived, so a later release can drop the mark.
package archivedclaims

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
)

// evidenceIndexMigration archived the duplicates this task deletes.
const evidenceIndexMigration = "core/1791101875"

// DataTask walks every claim in key order and deletes the archived ones.
func DataTask() datatask.Task {
	return datatask.Task{
		ID: "core/1795851263_archived_claims_deleted", Version: 1,
		Title:    systemtext.Literal("datatask.title.core.1795851263_archived_claims_deleted"),
		Requires: []string{evidenceIndexMigration},
		Steps: []datatask.Step{{
			Name: "conversation_claim", Limit: 200, MinLimit: 1,
			Audit:    datatask.AuditPolicy{PerRecord: true},
			Estimate: estimate, Batch: batch, EmptyWhenDone: true,
		}},
	}
}

func batch(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
	after, err := datatask.UUIDCursor(in.Cursor)
	if err != nil {
		return datatask.BatchOut{}, err
	}
	inspected, deleted, err := contacts.DeleteArchivedClaimsTx(ctx, tx, after, in.Limit)
	if err != nil {
		return datatask.BatchOut{}, err
	}
	return datatask.AdvanceUUID(in, inspected, deleted)
}

func estimate(ctx context.Context, q datatask.Querier, maximum int64) (int64, bool, error) {
	if err := auth.Require(ctx, "data_task", principal.ActionRead); err != nil {
		return 0, false, err
	}
	count, err := contacts.CountArchivedClaims(ctx, q, maximum)
	return min(count, maximum), count > maximum, err
}
