// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package jobs

// The scoped read behind GET /admin/job-health. It lives beside Stats
// rather than in the composition layer because river_job has no RLS and no
// workspace column: every statement over it is a hand-imposed scope, and
// two readers spelling that scope in two packages is how the operational
// and the admin surface drift into different answers about the same table.

import (
	"context"
	"fmt"
	"log/slog"
	"slices"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// waitingStates are queued-and-not-yet-started. pending is River's staged
// state; it is counted here for the same reason the metric counts it —
// work nobody has done is waiting, whatever River calls the state.
const waitingStates = `('available','scheduled','pending')`

// KindHealth is one kind's live state under the caller's scope.
type KindHealth struct {
	Kind          string
	Queue         string
	FleetWide     bool
	Waiting       int64
	Running       int64
	Retrying      int64
	Discarded     int64
	Cancelled     int64
	Runnable      int64
	FutureRetries int64
	NextAttemptAt *time.Time
	// Dead is discarded plus cancelled: work that will not happen without
	// intervention. The two are reported together here because the question
	// the field answers is "will this run" and the answer is no either way;
	// the exposition endpoint keeps them apart, where the question is why.
	//
	// UNBOUNDED IN AGE, and that is what makes it a report figure rather than
	// an alarm: River retains a terminal row for seven days, so this counts a
	// week of history and an outage that ended an hour ago reads the same as one
	// still running.
	Dead int64
	// DeadRecent is the same count inside the caller's window — the number that
	// earns the banner. The pair is the whole point: an operator can still see
	// the week's total without being paged by it, and the red one answers "is
	// something wrong NOW", which is the distinction the unbounded count cannot
	// draw.
	DeadRecent int64
	// OldestWaitingAgeSeconds is nil when nothing of this kind is runnable
	// right now. Nil and zero are different claims — nothing waiting versus
	// something that just became runnable — so the absence is carried
	// rather than flattened.
	OldestWaitingAgeSeconds *float64
}

// Failure is one recent failed or failing job.
type Failure struct {
	// ID is river_job.id, and it is the ONE key that makes the rest of this
	// row actionable. River's own log lines carry job_id=<id>, and every
	// psql question an operator then asks — what were the args, what does
	// the full errors array say, is it still retrying — is keyed by it. A
	// failure list that pointed at the process log while naming no id was
	// pointing at a log nobody could search.
	ID          int64
	Kind        string
	State       string
	Attempt     int
	MaxAttempts int
	FailedAt    time.Time
	// FirstFailedAt is when the FIRST recorded attempt STARTED, which is what
	// River stores on an attempt error (AttemptError.At is the attempt's start,
	// not the moment it failed — the same trap FailedAt above reads columns to
	// avoid). It is named for what an operator asks rather than for the column:
	// the question is "how long has this been going wrong", and the first
	// attempt's start is the honest answer to it. A job that failed on a wall-clock
	// timeout began failing at that start, whatever second the timeout fired.
	//
	// It answers what the attempt counter cannot: "failing since 21:08" and
	// "failed once at 21:08" are different operator situations, and an attempt
	// count of 3 says which ladder rung the job is on, never how long it has been
	// on the ladder.
	//
	// Nil when no attempt error was recorded at all — a cancelled job that never
	// ran records none, and a zero time would read on a screen as 1970 rather than
	// as absence. The absence is carried for the same reason
	// OldestWaitingAgeSeconds carries its own: nil and a value are different
	// claims, and flattening one into the other invents a fact.
	FirstFailedAt *time.Time
	// StoredReason is river_job.errors' text VERBATIM, and the caller must
	// not put it on a wire without vetting it: the column holds whatever a
	// worker returned, and a worker that bypassed Fault stored a raw cause
	// that routinely names the address or record a provider refused. Ask
	// VettedFailure.
	StoredReason string
}

// Health is one read of the installation's job table for its admin.
type Health struct {
	Kinds    []KindHealth
	Failures []Failure
}

// recentFailureLimit bounds the failure list. It is a bounded view, not a
// log: an admin needs to see what is failing now, and an unbounded list
// over a table River retains for days is a different product.
const recentFailureLimit = 50

// ReadHealth reads the installation's job table for its admin: every row,
// because an installation holds one company and so every job is its work.
//
// dispatcherKinds is supplied by the caller rather than read off the
// declaration in this package: it marks which kinds the page reports as
// fleet-wide, and a list the calling surface states for itself is one that
// surface can be gated on.
//
// `recent` bounds DeadRecent, and arrives from the caller for the same reason:
// it is an installation setting, and this package holds no settings reader.
func ReadHealth(ctx context.Context, pool *pgxpool.Pool, dispatcherKinds []string, recent time.Duration) (Health, error) {
	kinds, err := healthByKind(ctx, pool, dispatcherKinds, recent)
	if err != nil {
		return Health{}, err
	}
	failures, err := recentFailures(ctx, pool)
	if err != nil {
		return Health{}, err
	}
	return Health{Kinds: kinds, Failures: failures}, nil
}

// ReadKindHealth omits the failure sample and the windowed DeadRecent count.
func ReadKindHealth(ctx context.Context, pool *pgxpool.Pool, dispatcherKinds []string) ([]KindHealth, error) {
	return healthByKind(ctx, pool, dispatcherKinds, 0)
}

// healthByKind groups the installation's live rows by kind and queue.
func healthByKind(ctx context.Context, pool *pgxpool.Pool, dispatcherKinds []string, recent time.Duration) ([]KindHealth, error) {
	const q = `
		SELECT kind,
		       queue,
		       coalesce(kind = ANY($1::text[]), false) AS fleet_wide,
		       count(*) FILTER (WHERE state::text IN ` + waitingStates + `)::bigint,
		       count(*) FILTER (WHERE state::text = 'running')::bigint,
		       count(*) FILTER (WHERE state::text = 'retryable')::bigint,
		       count(*) FILTER (WHERE state::text IN ` + terminalBadStates + `)::bigint,
		       -- The banner's own count. finalized_at is when a terminal row
		       -- BECAME terminal, which is the moment an operator is being asked
		       -- about — not created_at, which dates a job that may have sat in
		       -- the queue for hours before it died.
		       --
		       -- Compared directly, with no coalesce: river_job's own
		       -- finalized_or_finalized_at_null CHECK makes the column NOT NULL
		       -- for exactly these states, so a fallback here would be a branch
		       -- Postgres does not admit a row to reach.
		       -- Held by: TestATerminalRowMustCarryTheMomentItDied
		       count(*) FILTER (WHERE state::text IN ` + terminalBadStates + `
		                          AND finalized_at >= now() - $2::interval)::bigint,
		       count(*) FILTER (WHERE state::text IN ` + runnableStates + ` AND scheduled_at <= now())::bigint,
		       count(*) FILTER (WHERE state::text = 'retryable' AND scheduled_at > now())::bigint,
		       count(*) FILTER (WHERE state::text = 'discarded')::bigint,
		       count(*) FILTER (WHERE state::text = 'cancelled')::bigint,
		       min(scheduled_at) FILTER (WHERE state::text = 'retryable' AND scheduled_at > now()),
		       max(EXTRACT(EPOCH FROM (now() - scheduled_at)))
		           FILTER (WHERE state::text IN ` + runnableStates + `
		                     AND scheduled_at <= now())::double precision
		FROM river_job
		WHERE state <> 'completed'
		GROUP BY 1, 2, 3
		ORDER BY 1, 2, 3`

	rows, err := pool.Query(ctx, q, dispatcherKinds, recent)
	if err != nil {
		return nil, fmt.Errorf("jobs: reading job health by kind: %w", err)
	}
	defer rows.Close()

	var out []KindHealth
	for rows.Next() {
		var k KindHealth
		if err := rows.Scan(&k.Kind, &k.Queue, &k.FleetWide,
			&k.Waiting, &k.Running, &k.Retrying, &k.Dead, &k.DeadRecent,
			&k.Runnable, &k.FutureRetries, &k.Discarded, &k.Cancelled, &k.NextAttemptAt, &k.OldestWaitingAgeSeconds); err != nil {
			return nil, fmt.Errorf("jobs: scanning job health by kind: %w", err)
		}
		if k.NextAttemptAt != nil {
			at := k.NextAttemptAt.UTC()
			k.NextAttemptAt = &at
		}
		out = append(out, k)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("jobs: reading job health by kind: %w", err)
	}
	return out, nil
}

// recentFailures reads the most recent failed-or-failing rows under the
// caller's scope.
//
// River stores AttemptError{At, Attempt, Error, Trace} per element of a
// jsonb[]. Only the MESSAGE is selected, never the element: Trace carries a
// full panic stack, and serializing the object whole would carry it past
// the vetting the caller applies to the message alone. The index is
// 1-based and the newest attempt is last, so cardinality() is the newest —
// and it is null-safe in both directions, because errors is a nullable
// column and a cancelled row MAY carry no attempt error at all. River's
// cancel path is two paths, not one: a worker returning JobCancel(err)
// persists an AttemptError like any other failure, while a cancellation
// with no cause appends nothing, and the row is terminal either way.
//
// failed_at is read from COLUMNS, not from the attempt error's own
// timestamp: River sets AttemptError.At to the attempt's START (job_executor
// sets `At: e.start`), so a long-running job that failed after an hour would
// report the hour-ago moment it began and sort ahead of failures that
// actually happened later. finalized_at is the real failure moment for a
// terminal row, attempted_at is the closest available for a retryable one,
// and created_at covers a row that has never run — a total order over
// columns that always exist, with id breaking ties.
//
// Reading a column rather than the stored JSON also avoids casting
// app-written text in SQL:
// that column is app-written, and one malformed value would turn the whole
// endpoint into a 500 rather than one row into an approximation.
//
// first_failed_at is the one value that has no column to read, so errors[1]
// — the OLDEST attempt, the array being append-ordered — is read as TEXT and
// parsed in Go rather than cast in SQL. What that element carries is the
// attempt's START (see Failure.FirstFailedAt), which is why the field is not
// named for the moment of failure. A `::timestamptz` in the statement
// would put the same app-written column back in the query's critical path
// that the paragraph above keeps out of it: one unparseable value would fail
// the whole read instead of leaving one row's "failing since" unanswered.
// errors is a jsonb[], not a jsonb, so it is subscripted 1-based; the
// jsonb path operators (errors->-1 and friends) do not apply to it at all
// and would not compile against this column.
func recentFailures(ctx context.Context, pool *pgxpool.Pool) ([]Failure, error) {
	q := `
		SELECT id,
		       kind,
		       state::text,
		       attempt::int,
		       max_attempts::int,
		       coalesce(finalized_at, attempted_at, created_at),
		       errors[1]->>'at',
		       errors[cardinality(errors)]->>'error'
		FROM river_job
		WHERE state::text IN ('retryable','discarded','cancelled')
		ORDER BY coalesce(finalized_at, attempted_at, created_at) DESC, id DESC
		LIMIT ` + fmt.Sprint(recentFailureLimit)

	rows, err := pool.Query(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("jobs: reading recent job failures: %w", err)
	}
	defer rows.Close()

	var out []Failure
	for rows.Next() {
		var (
			f          Failure
			fallbackAt time.Time
			firstAt    *string
			stored     *string
		)
		if err := rows.Scan(&f.ID, &f.Kind, &f.State, &f.Attempt,
			&f.MaxAttempts, &fallbackAt, &firstAt, &stored); err != nil {
			return nil, fmt.Errorf("jobs: scanning recent job failures: %w", err)
		}
		// UTC because pgx returns a timestamptz in the session's zone, and
		// two offsets inside one array is needlessly hostile to whoever
		// reads the list.
		f.FailedAt = fallbackAt.UTC()
		f.FirstFailedAt = parseFirstFailedAt(ctx, f.Kind, firstAt)
		if stored != nil {
			f.StoredReason = *stored
		}
		out = append(out, f)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("jobs: reading recent job failures: %w", err)
	}

	// Sorted on the same expression the SELECT ordered by, so the visible
	// failed_at descends exactly as the field name promises. The database
	// already returns them in this order; re-sorting keeps that true if the
	// two ever drift apart.
	slices.SortStableFunc(out, func(a, b Failure) int { return b.FailedAt.Compare(a.FailedAt) })
	return out, nil
}

// parseFirstFailedAt reads the oldest attempt error's own timestamp, and
// answers nil for every row that has none to read.
//
// Two absences arrive here and both are honest: a row with an empty errors
// array (River's causeless cancel path appends nothing) and a row whose
// element carries no `at` key. Neither is a failure of this read, and
// neither may become a zero time — the field is a pointer precisely so
// absence stays absence instead of rendering as 1970 in a failure list.
//
// A value that is PRESENT and unparseable is a third thing, and it is
// logged rather than dropped in silence. River writes RFC 3339 here, so
// text this cannot parse means something other than River wrote the row —
// worth an operator knowing about, and not worth failing the whole read
// for, because the rest of the row still tells them what died.
func parseFirstFailedAt(ctx context.Context, kind string, raw *string) *time.Time {
	if raw == nil {
		return nil
	}
	at, err := time.Parse(time.RFC3339Nano, *raw)
	if err != nil {
		slog.WarnContext(ctx, "jobs: a job failure carries an unparseable first-attempt timestamp",
			"kind", kind, "err", err)
		return nil
	}
	utc := at.UTC()
	return &utc
}
