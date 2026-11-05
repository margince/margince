// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package database

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// steppedClock answers a time that moves by step on every read, so a start
// hook and its end hook are one step apart.
func steppedClock(step time.Duration) func() time.Time {
	at := time.Date(2026, 10, 29, 9, 0, 0, 0, time.UTC)
	return func() time.Time {
		at = at.Add(step)
		return at
	}
}

func TestAMeteredQueryCountsOneStatementAndItsTime(t *testing.T) {
	t.Parallel()
	tracer := statementTracer{now: steppedClock(5 * time.Millisecond)}
	ctx, meter := WithStatementMeter(context.Background())

	traced := tracer.TraceQueryStart(ctx, nil, pgx.TraceQueryStartData{SQL: "SELECT 1"})
	tracer.TraceQueryEnd(traced, nil, pgx.TraceQueryEndData{})

	if got := meter.Cost(); got != (StatementCost{Statements: 1, DBTime: 5 * time.Millisecond}) {
		t.Fatalf("cost = %+v, want one statement of 5ms", got)
	}
}

func TestAFailedOrCancelledStatementCountsAsAFailure(t *testing.T) {
	t.Parallel()
	for name, err := range map[string]error{
		"failed":    errors.New("relation does not exist"),
		"cancelled": context.Canceled,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			tracer := statementTracer{now: steppedClock(time.Millisecond)}
			ctx, meter := WithStatementMeter(context.Background())

			traced := tracer.TraceQueryStart(ctx, nil, pgx.TraceQueryStartData{})
			tracer.TraceQueryEnd(traced, nil, pgx.TraceQueryEndData{Err: err})

			if got := meter.Cost(); got.Statements != 1 || got.Failed != 1 {
				t.Fatalf("cost = %+v, want one statement that failed", got)
			}
		})
	}
}

func TestABatchCountsEachQueryPgxReadAndEachThatFailed(t *testing.T) {
	t.Parallel()
	tracer := statementTracer{now: steppedClock(2 * time.Millisecond)}
	ctx, meter := WithStatementMeter(context.Background())
	failed := errors.New("second failed")

	traced := tracer.TraceBatchStart(ctx, nil, pgx.TraceBatchStartData{})
	tracer.TraceBatchQuery(traced, nil, pgx.TraceBatchQueryData{})
	tracer.TraceBatchQuery(traced, nil, pgx.TraceBatchQueryData{Err: failed})
	tracer.TraceBatchEnd(traced, nil, pgx.TraceBatchEndData{Err: failed})

	if got := meter.Cost(); got != (StatementCost{Statements: 2, Failed: 1, DBTime: 2 * time.Millisecond}) {
		t.Fatalf("cost = %+v, want the two queries read, one failure, one round trip of 2ms", got)
	}
}

func TestABatchThatFailsBeforeAnyQueryRunsCountsNoStatement(t *testing.T) {
	t.Parallel()
	tracer := statementTracer{now: steppedClock(time.Millisecond)}
	ctx, meter := WithStatementMeter(context.Background())

	traced := tracer.TraceBatchStart(ctx, nil, pgx.TraceBatchStartData{})
	tracer.TraceBatchEnd(traced, nil, pgx.TraceBatchEndData{Err: errors.New("cannot encode")})

	if got := meter.Cost(); got.Statements != 0 || got.Failed != 1 {
		t.Fatalf("cost = %+v, want no statement and the batch's one failure", got)
	}
}

func TestAnExplicitPrepareCountsAndOneInsideAQueryDoesNot(t *testing.T) {
	t.Parallel()
	tracer := statementTracer{now: steppedClock(time.Millisecond)}
	ctx, meter := WithStatementMeter(context.Background())

	prepared := tracer.TracePrepareStart(ctx, nil, pgx.TracePrepareStartData{})
	tracer.TracePrepareEnd(prepared, nil, pgx.TracePrepareEndData{})
	query := tracer.TraceQueryStart(ctx, nil, pgx.TraceQueryStartData{})
	inner := tracer.TracePrepareStart(query, nil, pgx.TracePrepareStartData{})
	tracer.TracePrepareEnd(inner, nil, pgx.TracePrepareEndData{})
	tracer.TraceQueryEnd(query, nil, pgx.TraceQueryEndData{})

	if got := meter.Cost(); got.Statements != 2 {
		t.Fatalf("cost = %+v, want the explicit prepare and the query", got)
	}
}

