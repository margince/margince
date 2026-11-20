// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package importscopes is the data task that gives mail and meetings captured
// before connections carried a sending company the company of their mailbox.
package importscopes

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
)

// DataTask copies the company from the connection an import came from. An
// import whose connection has no owner stays unscoped rather than guessed at.
func DataTask() datatask.Task {
	return datatask.Task{
		ID: "core/1795180850_import_sending_companies", Version: 1,
		Title:    systemtext.Literal("datatask.title.core.1795180850_import_sending_companies"),
		Requires: []string{"core/1795180850"},
		Steps: []datatask.Step{{
			Name: "scope_imports", Limit: 500, MinLimit: 1,
			Audit:         datatask.AuditPolicy{Reason: "capture.recordImportTx stamps the same company from the connection on a new import, without an audit"},
			EmptyWhenDone: true,
			Estimate:      estimate, Batch: batch,
		}},
	}
}

func batch(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return datatask.BatchOut{}, err
	}
	after, err := datatask.UUIDCursor(in.Cursor)
	if err != nil {
		return datatask.BatchOut{}, err
	}
	scoped, err := capture.ScopeImportsTx(ctx, tx, after, in.Limit)
	if err != nil {
		return datatask.BatchOut{}, err
	}
	return datatask.AdvanceUUID(in, scoped, len(scoped))
}

func estimate(ctx context.Context, q datatask.Querier, maximum int64) (int64, bool, error) {
	if err := auth.Require(ctx, "data_task", principal.ActionRead); err != nil {
		return 0, false, err
	}
	count, err := capture.CountUnscopedImports(ctx, q, maximum)
	return min(count, maximum), count > maximum, err
}
