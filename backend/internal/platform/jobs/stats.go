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

// SweepTag marks a job row as one share of a fleet pass. Every
// site that fans work out to the fleet stamps it; a job enqueued by a user
// action carries no tag and is correctly not counted as a fleet pass.
//
// Nothing enforces that a new fan-out remembers the tag — an untagged one
// is simply absent from the sweep gauges. Deriving that obligation needs a
// static notion of "this insert is a fan-out" the tree does not support
// today.
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
// tagged row, and whether the latest of those that ENDED failed. An
// installation holds one company, so a kind has one pass to count: Passes is 1
// when the kind has a tagged row at all and Failed is 1 when the latest ended
// one is dead.
type SweepPass struct {
	Kind   string
	Passes int64
	Failed int64
}

// SweepUnit is one fan-out kind read per FAN-OUT UNIT — the grain the
// dispatcher actually ran the pass at — for the kinds whose unit is finer than
// the installation.
//
// It exists because SweepPass counts one pass per kind, and three dispatchers
// fan out per CONNECTION and one per BUILD. If one connection's child fails
// while another succeeds afterwards, the kind's most recent child is the
// successful one: the failure is real, the installation is being half-served,
// and the kind pair reports nothing. Counting at the declared unit is what
// makes that visible.
//
// Only the kinds whose unit is NOT the installation are reported. For the
// others the unit is the whole kind, so this pair would restate SweepPass value
// for value; publishing both would be one number twice.
//
// The two families therefore OVERLAP rather than partition: a per-connection
// kind is reported by both, at two grains. The two must never be summed, and an
// alert reads whichever grain it means.
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
	// Units is empty when no declared kind fans out below the installation,
	// which is a legitimate fleet rather than a failed read — Stats returns
	// the error for that.
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
// finer than the installation, paired with the args key that identifies one — the
// two arrays statsBySweepUnit joins the job table against.
//
// Derived from the contract on every call rather than kept as a list: a
// dispatcher that changes its fan_out_unit, or a new one that fans out per
// connection, is enrolled by the declaration that made it so. Both arrays are
// built in one pass so they stay index-aligned by construction.
func subWorkspaceFanOuts() (kinds, argsKeys []string) {
	return subWorkspaceFanOutsOf(FanOutUnits())
}

// subWorkspaceFanOutsOf is the selection itself, over a table it is handed.
//
// Separate from its caller so the partition can be exercised against a table a
// test builds. An installation-grain fan-out must be kept out of the unit pair,
// because margince_sweep_workspaces already states exactly that number.
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
	// The age is EXTRACTed from the database's own now(), never subtracted
	// from the app clock: the two clocks differ by enough to move an exact
	// assertion, which is a live intermittent flake elsewhere in this tree.
	// The age has NO coalesce: a group with nothing runnable answers NULL,
	// which the reader carries as "no such job" rather than as a measured
	// zero.
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

// statsBySweep reports, per fleet-pass kind, whether it has run and whether
// its latest outcome is a failure.
//
// It reads the LATEST OUTCOME PER KIND, never a batch. There is no such thing
// as "the last pass" in this table: River resolves a uniqueness conflict with
// ON CONFLICT DO UPDATE SET kind = EXCLUDED.kind, which writes neither
// created_at nor metadata, so a child still active from the previous pass is
// deduplicated and produces no row for the current one.
//
// The sweep tag is what separates a fleet pass from a job someone triggered by
// hand. A fan-out dispatcher's own row is excluded: it enqueues and does no
// work, and the children it enqueues are what this query counts for that kind.
//
// The two readings come from DIFFERENT rows of the same kind. Coverage asks
// "did the pass run", so it counts any tagged row, a pending one included. The
// outcome asks "what did the last attempt that ENDED do", so it reads the
// latest row in a terminal state and ignores one still on its way. Reading both
// from the newest row would let a tick that exhausted its attempts read as
// healthy for as long as a successor existed, since a periodic pass has a
// pending or running next tick almost all the time.
//
// A later COMPLETED run still supersedes an earlier failure: it is terminal, so
// it is the latest ended row.
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
// dispatcher fans out per connection or per build rather than per workspace.
//
// The reading rule is the SAME — coverage from any tagged row of the unit, the
// outcome from the latest row of it that ENDED, for the reason statsBySweep sets
// out in full — and only the key it is latest PER changes:
// args->>'connection_id' instead of the kind. That is what removes the
// masking. An installation with a healthy connection and a broken one is one
// healthy unit and one failed unit here, where the kind pair sees only
// whichever child ran last.
//
// The kind→key pairing arrives as two aligned arrays and is JOINED against
// rather than spliced into the SQL, so a kind name and an args key never reach
// the statement as text. The unnest join is also what bounds the scan to the
// declared sub-unit kinds: a row of any other kind matches no pair and is
// not read.
//
// An empty pairing means the contract declares no such dispatcher, and the
// query is skipped rather than run with two empty arrays — the answer is the
// same, and the skip says why it is empty.
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
