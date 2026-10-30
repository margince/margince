// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The open tasks a closing deal leaves behind, for the surface that closes it.
//
// A task on a closed deal can still be real work, so the close asks. The web
// asked by judging "open" in the client, which left the tool unable to ask.
//
// The settle write is activities.SetTaskDoneTx, what the bulk change applies
// per row, so completing here leaves the audit rows the web leaves.
//
// This reads after the move; the web reads before, to fill its dialog.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// dealCloseTasks answers and settles a closing deal's open tasks.
type dealCloseTasks struct {
	tasks *activities.Store
	db    *database.DB
}

func newDealCloseTasks(tasks *activities.Store, db *database.DB) dealCloseTasks {
	return dealCloseTasks{tasks: tasks, db: db}
}

// openTasksOf lists the deal's open tasks through the activities reader. The
// caller's row scope and field masks are then the reader's own.
func (d dealCloseTasks) openTasksOf(ctx context.Context, dealID ids.UUID) ([]crmcontracts.Activity, error) {
	kind, entity := "task", "deal"
	rows, _, err := d.tasks.ListActivities(ctx, activities.ListActivitiesInput{
		Kind: &kind, EntityType: &entity, EntityID: &dealID, ReadableOnly: true,
	})
	if err != nil {
		return nil, fmt.Errorf("compose: reading a closing deal's open tasks: %w", err)
	}
	open := make([]crmcontracts.Activity, 0, len(rows))
	for _, row := range rows {
		// A task with no is_done is not an open task. The column is the
		// record's own answer. Its absence means the row is not one to settle.
		if row.IsDone != nil && !*row.IsDone {
			open = append(open, row)
		}
	}
	return open, nil
}

// OpenTasksOnDeal reports the deal's open tasks for a close result.
func (d dealCloseTasks) OpenTasksOnDeal(ctx context.Context, dealID ids.UUID) ([]crmcontracts.DealOpenTask, error) {
	rows, err := d.openTasksOf(ctx, dealID)
	if err != nil {
		return nil, err
	}
	out := make([]crmcontracts.DealOpenTask, 0, len(rows))
	for _, row := range rows {
		task := crmcontracts.DealOpenTask{Id: row.Id, DueAt: row.DueAt}
		if row.Subject != nil {
			task.Subject = *row.Subject
		}
		out = append(out, task)
	}
	return out, nil
}

// CompleteOpenTasksOnDeal ticks the deal's open tasks done and answers the ones
// it settled.
//
// One transaction, so a caller never learns a settle half happened. A task
// whose version moved between the read and the write is left alone rather than
// forced, the answer the bulk change gives it.
func (d dealCloseTasks) CompleteOpenTasksOnDeal(ctx context.Context, dealID ids.UUID) ([]ids.UUID, error) {
	var settled []ids.UUID
	err := d.db.Tx(ctx, func(tx pgx.Tx) error {
		rows, err := d.openTasksOf(ctx, dealID)
		if err != nil {
			return err
		}
		settled = make([]ids.UUID, 0, len(rows))
		for _, row := range rows {
			if row.Version == nil {
				continue
			}
			id := ids.From[ids.ActivityKind](ids.UUID(row.Id))
			if _, err := d.tasks.SetTaskDoneTx(ctx, tx, id, true, *row.Version); err != nil {
				return fmt.Errorf("compose: completing a closing deal's task: %w", err)
			}
			settled = append(settled, ids.UUID(row.Id))
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return settled, nil
}
