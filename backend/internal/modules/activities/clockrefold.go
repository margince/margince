// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// Which activities' time has come since the clock refold last ran. The
// last-activity clocks count only what has happened. No row changes when a
// booked meeting's start passes, so the refold job asks this module which
// records those activities are filed under.

import (
	"bytes"
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
	At time.Time `json:"passed_up_to"`
	ID ids.UUID  `json:"passed_id"`
}

// before orders two marks the way the walk orders activities: by instant, then by id.
func (m ClockRefoldMark) before(other ClockRefoldMark) bool {
	if order := m.At.Compare(other.At); order != 0 {
		return order < 0
	}
	return bytes.Compare(m.ID[:], other.ID[:]) < 0
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
	mark, stored, err := storekit.InstallationStateTx[ClockRefoldMark](ctx, tx, storekit.StateActivityClockRefoldMark)
	if err != nil {
		return ClockRefoldMark{}, time.Time{}, fmt.Errorf("activities: reading the clock refold mark: %w", err)
	}
	var asOf time.Time
	var migrated *time.Time
	err = tx.QueryRow(ctx, `
		SELECT (SELECT applied_at FROM schema_migrations_core WHERE version = $2),
		       least(now() - make_interval(secs => $1),
		             (SELECT min(xact_start) FROM pg_stat_activity
		               WHERE datname = current_database() AND pid <> pg_backend_pid()))`,
		clockRefoldSettle.Seconds(), clockRefoldMigration).Scan(&migrated, &asOf)
	if err != nil {
		return ClockRefoldMark{}, time.Time{}, fmt.Errorf("activities: reading the clock refold window: %w", err)
	}
	if !stored && migrated != nil {
		mark.At = *migrated
	}
	return mark, asOf, nil
}

// PassedActivitiesTx reads the next batch after the mark and up to asOf. A
// batch shorter than limit reaches asOf, so Through moves the mark there.
func PassedActivitiesTx(ctx context.Context, tx pgx.Tx, after ClockRefoldMark, asOf time.Time, limit int) (PassedActivities, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return PassedActivities{}, err
	}
	out, err := refoldTargetsTx(ctx, tx, `
		  SELECT a.id, a.occurred_at FROM activity a
		   WHERE a.archived_at IS NULL
		     AND a.created_at < a.occurred_at -- occurred_at raw: written ahead of its time, a row the refold re-reads
		     AND (a.occurred_at, a.id) > ($1, $2)
		     AND a.occurred_at <= $3 -- occurred_at raw: the keyset bound of the walk; DayActivitiesComeTx reads day entries
		   ORDER BY a.occurred_at, a.id
		   LIMIT $4`, after.At, after.ID, asOf, limit)
	if err != nil {
		return PassedActivities{}, err
	}
	if out.Count < limit {
		out.Through = ClockRefoldMark{At: asOf, ID: clockRefoldEnd}
	}
	return out, nil
}

// DayActivitiesComeTx reads the next batch of day entries the (occurred_at, id)
// walk cannot time, after the given key. A day's moment is its date in zone,
// not its stored noon. Another kind has begun from its date's start, which in
// every zone east of UTC-12 is before the noon the walk waits for. So the run its
// date begins reads it. A meeting takes place when its date ends, before its
// noon east of UTC+12 and after it elsewhere. So the arm reads every meeting
// whose date ended since the mark, and a refold the walk repeats changes nothing.
// A batch shorter than limit is the last.
func DayActivitiesComeTx(
	ctx context.Context, tx pgx.Tx, mark ClockRefoldMark, asOf time.Time, zone *time.Location, after ClockRefoldMark, limit int,
) (PassedActivities, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return PassedActivities{}, err
	}
	today, markDay := occurredday.AnchorIn(asOf, zone), occurredday.AnchorIn(mark.At, zone)
	return refoldTargetsTx(ctx, tx, `
		SELECT a.id, a.occurred_at FROM activity a
		 WHERE a.archived_at IS NULL AND a.occurred_precision = 'day'
		   AND ((a.kind <> $1 AND a.occurred_at = $2 AND $4 < $2
		         AND a.created_at < a.occurred_at -- occurred_at raw: written ahead of its time, a row the refold re-reads
		         AND a.occurred_at > $3) -- occurred_at raw: past the bound of the walk, at the anchor of today in zone
		     OR (a.kind = $1 AND `+occurredday.DatesSQL("a", "$4", "$2")+`))
		   AND (a.occurred_at, a.id) > ($5, $6)
		 ORDER BY a.occurred_at, a.id
		 LIMIT $7`,
		KindMeeting, today, asOf, markDay, after.At, after.ID, limit)
}

// refoldTargetsTx reads the records the activities passed selects are filed
// under. passed is a SELECT of id and occurred_at; Through is the last of them.
func refoldTargetsTx(ctx context.Context, tx pgx.Tx, passed string, args ...any) (PassedActivities, error) {
	rows, err := tx.Query(ctx, `WITH passed AS (`+passed+`)
		SELECT p.id, p.occurred_at, l.contact_id, l.deal_id, l.company_id, l.project_id
		  FROM passed p LEFT JOIN activity_link l ON l.activity_id = p.id
		 ORDER BY p.occurred_at, p.id`, args...)
	if err != nil {
		return PassedActivities{}, fmt.Errorf("activities: reading activities whose time has come: %w", err)
	}
	defer rows.Close()
	var out PassedActivities
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
	return out, nil
}

// AdvanceClockRefoldMarkTx stores the mark once the batch before it is
// refolded. The mark never moves back.
func AdvanceClockRefoldMarkTx(ctx context.Context, tx pgx.Tx, to ClockRefoldMark) error {
	if err := auth.RequireSystem(ctx); err != nil {
		return err
	}
	// The row lock makes two runs take turns, so the later one compares
	// against what the earlier one stored.
	held, err := storekit.LockInstallationStateTx(ctx, tx, storekit.StateActivityClockRefoldMark, to)
	if err != nil {
		return fmt.Errorf("activities: reading the clock refold mark: %w", err)
	}
	if !held.before(to) {
		return nil
	}
	if err := storekit.StoreInstallationStateTx(ctx, tx, storekit.StateActivityClockRefoldMark, to); err != nil {
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
