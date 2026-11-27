// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package standingline

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/events"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// backlogWindow skips replayed history: a new consumer group starts at the
// stream's beginning, and an old activity is not news.
const backlogWindow = 7 * 24 * time.Hour

const activityUpdated = "activity.updated"

// Enqueue queues one check of the activity, in the transaction it is given.
type Enqueue func(ctx context.Context, tx pgx.Tx, workspace ids.WorkspaceID, activity ids.UUID) error

// Trigger is the cg:standing-line consumer: one check for each activity that
// lands or changes.
type Trigger struct {
	pool      *pgxpool.Pool
	enqueue   Enqueue
	workspace func(context.Context) (ids.WorkspaceID, error)
	log       *slog.Logger
	now       func() time.Time
}

// NewTrigger builds the consumer over the installation's one workspace.
func NewTrigger(
	pool *pgxpool.Pool, enqueue Enqueue, workspace func(context.Context) (ids.WorkspaceID, error), log *slog.Logger,
) *Trigger {
	return &Trigger{pool: pool, enqueue: enqueue, workspace: workspace, log: log, now: time.Now}
}

// HandleEvent queues one check for an activity that may move the picture.
func (t *Trigger) HandleEvent(ctx context.Context, env events.Envelope) error {
	if !events.StandingLineTypes.Has(env.Type) || t.now().Sub(env.OccurredAt) > backlogWindow {
		return nil
	}
	if !t.moves(ctx, env) {
		return nil
	}
	ws, err := t.workspace(ctx)
	if err != nil {
		return err
	}
	return pgx.BeginFunc(ctx, t.pool, func(tx pgx.Tx) error {
		return t.enqueue(ctx, tx, ws, env.Entity.ID)
	})
}

// moves says whether the event can change where a record stands. That is a
// new activity other than a task, or one filed or marked held later. Margince
// files many tasks of its own, and what is owed is on the page.
func (t *Trigger) moves(ctx context.Context, env events.Envelope) bool {
	if env.Type == activityUpdated {
		var updated crmcontracts.PublicEventActivityUpdated
		if err := json.Unmarshal(env.Payload, &updated); err != nil {
			t.log.WarnContext(ctx, "standing line trigger: unreadable payload", "event", env.EventID.String(), "err", err)
			return false
		}
		return updated.ChangedFields.Relinked != nil || updated.ChangedFields.MeetingStatus != nil
	}
	var captured crmcontracts.PublicEventActivityCaptured
	if err := json.Unmarshal(env.Payload, &captured); err != nil {
		t.log.WarnContext(ctx, "standing line trigger: unreadable payload", "event", env.EventID.String(), "err", err)
		return false
	}
	return captured.Kind != string(crmcontracts.ActivityKindTask)
}
