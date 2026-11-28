// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package jobs

// Reading the job table, rather than working it. Every SQL statement over
// river_job that serves a reader lives here, so the operational and the
// admin surface cannot drift into separate state, error and scoping
// vocabularies over one table that has no RLS to hold them together.

import (
	"context"
	"fmt"
	"maps"
	"slices"

	"github.com/jackc/pgx/v5/pgxpool"
)

// SweepTag marks a job row as one share of a fleet pass. Every site that fans
// work out stamps it. A job enqueued by a user action carries no tag and is not
// counted as a fleet pass.
//
// Nothing enforces that a new fan-out remembers the tag. An untagged one is
// absent from the sweep gauges. Deriving that obligation needs a static notion
// of a fan-out insert, which the tree does not support.
const SweepTag = "sweep"

// sweepTagPredicate is the tag test, spelled once. River stores tags in a
// varchar(255)[] column, so membership is the array operator rather than a
// join.
const sweepTagPredicate = `'` + SweepTag + `' = ANY(tags)`

// runnableStates are the states a row can be worked FROM right now. The
// scheduled_at <= now() test that always accompanies this is what makes
// including 'scheduled' correct rather than misleading: a scheduled row
// whose time has passed IS runnable and unclaimed, and excluding it would
// let a stopped scheduler read as a perfectly healthy queue on the one
// gauge whose job is to catch exactly that. A row scheduled for the future
// fails the time test and contributes nothing, so no age is ever measured
// backwards.
const runnableStates = `('available','retryable','scheduled')`

// terminalBadStates are the states a pass can end in having done nothing.
// cancelled counts: a cancelled pass did not run, whatever the reason, and the
// sweep pair answers "did the pass run".
const terminalBadStates = `('discarded','cancelled')`

// terminalStates are the states a run has ENDED in, good or bad. A row in any
// other state has not finished, so it answers nothing about how the pass went —
// see statsBySweep, where reading an unfinished row as the pass's outcome is the
// blind spot this names.
const terminalStates = `('completed','discarded','cancelled')`

// StateRow is one (queue, kind, state) group of the job table as it stands
// right now.
type StateRow struct {
	Queue string
	Kind  string
	State string
	Count int64
	// OldestRunnableAgeSeconds is how long the oldest job in this group has
	// been ELIGIBLE and unclaimed, and is NIL when the group holds no such
	// job. Nil and zero are different claims — nothing is runnable versus
	// something became runnable a moment ago — and flattening them would
	// report a queue of future-scheduled work as "the oldest runnable job
	// has waited 0 seconds", which is a statement about a job that does not
	// exist. The endpoint carries the same distinction.
	OldestRunnableAgeSeconds *float64
}

// SweepPass is one fleet-pass kind read as a whole: whether it has a surviving
// tagged row, and whether the latest of those that ended failed. An installation
// holds one company, so a kind has one pass. Passes is 1 when the kind has a
// tagged row. Failed is 1 when the latest ended one is dead.
type SweepPass struct {
	Kind   string
	Passes int64
	Failed int64
}

// SweepUnit is one fan-out kind read per fan-out unit, the grain the
// dispatcher ran the pass at. It covers the kinds whose unit is finer than the
// installation.
//
// It exists because SweepPass counts one pass per kind, and three dispatchers
// fan out per connection and one per build. If one connection's child fails and
// another then succeeds, the kind's latest child is the successful one, and the
// kind pair reports nothing. Counting at the declared unit makes it visible.
//
// Kinds whose unit is the installation are not reported, because this pair
// would restate SweepPass value for value. A per-connection kind is reported by
// both pairs at two grains, so the two must never be summed.
type SweepUnit struct {
	Kind string
	Unit FanOutUnit
	// Units is how many distinct units of this kind have a surviving child,
	// and Failed how many of those most recently ended dead. The pair reads
	// exactly as SweepPass's does, one grain down.
	Units  int64
	Failed int64
}

// Snapshot is one read of the job table, for a reader that renders it.
type Snapshot struct {
	Rows   []StateRow
	Sweeps []SweepPass
	// Units is empty when no declared kind fans out below the installation.
	// That is a legitimate fleet, not a failed read. Stats returns the error
	// for a failed read.
	Units []SweepUnit
	// Failures is the failing rows grouped by kind and by the sentence they
	// recorded — the input the exposition classifies. Empty is a fleet with
	// nothing failing, which is the healthy case and not a failed read.
	Failures []FailureCount
}

