// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// Which activities' time has come since the clock refold last ran. The
// last-activity clocks count only what has happened. No row changes when a
// booked meeting's start passes, so the refold job asks this module which
// records those activities are filed under.

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/occurredday"
)

// clockRefoldSettle holds the window back from now, so a write that started
// just before the refold reads has committed before the mark passes it.
const clockRefoldSettle = 5 * time.Minute

// clockRefoldMigration is the migration that made the clocks count only the
// past. Every activity dated before it ran was already in the stored clocks,
// so a run with no mark stored yet starts there.
const clockRefoldMigration = "1792656701"

// ClockRefoldMark is how far the refold has come, in (occurred_at, id) order.
type ClockRefoldMark struct {
	At time.Time
	ID ids.UUID
}

// clockRefoldEnd sorts after every id at one instant, so a mark set to it
// passes all of that instant's activities.
var clockRefoldEnd = ids.MustParse("ffffffff-ffff-ffff-ffff-ffffffffffff")

// PassedActivities is one batch of activities written before they happened
// whose time has now come, and the records they are filed under.
type PassedActivities struct {
	Targets storekit.LastActivityTargets
	// Through is where the mark goes once these records are refolded.
	Through ClockRefoldMark
	Count   int
}

// ClockRefoldStartTx answers the stored mark and the instant this run reads up
// to. The instant is the database's, because the clock functions read now().
// It never passes the start of a transaction still open. That writer's
// trigger left out what was ahead of its now(), and its rows are not visible
// yet. With no mark stored, the run starts when clockRefoldMigration ran.
func ClockRefoldStartTx(ctx context.Context, tx pgx.Tx) (ClockRefoldMark, time.Time, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return ClockRefoldMark{}, time.Time{}, err
	}
	var mark ClockRefoldMark
	var asOf time.Time
	err := tx.QueryRow(ctx, `
		SELECT coalesce(m.passed_up_to,
		                (SELECT applied_at FROM schema_migrations_core WHERE version = $2),
		                '0001-01-01 00:00:00+00'),
		       coalesce(m.passed_id, '00000000-0000-0000-0000-000000000000'),
		       least(now() - make_interval(secs => $1),
		             (SELECT min(xact_start) FROM pg_stat_activity
		               WHERE datname = current_database() AND pid <> pg_backend_pid()))
		  FROM (SELECT) one LEFT JOIN activity_clock_refold_mark m ON true`,
		clockRefoldSettle.Seconds(), clockRefoldMigration).Scan(&mark.At, &mark.ID, &asOf)
	if err != nil {
		return ClockRefoldMark{}, time.Time{}, fmt.Errorf("activities: reading the clock refold mark: %w", err)
	}
	return mark, asOf, nil
}

// PassedActivitiesTx reads the next batch after the mark and up to asOf. A
// batch shorter than limit reaches asOf, so Through moves the mark there.
func PassedActivitiesTx(ctx context.Context, tx pgx.Tx, after ClockRefoldMark, asOf time.Time, limit int) (PassedActivities, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return PassedActivities{}, err
	}
	rows, err := tx.Query(ctx, `
		WITH passed AS (
		  SELECT a.id, a.occurred_at FROM activity a
		   WHERE a.archived_at IS NULL AND a.created_at < a.occurred_at
		     AND (a.occurred_at, a.id) > ($1, $2)
		     AND a.occurred_at <= $3 -- occurred_at raw: the keyset bound of the walk; DayActivitiesComeTx reads day entries
		   ORDER BY a.occurred_at, a.id
		   LIMIT $4)
		SELECT p.id, p.occurred_at, l.contact_id, l.deal_id, l.company_id, l.project_id
		  FROM passed p LEFT JOIN activity_link l ON l.activity_id = p.id
		 ORDER BY p.occurred_at, p.id`, after.At, after.ID, asOf, limit)
	if err != nil {
		return PassedActivities{}, fmt.Errorf("activities: reading activities whose time has come: %w", err)
	}
	defer rows.Close()
	out := PassedActivities{Through: ClockRefoldMark{At: asOf, ID: clockRefoldEnd}}
	var last ids.UUID
	for rows.Next() {
		var id ids.UUID
		var at time.Time
		var contact, deal, company, project *ids.UUID
		if err := rows.Scan(&id, &at, &contact, &deal, &company, &project); err != nil {
			return PassedActivities{}, fmt.Errorf("activities: reading activities whose time has come: %w", err)
		}
		if out.Count == 0 || id != last {
			out.Count++
			last = id
			out.Through = ClockRefoldMark{At: at, ID: id}
		}
		out.Targets.Contacts = appendSet(out.Targets.Contacts, contact)
		out.Targets.Deals = appendSet(out.Targets.Deals, deal)
		out.Targets.Companies = appendSet(out.Targets.Companies, company)
		out.Targets.Projects = appendSet(out.Targets.Projects, project)
	}
	if err := rows.Err(); err != nil {
		return PassedActivities{}, fmt.Errorf("activities: reading activities whose time has come: %w", err)
	}
	if out.Count < limit {
		out.Through = ClockRefoldMark{At: asOf, ID: clockRefoldEnd}
	}
	return out, nil
}