func TestACopyFromCountsOneStatement(t *testing.T) {
	t.Parallel()
	tracer := statementTracer{now: steppedClock(4 * time.Millisecond)}
	ctx, meter := WithStatementMeter(context.Background())

	traced := tracer.TraceCopyFromStart(ctx, nil, pgx.TraceCopyFromStartData{})
	tracer.TraceCopyFromEnd(traced, nil, pgx.TraceCopyFromEndData{})

	if got := meter.Cost(); got != (StatementCost{Statements: 1, DBTime: 4 * time.Millisecond}) {
		t.Fatalf("cost = %+v, want one statement of 4ms", got)
	}
}

func TestAnAcquireRecordsPoolWaitAndNoStatement(t *testing.T) {
	t.Parallel()
	tracer := statementTracer{now: steppedClock(3 * time.Millisecond)}
	ctx, meter := WithStatementMeter(context.Background())

	traced := tracer.TraceAcquireStart(ctx, nil, pgxpool.TraceAcquireStartData{})
	tracer.TraceAcquireEnd(traced, nil, pgxpool.TraceAcquireEndData{Err: errors.New("pool closed")})

	if got := meter.Cost(); got != (StatementCost{Acquires: 1, PoolWait: 3 * time.Millisecond}) {
		t.Fatalf("cost = %+v, want one acquire of 3ms and nothing else", got)
	}
}

func TestAStatementWithNoMeterRecordsNothingAndKeepsItsContext(t *testing.T) {
	t.Parallel()
	tracer := statementTracer{now: func() time.Time {
		t.Fatal("the tracer read the clock for a statement nobody meters")
		return time.Time{}
	}}
	ctx := context.Background()

	traced := tracer.TraceQueryStart(ctx, nil, pgx.TraceQueryStartData{})
	tracer.TraceQueryEnd(traced, nil, pgx.TraceQueryEndData{Err: errors.New("ignored")})
	tracedAcquire := tracer.TraceAcquireStart(ctx, nil, pgxpool.TraceAcquireStartData{})
	tracer.TraceAcquireEnd(tracedAcquire, nil, pgxpool.TraceAcquireEndData{})

	if traced != ctx || tracedAcquire != ctx {
		t.Fatal("a start hook with no meter wrapped the context")
	}
}

func TestEveryPoolOfThisProductCarriesTheStatementTracer(t *testing.T) {
	t.Parallel()
	cfg, err := PoolConfig("postgres://u:p@localhost:5432/db")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := cfg.ConnConfig.Tracer.(statementTracer); !ok {
		t.Fatalf("pool tracer = %T, want the statement tracer", cfg.ConnConfig.Tracer)
	}
	if _, ok := cfg.ConnConfig.Tracer.(pgxpool.AcquireTracer); !ok {
		t.Fatal("the tracer does not trace acquires, so no pool wait reaches a meter")
	}
	if _, ok := cfg.ConnConfig.Tracer.(pgx.CopyFromTracer); !ok {
		t.Fatal("the tracer does not trace CopyFrom, so a copy goes uncounted")
	}
}

func BenchmarkStatementTracer(b *testing.B) {
	tracer := statementTracer{now: time.Now}
	run := func(b *testing.B, ctx context.Context) {
		b.Helper()
		b.ReportAllocs()
		for b.Loop() {
			traced := tracer.TraceQueryStart(ctx, nil, pgx.TraceQueryStartData{})
			tracer.TraceQueryEnd(traced, nil, pgx.TraceQueryEndData{})
		}
	}
	b.Run("no meter", func(b *testing.B) { run(b, context.Background()) })
	b.Run("metered", func(b *testing.B) {
		ctx, _ := WithStatementMeter(context.Background())
		run(b, ctx)
	})
}