// Stats reads the live job table for the metric surface.
//
// THREE statements, not one. They read different populations — the runtime
// gauges must exclude completed rows (a finished job is history, not depth)
// while both sweep reads must include them (a pass whose units all succeeded
// is the healthy case, and it is completed rows that say so) — and the two
// sweep reads group at different grains besides. Folding them together needs
// a UNION with a discriminator and several nullable halves; three legible
// grouped scans inside one budget is the better trade.
//
// A caller that could not complete the read gets the error, never a partial
// or empty Snapshot: an unmeasured fleet renders identically to an idle one,
// and telling those apart is the whole point of the surface.
func Stats(ctx context.Context, pool *pgxpool.Pool) (Snapshot, error) {
	rows, err := statsByState(ctx, pool)
	if err != nil {
		return Snapshot{}, err
	}
	sweeps, err := statsBySweep(ctx, pool)
	if err != nil {
		return Snapshot{}, err
	}
	units, err := statsBySweepUnit(ctx, pool)
	if err != nil {
		return Snapshot{}, err
	}
	failures, err := statsByFailure(ctx, pool)
	if err != nil {
		return Snapshot{}, err
	}
	return Snapshot{Rows: rows, Sweeps: sweeps, Units: units, Failures: failures}, nil
}

// subWorkspaceFanOuts answers the fan-out child kinds whose declared unit is
// finer than the installation, paired with the args key that identifies one.
// statsBySweepUnit joins the job table against the two arrays.
//
// It derives the pairs from the contract on every call. A dispatcher that
// changes its fan_out_unit, or a new one that fans out per connection, is
// enrolled by its declaration. Both arrays come from one pass, which keeps
// them index-aligned.
func subWorkspaceFanOuts() (kinds, argsKeys []string) {
	return subWorkspaceFanOutsOf(FanOutUnits())
}

// subWorkspaceFanOutsOf is the selection itself, over a table it is handed, so
// a test can build the table. An installation-grain fan-out stays out of the
// unit pair, because margince_sweep_workspaces already states that number.
func subWorkspaceFanOutsOf(units map[string]FanOutUnit) (kinds, argsKeys []string) {
	for _, kind := range slices.Sorted(maps.Keys(units)) {
		key := units[kind].ArgsKey()
		if units[kind] == FanOutInstallation || key == "" {
			continue
		}
		kinds = append(kinds, kind)
		argsKeys = append(argsKeys, key)
	}
	return kinds, argsKeys
}

// fanOutDispatcherKinds answers the kinds statsBySweep leaves out: a dispatcher
// that fans out to children. Its own row enqueues and does no work, so the
// children it enqueues are the real signal.
func fanOutDispatcherKinds() []string {
	return fanOutDispatcherKindsOf(allSpecs())
}

// fanOutDispatcherKindsOf is the selection itself, over a table a test can
// build.
func fanOutDispatcherKindsOf(table map[string]Spec) []string {
	var kinds []string
	for _, kind := range slices.Sorted(maps.Keys(table)) {
		if table[kind].FanOutTo != "" {
			kinds = append(kinds, kind)
		}
	}
	return kinds
}

func statsByState(ctx context.Context, pool *pgxpool.Pool) ([]StateRow, error) {
	// The age comes from the database's own now(), never the app clock, because
	// the two differ by enough to move an exact assertion. The age has no
	// coalesce. A group with nothing runnable answers NULL, which the reader
	// carries as "no such job" and not as a measured zero.
	const q = `
		SELECT queue,
		       kind,
		       state::text,
		       count(*)::bigint,
		       max(EXTRACT(EPOCH FROM (now() - scheduled_at)))
		           FILTER (WHERE state::text IN ` + runnableStates + `
		                     AND scheduled_at <= now())::double precision
		FROM river_job
		WHERE state <> 'completed'
		GROUP BY 1, 2, 3`

	cursor, err := pool.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("jobs: reading job state counts: %w", err)
	}
	defer cursor.Close()

	var out []StateRow
	for cursor.Next() {
		var r StateRow
		if err := cursor.Scan(&r.Queue, &r.Kind,
			&r.State, &r.Count, &r.OldestRunnableAgeSeconds); err != nil {
			return nil, fmt.Errorf("jobs: scanning job state counts: %w", err)
		}
		out = append(out, r)
	}
	if err := cursor.Err(); err != nil {
		return nil, fmt.Errorf("jobs: reading job state counts: %w", err)
	}
	return out, nil
}

