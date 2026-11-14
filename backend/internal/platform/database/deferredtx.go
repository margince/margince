// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package database

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// deferredTx is a read snapshot that holds session statements: a lane's
// savepoint and budget, or a plan cache mode. It sends them on the next
// exchange it sends anyway. A scope that sends nothing sends none of them.
type deferredTx struct {
	pgx.Tx
	pending []deferredStatement
	// scopes numbers the savepoints, so an abandoned lane rolls back to its own
	// even while an inner lane's release is still pending.
	scopes int
}

// deferredStatement is one queued statement. scan reads its row; without one
// its result is only checked. opens marks the statement whose success opens
// its scope. savepoint marks one that must exist before the next statement is
// prepared. A batch prepares every statement before it runs any, so a failed
// prepare would take the savepoint with it.
type deferredStatement struct {
	sql       string
	args      []any
	scan      func(pgx.Row) error
	opens     bool
	savepoint bool
	scope     *deferredScope
}

// deferredScope is one bracket of session statements. sent turns true once
// the server ran its opening statement.
type deferredScope struct {
	id   int
	sent bool
}

func newDeferredTx(tx pgx.Tx) *deferredTx { return &deferredTx{Tx: tx} }

// deferredOf answers tx as a deferredTx when it is one.
func deferredOf(tx pgx.Tx) (*deferredTx, bool) {
	d, ok := tx.(*deferredTx)
	return d, ok
}

// underlying is the driver's transaction behind tx, the key its memo has.
//
//nolint:ireturn // pgx.Tx is the driver's transaction interface.
func underlying(tx pgx.Tx) pgx.Tx {
	if d, ok := deferredOf(tx); ok {
		return d.Tx
	}
	return tx
}

// open starts a scope whose opening statements ride on the next exchange.
func (d *deferredTx) open(statements ...deferredStatement) *deferredScope {
	d.scopes++
	scope := &deferredScope{id: d.scopes}
	d.queue(scope, statements...)
	return scope
}

func (d *deferredTx) queue(scope *deferredScope, statements ...deferredStatement) {
	for _, statement := range statements {
		statement.scope = scope
		d.pending = append(d.pending, statement)
	}
}

// close queues a scope's closing statements, or drops its opening ones when
// nothing was sent while it was open.
func (d *deferredTx) close(scope *deferredScope, closing ...deferredStatement) {
	if !scope.sent {
		d.drop(scope)
		return
	}
	d.queue(scope, closing...)
}

// drop removes every pending statement of scope and of the scopes opened
// inside it, which a rollback to scope undoes anyway.
func (d *deferredTx) drop(scope *deferredScope) {
	kept := d.pending[:0]
	for _, statement := range d.pending {
		if statement.scope.id < scope.id {
			kept = append(kept, statement)
		}
	}
	d.pending = kept
}

// take empties the queue into batch.
func (d *deferredTx) take(batch *pgx.Batch) []deferredStatement {
	taken := d.pending
	d.pending = nil
	for _, statement := range taken {
		batch.Queue(statement.sql, statement.args...)
	}
	return taken
}

// readTaken reads the results of the statements take queued first in br, and
// marks a scope sent once its opening statement ran.
func readTaken(br pgx.BatchResults, taken []deferredStatement) error {
	for _, statement := range taken {
		var err error
		if statement.scan != nil {
			err = statement.scan(br.QueryRow())
		} else {
			_, err = br.Exec()
		}
		if err != nil {
			return fmt.Errorf("pg: a deferred session statement: %w", err)
		}
		if statement.opens {
			statement.scope.sent = true
		}
	}
	return nil
}

// holdsSavepoint is whether a pending statement must run before the next
// statement is prepared.
func (d *deferredTx) holdsSavepoint() bool {
	for _, statement := range d.pending {
		if statement.savepoint {
			return true
		}
	}
	return false
}

// rides is whether the next statement may carry what is pending.
func (d *deferredTx) rides(args []any) bool {
	return len(d.pending) > 0 && carries(args) && !d.holdsSavepoint()
}

// flush sends what is pending on its own, before an exchange that cannot
// carry it.
func (d *deferredTx) flush(ctx context.Context) error {
	if len(d.pending) == 0 {
		return nil
	}
	batch := &pgx.Batch{}
	taken := d.take(batch)
	br := d.Tx.SendBatch(ctx, batch)
	return errors.Join(readTaken(br, taken), br.Close())
}

// carries is whether a statement with these arguments may share a batch. A
// batch runs in the connection's default mode, so one naming its own mode or
// rewriting its query is sent alone.
func carries(args []any) bool {
	if len(args) == 0 {
		return true
	}
	switch args[0].(type) {
	case pgx.QueryExecMode, pgx.QueryRewriter:
		return false
	}
	return true
}

