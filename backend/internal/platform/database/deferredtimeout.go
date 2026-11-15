// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package database

import "github.com/jackc/pgx/v5"

// A deferred snapshot tracks the statement_timeout its session will hold once
// the pending statements run. A lane then reads the page's timeout only when
// nothing has told the snapshot what it is, and sets its budget only when a
// different one is in force. A lane that closes on a budget the next lane
// opens with leaves that budget in place: the restore and the set between them
// cancel out, so neither is sent.

// laneTimeout is what a lane changed about the statement timeout, so closing it
// or rolling it back puts the right value back.
type laneTimeout struct {
	budget string
	// prior is the timeout in force when the lane opened; read reports whether
	// a deferred read fills it in.
	prior string
	known bool
	// inherited is the closing restore of the lane before, held back because
	// this lane opened on the same budget. It goes back in the queue when this
	// lane leaves nothing behind it.
	inherited *deferredStatement
}

// openTimeout queues what the lane needs for budget, around the savepoint
// statement it is given, and answers the scope.
func (d *deferredTx) openTimeout(budget string, savepoint deferredStatement) (*deferredScope, *laneTimeout) {
	lane := &laneTimeout{budget: budget}
	var statements []deferredStatement
	if held := d.heldRestore(budget); held != nil {
		lane.prior, lane.known, lane.inherited = budget, true, held
	} else if d.timeout != nil {
		lane.prior, lane.known = *d.timeout, true
	} else {
		statements = append(statements, deferredStatement{
			sql:  `SELECT current_setting('statement_timeout')`,
			scan: func(row pgx.Row) error { lane.known = true; return row.Scan(&lane.prior) },
		})
	}
	statements = append(statements, savepoint)
	if !lane.known || lane.prior != budget {
		statements = append(statements, deferredStatement{sql: boundStatementSQL, args: []any{budget}})
	}
	d.timeout = &lane.budget
	return d.open(statements...), lane
}

// closeTimeout queues the restore of the timeout the lane found, before the
// release. A lane that took over the restore of the lane before sends that
// one; a lane that found its own budget in force restores nothing.
func (d *deferredTx) closeTimeout(scope *deferredScope, lane *laneTimeout, release deferredStatement) {
	if !scope.sent {
		d.drop(scope)
		d.returnTimeout(lane)
		return
	}
	var closing []deferredStatement
	d.timeout = &lane.prior
	switch {
	case lane.inherited != nil:
		closing = append(closing, *lane.inherited)
		d.timeout = restoreTarget(*lane.inherited)
	case lane.prior != lane.budget:
		closing = append(closing, deferredStatement{sql: boundStatementSQL, args: []any{lane.prior}, restores: lane.budget})
	}
	d.queue(scope, append(closing, release)...)
}

// restoreTarget is the timeout a queued restore puts back.
func restoreTarget(restore deferredStatement) *string {
	target, _ := restore.args[0].(string)
	return &target
}

// returnTimeout is the timeout after a lane was dropped or rolled back: the one
// it found, and the restore it held back goes into the queue again.
func (d *deferredTx) returnTimeout(lane *laneTimeout) {
	d.timeout = nil
	if lane.known {
		d.timeout = &lane.prior
	}
	if held := lane.inherited; held != nil {
		d.pending = append(d.pending, *held)
		d.timeout = restoreTarget(*held)
	}
}

// heldRestore takes the last pending timeout statement out of the queue when
// it restores from budget, and answers it.
func (d *deferredTx) heldRestore(budget string) *deferredStatement {
	for i := len(d.pending) - 1; i >= 0; i-- {
		statement := d.pending[i]
		if statement.sql != boundStatementSQL {
			continue
		}
		if statement.restores != budget {
			return nil
		}
		d.pending = append(d.pending[:i:i], d.pending[i+1:]...)
		return &statement
	}
	return nil
}

// forgetTimeout drops what the snapshot knows when a statement it does not
// track sets the timeout.
func (d *deferredTx) forgetTimeout(sql string) {
	if sql == boundStatementSQL {
		d.timeout = nil
	}
}
