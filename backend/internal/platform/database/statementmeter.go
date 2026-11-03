// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package database

import (
	"context"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// StatementCost is what the pgx calls made under one metered context cost.
// DBTime runs from the start of each pgx call, before encoding or prepare,
// until its result is closed. So it holds the network, the server's planning
// and execution, and the rows read in Go between Query and Close. PoolWait is
// the whole of each Acquire call: checkout, any connect and health check.
type StatementCost struct {
	Statements int64
	Failed     int64
	Acquires   int64
	DBTime     time.Duration
	PoolWait   time.Duration
}

// Minus answers the cost run since earlier was read off the same meter.
func (c StatementCost) Minus(earlier StatementCost) StatementCost {
	return StatementCost{
		Statements: c.Statements - earlier.Statements,
		Failed:     c.Failed - earlier.Failed,
		Acquires:   c.Acquires - earlier.Acquires,
		DBTime:     c.DBTime - earlier.DBTime,
		PoolWait:   c.PoolWait - earlier.PoolWait,
	}
}

// Plus answers both costs together.
func (c StatementCost) Plus(other StatementCost) StatementCost {
	return StatementCost{
		Statements: c.Statements + other.Statements,
		Failed:     c.Failed + other.Failed,
		Acquires:   c.Acquires + other.Acquires,
		DBTime:     c.DBTime + other.DBTime,
		PoolWait:   c.PoolWait + other.PoolWait,
	}
}

// StatementMeter adds up the cost of every statement and pool acquire made
// with a context it is bound to. A lock guards it because a Detached write
// runs on a second connection beside the one the read holds.
type StatementMeter struct {
	mu   sync.Mutex
	cost StatementCost
}

type statementMeterKey struct{}

// WithStatementMeter binds a fresh meter to ctx. Every statement made with ctx,
// or with a context derived from it, through a pool from PoolConfig adds to it.
func WithStatementMeter(ctx context.Context) (context.Context, *StatementMeter) {
	meter := &StatementMeter{}
	return context.WithValue(ctx, statementMeterKey{}, meter), meter
}

// StatementMeterOf answers the meter bound to ctx, if any.
func StatementMeterOf(ctx context.Context) (*StatementMeter, bool) {
	meter, ok := ctx.Value(statementMeterKey{}).(*StatementMeter)
	return meter, ok
}

// Cost answers what the meter has added up so far.
func (m *StatementMeter) Cost() StatementCost {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.cost
}

func (m *StatementMeter) add(spent StatementCost) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.cost = m.cost.Plus(spent)
}

// statementTracer feeds the meter on a pgx call's context. A context with no
// meter costs one value lookup per hook and records nothing. A pool holds one
// tracer, so the census rides here; it is nil unless SQLCensusDirEnv is set.
type statementTracer struct {
	now    func() time.Time
	census *sqlCensus
}

var (
	_ pgx.QueryTracer       = statementTracer{}
	_ pgx.BatchTracer       = statementTracer{}
	_ pgx.PrepareTracer     = statementTracer{}
	_ pgx.CopyFromTracer    = statementTracer{}
	_ pgxpool.AcquireTracer = statementTracer{}
)

type statementStartKey struct{}

type callKind int

const (
	statementCall callKind = iota
	batchCall
	acquireCall
	// innerCall is a prepare pgx makes inside a Query, Exec or batch. It is
	// part of that call and already in its time, so it records nothing.
	innerCall
)

var innerPrepare = &statementStart{kind: innerCall}

// statementStart is what a start hook leaves on the context for its end hook.
// A batch counts its queries on it as pgx reads each result.
type statementStart struct {
	meter   *StatementMeter
	at      time.Time
	kind    callKind
	queries int64
	failed  int64
	ended   bool
}

func (t statementTracer) started(ctx context.Context, kind callKind) context.Context {
	meter, ok := StatementMeterOf(ctx)
	if !ok {
		return ctx
	}
	return context.WithValue(ctx, statementStartKey{}, &statementStart{meter: meter, at: t.now(), kind: kind})
}

