// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/signals"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
)

// newslettersDataTask takes back what a newsletter left behind before it
// stopped counting as a touch: the stored last-activity clocks it moved, the
// quiet reminders it closed and the ghosted-thread signals it raised.
func newslettersDataTask(pool *pgxpool.Pool) datatask.Task {
	audit := datatask.AuditPolicy{Reason: "the last-activity triggers move the same derived clock without an audit"}
	return datatask.Task{
		ID: "core/1794394463_newsletters_stop_counting", Version: 1,
		Title: systemtext.Literal("datatask.title.core.1794394463_newsletters_stop_counting"), Requires: []string{"core/1794394463"},
		Steps: []datatask.Step{
			clockRefoldStep("contacts", storekit.ClockContacts, storekit.ClockNotFolded, audit),
			clockRefoldStep("companies", storekit.ClockCompanies, storekit.ClockNotFolded, audit),
			clockRefoldStep("deals", storekit.ClockDeals, storekit.ClockNotFolded, audit),
			clockRefoldStep("projects", storekit.ClockProjects, storekit.ClockNotFolded, audit),
			newsletterRemindersStep(activities.NewStore(InstallationDB(pool))),
			newsletterSignalsStep(),
		},
	}
}

// newsletterRemindersStep reopens the quiet reminders the engine closed on a
// newsletter alone.
func newsletterRemindersStep(tasks *activities.Store) datatask.Step {
	return datatask.Step{
		Name: "quiet_reminders", Limit: 50, MinLimit: 5,
		Audit: datatask.AuditPolicy{PerRecord: true},
		Estimate: func(ctx context.Context, q datatask.Querier, maximum int64) (int64, bool, error) {
			if err := auth.Require(ctx, "data_task", principal.ActionRead); err != nil {
				return 0, false, err
			}
			count, err := activities.CountQuietReminders(ctx, q, maximum)
			return min(count, maximum), count > maximum, err
		},
		Batch: func(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
			after, err := datatask.UUIDCursor(in.Cursor)
			if err != nil {
				return datatask.BatchOut{}, err
			}
			inspected, reopened, err := tasks.ReopenNewsletterClosedRemindersTx(ctx, tx, after, in.Limit)
			if err != nil {
				return datatask.BatchOut{}, err
			}
			return datatask.AdvanceUUID(in, inspected, reopened)
		},
	}
}

// newsletterSignalsStep archives the ghosted-thread signals that cite a
// newsletter as the last word on the thread.
func newsletterSignalsStep() datatask.Step {
	return datatask.Step{
		Name: "ghosted_signals", Limit: 200, MinLimit: 10,
		Audit: datatask.AuditPolicy{PerRecord: true},
		Estimate: func(ctx context.Context, q datatask.Querier, maximum int64) (int64, bool, error) {
			if err := auth.Require(ctx, "data_task", principal.ActionRead); err != nil {
				return 0, false, err
			}
			count, err := signals.CountDerivedSignalsOfKind(ctx, q, kindGhostedThread, maximum)
			return min(count, maximum), count > maximum, err
		},
		Batch: func(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
			after, err := datatask.UUIDCursor(in.Cursor)
			if err != nil {
				return datatask.BatchOut{}, err
			}
			inspected, archived, err := signals.ArchiveCitingBulkMailTx(ctx, tx, kindGhostedThread, after, in.Limit)
			if err != nil {
				return datatask.BatchOut{}, err
			}
			return datatask.AdvanceUUID(in, inspected, archived)
		},
	}
}
