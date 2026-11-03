// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/datatask"
)

// retiredThreadSplitDataTask keeps a released ID whose repair was withdrawn. Version 2
// supersedes any version 1 progress a database recorded, and its one step has no work.
func retiredThreadSplitDataTask() datatask.Task {
	return datatask.Task{ID: "core/1793602001_subject_thread_boundaries", Version: 2, Title: "Separate correspondence at subject boundaries (withdrawn)", Requires: []string{"core/1793602000"}, Steps: []datatask.Step{{
		Name: "withdrawn", Limit: 1, MinLimit: 1,
		Audit: datatask.AuditPolicy{Reason: "the step changes no row"},
		Estimate: func(context.Context, datatask.Querier, int64) (int64, bool, error) {
			return 0, false, nil
		},
		Batch: func(context.Context, pgx.Tx, datatask.BatchIn) (datatask.BatchOut, error) {
			return datatask.BatchOut{Done: true}, nil
		},
	}}}
}