func startOf(ctx context.Context) (*statementStart, bool) {
	start, ok := ctx.Value(statementStartKey{}).(*statementStart)
	return start, ok
}

// ended adds what the start hook on ctx began, with err as its failure.
func (t statementTracer) ended(ctx context.Context, err error) {
	start, ok := startOf(ctx)
	// An inner prepare records nothing. pgx ends a batch that failed before
	// send twice, once in SendBatch and once in Close.
	if !ok || start.kind == innerCall || start.ended {
		return
	}
	start.ended = true
	elapsed := t.now().Sub(start.at)
	var spent StatementCost
	switch start.kind {
	case acquireCall:
		spent = StatementCost{Acquires: 1, PoolWait: elapsed}
	case batchCall:
		spent = StatementCost{Statements: start.queries, Failed: start.failed, DBTime: elapsed}
		// A batch that failed before any query ran has no statement to blame.
		if err != nil && start.queries == 0 {
			spent.Failed = 1
		}
	case statementCall:
		spent = StatementCost{Statements: 1, DBTime: elapsed}
		if err != nil {
			spent.Failed = 1
		}
	}
	start.meter.add(spent)
}

func (t statementTracer) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	t.census.record(data.SQL)
	return t.started(ctx, statementCall)
}

// TraceQueryEnd runs when the rows close, so Query's time holds the scan loop.
func (t statementTracer) TraceQueryEnd(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryEndData) {
	t.ended(ctx, data.Err)
}

func (t statementTracer) TraceBatchStart(ctx context.Context, _ *pgx.Conn, _ pgx.TraceBatchStartData) context.Context {
	return t.started(ctx, batchCall)
}

// TraceBatchQuery runs once per query whose result pgx read; one conn reads
// them in turn, so the start needs no lock.
func (t statementTracer) TraceBatchQuery(ctx context.Context, _ *pgx.Conn, data pgx.TraceBatchQueryData) {
	t.census.record(data.SQL)
	start, ok := startOf(ctx)
	if !ok {
		return
	}
	start.queries++
	if data.Err != nil {
		start.failed++
	}
}

func (t statementTracer) TraceBatchEnd(ctx context.Context, _ *pgx.Conn, data pgx.TraceBatchEndData) {
	t.ended(ctx, data.Err)
}

// TracePrepareStart counts an explicit Prepare, and leaves out the one pgx
// makes inside another call.
func (t statementTracer) TracePrepareStart(ctx context.Context, _ *pgx.Conn, _ pgx.TracePrepareStartData) context.Context {
	if start, ok := startOf(ctx); ok && start.kind != acquireCall {
		return context.WithValue(ctx, statementStartKey{}, innerPrepare)
	}
	return t.started(ctx, statementCall)
}

func (t statementTracer) TracePrepareEnd(ctx context.Context, _ *pgx.Conn, data pgx.TracePrepareEndData) {
	t.ended(ctx, data.Err)
}

func (t statementTracer) TraceCopyFromStart(ctx context.Context, _ *pgx.Conn, _ pgx.TraceCopyFromStartData) context.Context {
	return t.started(ctx, statementCall)
}

func (t statementTracer) TraceCopyFromEnd(ctx context.Context, _ *pgx.Conn, data pgx.TraceCopyFromEndData) {
	t.ended(ctx, data.Err)
}

func (t statementTracer) TraceAcquireStart(ctx context.Context, _ *pgxpool.Pool, _ pgxpool.TraceAcquireStartData) context.Context {
	return t.started(ctx, acquireCall)
}

// TraceAcquireEnd records the wait; a failed acquire is not a failed statement.
func (t statementTracer) TraceAcquireEnd(ctx context.Context, _ *pgxpool.Pool, _ pgxpool.TraceAcquireEndData) {
	t.ended(ctx, nil)
}