// statsBySweep reports, per fleet-pass kind, whether it has run and whether its
// latest outcome is a failure.
//
// It reads the latest outcome per kind, never a batch. This table has no
// "last pass". River resolves a uniqueness conflict with ON CONFLICT DO UPDATE
// SET kind = EXCLUDED.kind, which writes neither created_at nor metadata. A
// child still active from the previous pass is therefore deduplicated and
// produces no row for the current one.
//
// The sweep tag separates a fleet pass from a job someone triggered by hand. A
// fan-out dispatcher's own row is excluded: it enqueues and does no work, and
// its children are what this query counts.
//
// The two readings come from different rows of the same kind. Coverage counts
// any tagged row, a pending one included. The outcome reads the latest row in a
// terminal state and ignores one still on its way. Reading both from the newest
// row would let a tick that exhausted its attempts read as healthy while a
// successor existed. A periodic pass almost always has one. A later completed
// run still supersedes an earlier failure.
func statsBySweep(ctx context.Context, pool *pgxpool.Pool) ([]SweepPass, error) {
	dispatchers := fanOutDispatcherKinds()
	const q = `
		WITH tagged AS (
		    SELECT kind, state::text AS state, created_at, id
		    FROM river_job
		    WHERE ` + sweepTagPredicate + `
		      AND NOT (kind = ANY(coalesce($1::text[], ARRAY[]::text[])))
		),
		ended AS (
		    SELECT DISTINCT ON (kind) kind, state
		    FROM tagged
		    WHERE state IN ` + terminalStates + `
		    ORDER BY kind, created_at DESC, id DESC
		)
		SELECT t.kind,
		       1::bigint,
		       (count(e.kind) FILTER (WHERE e.state IN ` + terminalBadStates + `) > 0)::int::bigint
		FROM (SELECT DISTINCT kind FROM tagged) t
		LEFT JOIN ended e ON e.kind = t.kind
		GROUP BY t.kind`

	cursor, err := pool.Query(ctx, q, dispatchers)
	if err != nil {
		return nil, fmt.Errorf("jobs: reading sweep passes: %w", err)
	}
	defer cursor.Close()

	var out []SweepPass
	for cursor.Next() {
		var s SweepPass
		if err := cursor.Scan(&s.Kind, &s.Passes, &s.Failed); err != nil {
			return nil, fmt.Errorf("jobs: scanning sweep passes: %w", err)
		}
		out = append(out, s)
	}
	if err := cursor.Err(); err != nil {
		return nil, fmt.Errorf("jobs: reading sweep passes: %w", err)
	}
	return out, nil
}

// statsBySweepUnit is statsBySweep one grain down, for the kinds whose
// dispatcher fans out per connection or per build.
//
// The reading rule is the same: coverage from any tagged row of the unit, and
// the outcome from the latest row of it that ended. Only the key it is latest
// per changes, to args->>'connection_id' (or the build key) instead of the
// kind. That removes the masking. An installation with a healthy connection
// and a broken one is one healthy unit and one failed unit here. The kind pair
// sees only whichever child ran last.
//
// The kind-to-key pairing arrives as two aligned arrays and is joined against,
// not spliced into the SQL. A kind name and an args key never reach the
// statement as text. The join also bounds the scan to the declared unit kinds.
//
// An empty pairing means the contract declares no such dispatcher, and the
// query is skipped.
func statsBySweepUnit(ctx context.Context, pool *pgxpool.Pool) ([]SweepUnit, error) {
	kinds, argsKeys := subWorkspaceFanOuts()
	if len(kinds) == 0 {
		return nil, nil
	}

	const q = `
		WITH tagged AS (
		    SELECT j.kind, j.args->>u.args_key AS unit, j.state::text AS state,
		           j.created_at, j.id
		    FROM river_job j
		    JOIN unnest($1::text[], $2::text[]) AS u(kind, args_key) ON u.kind = j.kind
		    WHERE '` + SweepTag + `' = ANY(j.tags)
		      AND coalesce(j.args->>u.args_key, '') <> ''
		),
		grouped AS (
		    SELECT DISTINCT ON (kind, unit) kind, unit
		    FROM tagged ORDER BY kind, unit, created_at DESC, id DESC
		),
		ended AS (
		    SELECT DISTINCT ON (kind, unit) kind, unit, state
		    FROM tagged WHERE state IN ` + terminalStates + `
		    ORDER BY kind, unit, created_at DESC, id DESC
		)
		SELECT g.kind,
		       count(*)::bigint,
		       count(*) FILTER (WHERE e.state IN ` + terminalBadStates + `)::bigint
		FROM grouped g
		LEFT JOIN ended e ON e.kind = g.kind AND e.unit = g.unit
		GROUP BY g.kind`

	cursor, err := pool.Query(ctx, q, kinds, argsKeys)
	if err != nil {
		return nil, fmt.Errorf("jobs: reading sweep units: %w", err)
	}
	defer cursor.Close()

	units := FanOutUnits()
	var out []SweepUnit
	for cursor.Next() {
		var s SweepUnit
		if err := cursor.Scan(&s.Kind, &s.Units, &s.Failed); err != nil {
			return nil, fmt.Errorf("jobs: scanning sweep units: %w", err)
		}
		s.Unit = units[s.Kind]
		out = append(out, s)
	}
	if err := cursor.Err(); err != nil {
		return nil, fmt.Errorf("jobs: reading sweep units: %w", err)
	}
	return out, nil
}
