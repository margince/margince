// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package unitsql runs an extension's own SQL as the ext_runtime role, which
// holds schema ext and nothing on the core's tables. The database refuses a
// core table, whatever the statement text looks like.
package unitsql

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/margince/margince/backend/pkg/extension"
)

// Tx is the three SQL verbs of extension.Tx over one transaction. Each
// statement switches to Role and back, so the transaction's other users keep
// the application role.
type Tx struct {
	tx    pgx.Tx
	proof *roleProof
}

// roleProof is open from a switch to Role until RESET ROLE succeeds. A check
// that could not run, a cancelled one included, leaves it open.
type roleProof struct{ open bool }

// errRoleUnproven refuses the commit of a transaction whose last switch to
// Role was never seen undone.
var errRoleUnproven = errors.New("unitsql: a statement's return from the unit role was not checked, so the transaction is rolled back")

// Run hands fn the confined verbs over tx. It refuses to let tx commit while
// the return from Role is unproven, whatever fn did with the error.
func Run(tx pgx.Tx, fn func(Tx) error) error {
	sql := Tx{tx: tx, proof: &roleProof{}}
	if err := fn(sql); err != nil {
		return err
	}
	if sql.proof.open {
		return errRoleUnproven
	}
	return nil
}

// Role is the database role a unit's own SQL runs as: DML on schema ext and
// nothing on public. It is one role for every unit, not one per unit.
const Role = "ext_runtime"

const (
	// pgInsufficientPrivilege is the SQLSTATE of a statement that names a table
	// the role holds nothing on.
	pgInsufficientPrivilege = "42501"
	// pgInFailedTransaction is the SQLSTATE of a command sent to a transaction
	// that an earlier error already aborted.
	pgInFailedTransaction = "25P02"
)

// errRoleLeftUnaborted is the one answer that is no refusal: a statement left the
// unit role and the transaction it ran in could not be aborted.
var errRoleLeftUnaborted = errors.New("unitsql: a statement left the unit role and the transaction could not be aborted")

// takeRole switches the transaction to the unit role for one statement. It is
// SET LOCAL, so a switch never lifted ends with the transaction. That fails
// closed: the core's own door is refused, not given the unit's rights.
func (t Tx) takeRole(ctx context.Context) error {
	t.proof.open = true
	if _, err := t.tx.Exec(ctx, "SET LOCAL ROLE "+Role); err != nil {
		return fmt.Errorf("unitsql: confining the unit's SQL to its own tables: %w", err)
	}
	return nil
}

func (t Tx) resetRole(ctx context.Context) error {
	if _, err := t.tx.Exec(ctx, "RESET ROLE"); err != nil {
		return fmt.Errorf("unitsql: lifting the unit's SQL confinement: %w", err)
	}
	t.proof.open = false
	return nil
}

// liftRole ends the confinement after a statement that succeeded. The session
// user can always switch to any role it may use, so a unit's own statement
// (`DO`, `set_config('role', ...)`) can leave the unit role. A statement that
// ended in another role is caught here and its transaction aborted, so nothing
// it did commits. One that switched away and back inside itself is not seen:
// the role stops mistakes, not a unit that means harm.
func (t Tx) liftRole(ctx context.Context) error {
	role, err := t.currentRole(ctx)
	if err != nil {
		return err
	}
	if role != Role {
		if _, err := t.tx.Exec(ctx, "SELECT 1/0"); err == nil {
			return errRoleLeftUnaborted
		}
		return fmt.Errorf("%w: a statement changed the database role, so the transaction is rolled back", extension.ErrCoreTableRefused)
	}
	return t.resetRole(ctx)
}

func (t Tx) currentRole(ctx context.Context) (string, error) {
	rows, err := t.tx.Query(ctx, "SELECT current_user::text")
	if err == nil {
		var role string
		if role, err = pgx.CollectOneRow(rows, pgx.RowTo[string]); err == nil {
			return role, nil
		}
	}
	return "", fmt.Errorf("unitsql: checking that the unit's SQL stayed confined: %w", err)
}