func (d *deferredTx) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	if !d.rides(args) {
		if err := d.flush(ctx); err != nil {
			return pgconn.CommandTag{}, err
		}
		return d.Tx.Exec(ctx, sql, args...)
	}
	batch := &pgx.Batch{}
	taken := d.take(batch)
	batch.Queue(sql, args...)
	br := d.Tx.SendBatch(ctx, batch)
	if err := readTaken(br, taken); err != nil {
		return pgconn.CommandTag{}, errors.Join(err, br.Close())
	}
	tag, err := br.Exec()
	return tag, errors.Join(err, br.Close())
}

//nolint:ireturn // pgx.Rows is the driver's interface.
func (d *deferredTx) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	if !d.rides(args) {
		if err := d.flush(ctx); err != nil {
			return nil, err
		}
		return d.Tx.Query(ctx, sql, args...)
	}
	batch := &pgx.Batch{}
	taken := d.take(batch)
	batch.Queue(sql, args...)
	br := d.Tx.SendBatch(ctx, batch)
	if err := readTaken(br, taken); err != nil {
		return nil, errors.Join(err, br.Close())
	}
	rows, err := br.Query()
	if err != nil {
		return nil, errors.Join(err, br.Close())
	}
	return &batchRows{Rows: rows, br: br}, nil
}

//nolint:ireturn // pgx.Row is the driver's interface.
func (d *deferredTx) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	rows, err := d.Query(ctx, sql, args...)
	return &deferredRow{rows: rows, err: err}
}

//nolint:ireturn // pgx.BatchResults is the driver's interface.
func (d *deferredTx) SendBatch(ctx context.Context, b *pgx.Batch) pgx.BatchResults {
	if !d.rides(nil) {
		if err := d.flush(ctx); err != nil {
			return failedBatch{err: err}
		}
		return d.Tx.SendBatch(ctx, b)
	}
	batch := &pgx.Batch{}
	taken := d.take(batch)
	batch.QueuedQueries = append(batch.QueuedQueries, b.QueuedQueries...)
	br := d.Tx.SendBatch(ctx, batch)
	if err := readTaken(br, taken); err != nil {
		return failedBatch{err: errors.Join(err, br.Close())}
	}
	return br
}

//nolint:ireturn // pgx.Tx is the driver's transaction interface.
func (d *deferredTx) Begin(ctx context.Context) (pgx.Tx, error) {
	if err := d.flush(ctx); err != nil {
		return nil, err
	}
	return d.Tx.Begin(ctx)
}

func (d *deferredTx) CopyFrom(ctx context.Context, table pgx.Identifier, columns []string, rows pgx.CopyFromSource) (int64, error) {
	if err := d.flush(ctx); err != nil {
		return 0, err
	}
	return d.Tx.CopyFrom(ctx, table, columns, rows)
}

func (d *deferredTx) Prepare(ctx context.Context, name, sql string) (*pgconn.StatementDescription, error) {
	if err := d.flush(ctx); err != nil {
		return nil, err
	}
	return d.Tx.Prepare(ctx, name, sql)
}

// batchRows closes the batch its rows came from once they are read.
type batchRows struct {
	pgx.Rows
	br     pgx.BatchResults
	closed bool
	err    error
}

func (r *batchRows) Close() {
	if r.closed {
		return
	}
	r.closed = true
	r.Rows.Close()
	r.err = r.br.Close()
}

func (r *batchRows) Next() bool {
	if r.Rows.Next() {
		return true
	}
	r.Close()
	return false
}

func (r *batchRows) Err() error { return errors.Join(r.Rows.Err(), r.err) }

// deferredRow is pgx's QueryRow over a deferredTx query.
type deferredRow struct {
	rows pgx.Rows
	err  error
}

func (r *deferredRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	defer r.rows.Close()
	if !r.rows.Next() {
		if err := r.rows.Err(); err != nil {
			return err
		}
		return pgx.ErrNoRows
	}
	if err := r.rows.Scan(dest...); err != nil {
		return err
	}
	r.rows.Close()
	return r.rows.Err()
}

// failedBatch answers a batch whose deferred statements failed.
type failedBatch struct{ err error }

func (f failedBatch) Exec() (pgconn.CommandTag, error) { return pgconn.CommandTag{}, f.err }

//nolint:ireturn // pgx.Rows is the driver's interface.
func (f failedBatch) Query() (pgx.Rows, error) { return nil, f.err }

//nolint:ireturn // pgx.Row is the driver's interface.
func (f failedBatch) QueryRow() pgx.Row { return &deferredRow{err: f.err} }
func (f failedBatch) Close() error      { return f.err }