// DayRefoldTargets are the records day entries are filed under whose time came
// since the mark without the walk passing them.
type DayRefoldTargets struct {
	Targets storekit.LastActivityTargets
	Count   int
}

// DayActivitiesComeTx reads the day entries the (occurred_at, id) walk cannot
// time: a day's moment is its date in zone, not its stored noon. Another kind
// has begun from its date's start. East of UTC that is before the noon the walk
// waits for, so the run its date begins reads it. A meeting takes place when its
// date ends. West of UTC that is after the noon the walk passed, so the arm
// reads the meetings whose date ended since the mark. Each arm reads a date or two.
func DayActivitiesComeTx(ctx context.Context, tx pgx.Tx, mark ClockRefoldMark, asOf time.Time, zone *time.Location) (DayRefoldTargets, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return DayRefoldTargets{}, err
	}
	today, markDay := occurredday.AnchorIn(asOf, zone), occurredday.AnchorIn(mark.At, zone)
	rows, err := tx.Query(ctx, `
		SELECT a.id, l.contact_id, l.deal_id, l.company_id, l.project_id
		  FROM activity a LEFT JOIN activity_link l ON l.activity_id = a.id
		 WHERE a.archived_at IS NULL AND a.occurred_precision = 'day'
		   AND ((a.kind <> $1 AND a.occurred_at = $2 AND $4 < $2 AND a.created_at < a.occurred_at
		         AND a.occurred_at > $3) -- occurred_at raw: past the bound of the walk, at the anchor of today in zone
		     OR (a.kind = $1 AND a.occurred_at >= $4 AND a.occurred_at < $2
		         AND a.occurred_at <= $5 -- occurred_at raw: the walk passed it, its date ended since
		   ))`,
		KindMeeting, today, asOf, markDay, mark.At)
	if err != nil {
		return DayRefoldTargets{}, fmt.Errorf("activities: reading day entries whose time has come: %w", err)
	}
	defer rows.Close()
	var out DayRefoldTargets
	seen := map[ids.UUID]bool{}
	for rows.Next() {
		var id ids.UUID
		var contact, deal, company, project *ids.UUID
		if err := rows.Scan(&id, &contact, &deal, &company, &project); err != nil {
			return DayRefoldTargets{}, fmt.Errorf("activities: reading day entries whose time has come: %w", err)
		}
		if !seen[id] {
			seen[id] = true
			out.Count++
		}
		out.Targets.Contacts = appendSet(out.Targets.Contacts, contact)
		out.Targets.Deals = appendSet(out.Targets.Deals, deal)
		out.Targets.Companies = appendSet(out.Targets.Companies, company)
		out.Targets.Projects = appendSet(out.Targets.Projects, project)
	}
	if err := rows.Err(); err != nil {
		return DayRefoldTargets{}, fmt.Errorf("activities: reading day entries whose time has come: %w", err)
	}
	return out, nil
}

// AdvanceClockRefoldMarkTx stores the mark once the batch before it is
// refolded. The mark never moves back.
func AdvanceClockRefoldMarkTx(ctx context.Context, tx pgx.Tx, to ClockRefoldMark) error {
	if err := auth.RequireSystem(ctx); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO activity_clock_refold_mark (passed_up_to, passed_id) VALUES ($1, $2)
		ON CONFLICT (singleton) DO UPDATE SET passed_up_to = EXCLUDED.passed_up_to, passed_id = EXCLUDED.passed_id
		 WHERE (activity_clock_refold_mark.passed_up_to, activity_clock_refold_mark.passed_id) < (EXCLUDED.passed_up_to, EXCLUDED.passed_id)`,
		to.At, to.ID); err != nil {
		return fmt.Errorf("activities: advancing the clock refold mark: %w", err)
	}
	return nil
}

// appendSet adds id to set once; a link names one record, so most are nil.
func appendSet(set []ids.UUID, id *ids.UUID) []ids.UUID {
	if id == nil || slices.Contains(set, *id) {
		return set
	}
	return append(set, *id)
}
