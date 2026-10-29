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

// StatementCost is what the statements run under one metered context cost.
// DBTime is database time as the client saw it, from send until the result
// closes. It holds the network and the rows read in Go, not only the server.
type StatementCost struct {
	Statements int64
	Failed     int64
	DBTime     time.Duration
	PoolWait   time.Duration
}

// Minus answers the cost run since earlier was read off the same meter.
func (c StatementCost) Minus(earlier StatementCost) StatementCost {
	return StatementCost{
		Statements: c.Statements - earlier.Statements,
		Failed:     c.Failed - earlier.Failed,
		DBTime:     c.DBTime - earlier.DBTime,
		PoolWait:   c.PoolWait - earlier.PoolWait,
	}
}

// Plus answers both costs together.
func (c StatementCost) Plus(other StatementCost) StatementCost {
	return StatementCost{
		Statements: c.Statements + other.Statements,
		Failed:     c.Failed + other.Failed,
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

// statementTracer feeds the meter on a statement's context. A context with no
// meter costs one value lookup per hook and records nothing.
type statementTracer struct{ now func() time.Time }

var (
	_ pgx.QueryTracer       = statementTracer{}
	_ pgx.BatchTracer       = statementTracer{}
	_ pgxpool.AcquireTracer = statementTracer{}
)

type statementStartKey struct{}

// statementStart is what a start hook leaves on the context for its end hook.
// A start with no statements is a pool acquire, whose time is a wait.
type statementStart struct {
	meter      *StatementMeter
	at         time.Time
	statements int64
}

func (t statementTracer) started(ctx context.Context, statements int64) context.Context {
	meter, ok := StatementMeterOf(ctx)
	if !ok {
		return ctx
	}
	return context.WithValue(ctx, statementStartKey{}, statementStart{meter: meter, at: t.now(), statements: statements})
}

// ended adds what the start hook on ctx began, and err as one failure.
func (t statementTracer) ended(ctx context.Context, err error) {
	start, ok := ctx.Value(statementStartKey{}).(statementStart)
	if !ok {
		return
	}
	elapsed := t.now().Sub(start.at)
	spent := StatementCost{Statements: start.statements, DBTime: elapsed}
	if start.statements == 0 {
		spent = StatementCost{PoolWait: elapsed}
	}
	if err != nil {
		spent.Failed = 1
	}
	start.meter.add(spent)
}

func (t statementTracer) TraceQueryStart(ctx context.Context, _ *pgx.Conn, _ pgx.TraceQueryStartData) context.Context {
	return t.started(ctx, 1)
}

// TraceQueryEnd runs when the rows close, so Query's time holds the scan loop.
func (t statementTracer) TraceQueryEnd(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryEndData) {
	t.ended(ctx, data.Err)
}

// TraceBatchStart counts every queued statement, since one round trip sends them all.
func (t statementTracer) TraceBatchStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceBatchStartData) context.Context {
	return t.started(ctx, int64(data.Batch.Len()))
}

// TraceBatchQuery has nothing to add: a failed statement also fails the batch end.
func (statementTracer) TraceBatchQuery(context.Context, *pgx.Conn, pgx.TraceBatchQueryData) {}

func (t statementTracer) TraceBatchEnd(ctx context.Context, _ *pgx.Conn, data pgx.TraceBatchEndData) {
	t.ended(ctx, data.Err)
}

func (t statementTracer) TraceAcquireStart(ctx context.Context, _ *pgxpool.Pool, _ pgxpool.TraceAcquireStartData) context.Context {
	return t.started(ctx, 0)
}

// TraceAcquireEnd records the wait; a failed acquire is not a failed statement.
func (t statementTracer) TraceAcquireEnd(ctx context.Context, _ *pgxpool.Pool, _ pgxpool.TraceAcquireEndData) {
	t.ended(ctx, nil)
}