// settle ends the confinement after a statement and answers what the unit
// sees. A failed statement does not always abort the transaction: an argument
// the client cannot encode never reaches the server. So the role is reset on
// that path too, unless the transaction is aborted and nothing more can run.
func (t Tx) settle(ctx context.Context, stmtErr error) error {
	if stmtErr == nil {
		return t.liftRole(ctx)
	}
	if resetErr := t.resetRole(ctx); resetErr != nil && !inFailedTransaction(resetErr) {
		return fmt.Errorf("%w (and %w)", coreTableRefusal(stmtErr), resetErr)
	}
	return coreTableRefusal(stmtErr)
}

func inFailedTransaction(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == pgInFailedTransaction
}

// coreTableRefusal names what the unit did when the database refused a
// privilege, and passes every other failure through.
func coreTableRefusal(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == pgInsufficientPrivilege {
		return extension.ErrCoreTableRefused
	}
	return err
}

// unitStatement is the first argument of every statement a unit runs. The
// extended protocol takes one statement per call, where the simple one runs a
// string of them, and a unit's `RESET ROLE; ...` would lift the confinement.
const unitStatement = pgx.QueryExecModeDescribeExec

// execOne runs one statement through Query. pgx sends an Exec with no arguments
// over the simple protocol whatever the mode says, and Query honours the mode.
func (t Tx) execOne(ctx context.Context, sql string, args []any) (int64, error) {
	cursor, err := t.tx.Query(ctx, sql, append([]any{unitStatement}, args...)...)
	if err != nil {
		return 0, err
	}
	cursor.Close()
	return cursor.CommandTag().RowsAffected(), cursor.Err()
}

// Exec runs one statement as ext_runtime and answers how many rows it changed.
func (t Tx) Exec(ctx context.Context, sql string, args ...any) (int64, error) {
	if err := t.takeRole(ctx); err != nil {
		return 0, err
	}
	n, err := t.execOne(ctx, sql, args)
	if err := t.settle(ctx, err); err != nil {
		return 0, err
	}
	return n, nil
}

// Query runs one statement as ext_runtime; the role is checked when the cursor closes.
//
//nolint:ireturn // Rows is the published cursor; a unit must never hold pgx.Rows.
func (t Tx) Query(ctx context.Context, sql string, args ...any) (extension.Rows, error) {
	if err := t.takeRole(ctx); err != nil {
		return nil, err
	}
	cursor, err := t.tx.Query(ctx, sql, append([]any{unitStatement}, args...)...)
	if err != nil {
		return nil, t.settle(ctx, err)
	}
	return &rows{rows: cursor, settle: func(e error) error { return t.settle(ctx, e) }}, nil
}

// QueryRow runs one statement as ext_runtime; the role is checked when Scan returns.
//
//nolint:ireturn // Row is the published deferred read; the error is deferred to Scan by design.
func (t Tx) QueryRow(ctx context.Context, sql string, args ...any) extension.Row {
	if err := t.takeRole(ctx); err != nil {
		return row{err: err}
	}
	return row{
		row:    t.tx.QueryRow(ctx, sql, append([]any{unitStatement}, args...)...),
		settle: func(e error) error { return t.settle(ctx, e) },
	}
}

// rows is pgx's cursor behind the published one. The role is settled
// once the cursor is finished, by Close or by running out of rows, and what
// settling found surfaces through Err.
type rows struct {
	rows      pgx.Rows
	settle    func(error) error
	settled   bool
	settleErr error
}

func (r *rows) settleOnce() {
	if r.settled {
		return
	}
	r.settled = true
	r.settleErr = r.settle(r.rows.Err())
}

func (r *rows) Next() bool {
	if r.rows.Next() {
		return true
	}
	r.rows.Close()
	r.settleOnce()
	return false
}

func (r *rows) Scan(dest ...any) error { return r.rows.Scan(dest...) }

func (r *rows) Err() error {
	if !r.settled {
		return coreTableRefusal(r.rows.Err())
	}
	return r.settleErr
}

func (r *rows) Close() {
	r.rows.Close()
	r.settleOnce()
}

// row defers one read's error to Scan and translates the empty match
// into the published sentinel, so a unit never binds to pgx.ErrNoRows. An empty
// match leaves the transaction usable, so the role is settled on that path too.
type row struct {
	row    pgx.Row
	settle func(error) error
	err    error
}

func (r row) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	err := r.row.Scan(dest...)
	empty := errors.Is(err, pgx.ErrNoRows)
	if empty {
		err = nil
	}
	if err = r.settle(err); err != nil {
		return err
	}
	if empty {
		return extension.ErrNoRows
	}
	return nil
}
