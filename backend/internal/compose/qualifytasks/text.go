// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package qualifytasks

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
)

// TextDataTask gives each qualify task written before system text the template
// its subject came from, so it renders in the reader's language.
func TextDataTask() datatask.Task {
	return datatask.Task{
		ID: "core/1794744120_qualify_task_text_follows_the_reader", Version: 2, Requires: []string{"core/1794283985"},
		Title: systemtext.Literal("datatask.title.core.1794744120_qualify_task_text_follows_the_reader"),
		Steps: []datatask.Step{{
			Name: "describe_qualify_task_subjects", Limit: 200, MinLimit: 10,
			Audit:    datatask.AuditPolicy{PerRecord: true},
			Estimate: activities.CountQualifyTasksWithoutText,
			Batch: func(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
				after, err := datatask.UUIDCursor(in.Cursor)
				if err != nil {
					return datatask.BatchOut{}, err
				}
				scanned, described, err := activities.DescribeQualifyTaskTextTx(ctx, tx, after, in.Limit)
				if err != nil {
					return datatask.BatchOut{}, err
				}
				return datatask.AdvanceUUID(in, scanned, described)
			},
		}},
	}
}
