// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package qualifytasks repairs the "Review qualification" tasks written before
// today's rules: extra open ones on a lead, and subjects fixed in English.
package qualifytasks

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
)

// DuplicatesDataTask keeps the newest open qualify task on each lead and
// retires the rest as of now.
func DuplicatesDataTask(now func() time.Time) datatask.Task {
	return datatask.Task{
		ID: "core/1794740089_one_open_qualify_task_per_lead", Version: 1,
		Title: systemtext.Literal("datatask.title.core.1794740089_one_open_qualify_task_per_lead"),
		Steps: []datatask.Step{{
			Name: "retire_duplicate_qualify_tasks", Limit: 50, MinLimit: 1,
			Audit: datatask.AuditPolicy{PerRecord: true},
			Estimate: func(ctx context.Context, q datatask.Querier, maximum int64) (int64, bool, error) {
				return activities.CountLeadsWithDuplicateTasks(ctx, q, provenance.LeadRepliedSource, maximum)
			},
			Batch: func(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
				after, err := datatask.UUIDCursor(in.Cursor)
				if err != nil {
					return datatask.BatchOut{}, err
				}
				leads, changed, err := activities.RetireDuplicateLeadTasksTx(ctx, tx, provenance.LeadRepliedSource, after, in.Limit, now())
				if err != nil {
					return datatask.BatchOut{}, err
				}
				return datatask.AdvanceUUID(in, leads, changed)
			},
		}},
	}
}
