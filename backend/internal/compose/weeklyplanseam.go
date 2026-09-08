// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The one edge between the week just gone and the week ahead.
//
// compose/weekly owns weekly_review and weeklyplan owns the plan tables, and
// neither may write the other's. So the retrospective asks the plan what its
// week came to and writes the answer into its own row — two integers across the
// seam, and the plan's rows stay the plan's business.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/compose/weekly"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/modules/weeklyplan"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

type weeklyPlanOutcome struct{ store *weeklyplan.Store }

var _ weekly.WeekPlan = weeklyPlanOutcome{}

func (o weeklyPlanOutcome) CloseWeek(ctx context.Context, now time.Time) (int, int, error) {
	outcome, err := o.store.CloseWeek(ctx, now)
	if err != nil {
		return 0, 0, err
	}
	return outcome.Due, outcome.Kept, nil
}

// weeklyPlanStore builds the plan store.
//
// ONE spelling of "which Monday": the plan and the review beside it must be
// about the same seven days, and compose/weekly owns that answer. A module may
// not import compose, so it takes the function.
func weeklyPlanStore(pool *pgxpool.Pool) *weeklyplan.Store {
	return weeklyplan.NewStore(InstallationDB(pool), weekly.WeekStartOf, newTeammatesSeam(pool)).
		WithCapacity(weeklyPlanCapacity{pool: pool})
}

// weeklyPlanCapacity counts what next week already holds for one rep.
//
// It lives here because the meetings and the tasks belong to other modules and
// weeklyplan may not read their tables. Two counts across the seam, and the
// plan's own rows stay the plan's business — the same trade weeklyPlanOutcome
// above makes in the other direction.
type weeklyPlanCapacity struct{ pool *pgxpool.Pool }

var _ weeklyplan.Capacity = weeklyPlanCapacity{}

// ForWeek counts booked meetings and open tasks due in ONE named week's local
// window — the week the plan is about, which the caller names.
//
// Named rather than derived: this seam used to compute "next week" from the
// clock, so a plan stored against the current week was priced against the week
// after it, and the heading, the stored row and this line each answered about a
// different seven days.
//
// Booked only, never held. The question is what is COMMITTED — what a rep must
// leave room for — and an outcome is not a commitment: a held meeting is time
// already spent rather than time still owed, and counting it would price a week
// against work that is finished. Canceled and no_show are absent for the same
// reason, from the other side.
//
// The window is the LOCAL week resolved to instants, through weekly's own
// WeekStartOf, so the plan and the review beside it agree about which seven
// days these are.
func (c weeklyPlanCapacity) ForWeek(
	ctx context.Context, owner ids.UUID, weekStart time.Time,
) (weeklyplan.Committed, error) {
	var out weeklyplan.Committed
	err := database.WithWorkspaceTx(ctx, c.pool, func(tx pgx.Tx) error {
		zone, err := identity.TimezoneOf(ctx, tx)
		if err != nil {
			return err
		}
		// A calendar date carried as midnight UTC is the wrong shape for a
		// range: comparing timestamptz against it measures a week offset by the
		// installation's UTC offset, and across a DST change a fixed 168 hours
		// rather than the week people will live.
		var start, end time.Time
		if err := tx.QueryRow(ctx, `
			SELECT ($1::date)::timestamp AT TIME ZONE $2,
			       ($1::date + 7)::timestamp AT TIME ZONE $2`, weekStart, zone).
			Scan(&start, &end); err != nil {
			return fmt.Errorf("compose: bounding next week: %w", err)
		}
		return tx.QueryRow(ctx, `
			SELECT
			  (SELECT count(*) FROM activity m
			    WHERE m.kind = 'meeting' AND m.archived_at IS NULL
			      AND m.meeting_status = 'booked'
			      -- By HOST, falling back to the capturer only where no host is
			      -- recorded. A meeting a colleague booked or a calendar
			      -- connector imported is still this rep's time, and reading
			      -- the capturer alone left it out of the capacity they plan
			      -- against — the emptier the calendar looks, the more they
			      -- commit to.
			      AND (m.host_user_id = $4
			           OR (m.host_user_id IS NULL AND m.captured_by = $3))
			      AND m.occurred_at >= $1 AND m.occurred_at < $2),
			  (SELECT count(*) FROM activity t
			    WHERE t.kind = 'task' AND t.archived_at IS NULL
			      AND NOT t.is_done AND t.assignee_id = $4
			      AND t.due_at >= $1 AND t.due_at < $2)`,
			start, end, "human:"+owner.String(), owner).
			Scan(&out.Meetings, &out.Tasks)
	})
	if err != nil {
		return weeklyplan.Committed{}, err
	}
	return out, nil
}
