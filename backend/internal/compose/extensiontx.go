// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The published transaction seam over pgx: the three SQL verbs a unit's own
// tables need, the door onto the governed core port, and the two cursor
// adapters behind Rows and Row. Split from extruntime.go, which owns the
// Runtime's lifetime and scope, when the Runtime grew the port — these are the
// types a unit HOLDS, and that is one concern.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/margince/margince/backend/pkg/extension"
)

// extensionTx bridges the published three-verb seam to pgx. It lives in
// internal/ and not in pkg/ because pkg/ is stdlib-only (depguard
// pkg-purity, and TestPublishedSurfaceIsPure): a published interface can
// describe a transaction, but only the core can hold one.
//
// There is no lifetime guard on this type. A Tx used after its transaction
// ended answers pgx's own ErrTxClosed, which is already an honest refusal —
// and it is the accurate one, because the transaction can end while the
// Runtime is still perfectly live (the callback returned, the call did not).
// ErrRuntimeExpired there would name the wrong fault.
type extensionTx struct {
	tx pgx.Tx
	// core is what the transaction can reach BESIDE the unit's own SQL, and it
	// is built by the Runtime rather than by this type: whether the invocation
	// has a caller, and what the role bound at boot, are facts about the call,
	// not about the transaction.
	core extensionCore
	// ledger records what the unit's OWN SQL did — the audit row and the bus
	// event the three verbs below cannot write for it. Built by the Runtime for
	// the same reason core is: which unit is writing, and under whose identity,
	// are facts about the invocation rather than about the transaction.
	ledger extensionLedger
}

//nolint:ireturn // returning the published port IS the seam: a unit holds extension.Core, never a core type.
func (t extensionTx) Core() extension.Core {
	return t.core
}

func (t extensionTx) Record(ctx context.Context, ch extension.Change, ev extension.Event) error {
	return t.ledger.Record(ctx, ch, ev)
}

// extRuntimeRole is the database role a unit's own SQL runs as: DML on schema
// ext and nothing on public. It is one role for every unit, not one per unit.
const extRuntimeRole = "ext_runtime"

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
var errRoleLeftUnaborted = errors.New("compose: a statement left the unit role and the transaction could not be aborted")

// takeRole switches the transaction to the unit role for one statement. It is
// SET LOCAL, so a switch never lifted ends with the transaction. That fails
// closed: the core's own door is refused, not given the unit's rights.
func (t extensionTx) takeRole(ctx context.Context) error {
	if _, err := t.tx.Exec(ctx, "SET LOCAL ROLE "+extRuntimeRole); err != nil {
		return fmt.Errorf("compose: confining the unit's SQL to its own tables: %w", err)
	}
	return nil
}

func (t extensionTx) resetRole(ctx context.Context) error {
	if _, err := t.tx.Exec(ctx, "RESET ROLE"); err != nil {
		return fmt.Errorf("compose: lifting the unit's SQL confinement: %w", err)
	}
	return nil
}

// liftRole ends the confinement after a statement that succeeded. The session
// user can always switch to any role it may use, so a unit's own statement
// (`DO`, `set_config('role', ...)`) can leave the unit role. A statement that
// ended in another role is caught here and its transaction aborted, so nothing
// it did commits. One that switched away and back inside itself is not seen:
// the role stops mistakes, not a unit that means harm.
func (t extensionTx) liftRole(ctx context.Context) error {
	role, err := t.currentRole(ctx)
	if err != nil {
		return err
	}
	if role != extRuntimeRole {
		if _, err := t.tx.Exec(ctx, "SELECT 1/0"); err == nil {
			return errRoleLeftUnaborted
		}
		return fmt.Errorf("%w: a statement changed the database role, so the transaction is rolled back", extension.ErrCoreTableRefused)
	}
	return t.resetRole(ctx)
}

func (t extensionTx) currentRole(ctx context.Context) (string, error) {
	rows, err := t.tx.Query(ctx, "SELECT current_user::text")
	if err == nil {
		var role string
		if role, err = pgx.CollectOneRow(rows, pgx.RowTo[string]); err == nil {
			return role, nil
		}
	}
	return "", fmt.Errorf("compose: checking that the unit's SQL stayed confined: %w", err)
}

// settle ends the confinement after a statement and answers what the unit
// sees. A failed statement does not always abort the transaction: an argument
// the client cannot encode never reaches the server. So the role is reset on
// that path too, unless the transaction is aborted and nothing more can run.
func (t extensionTx) settle(ctx context.Context, stmtErr error) error {
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
func (t extensionTx) execOne(ctx context.Context, sql string, args []any) (int64, error) {
	rows, err := t.tx.Query(ctx, sql, append([]any{unitStatement}, args...)...)
	if err != nil {
		return 0, err
	}
	rows.Close()
	return rows.CommandTag().RowsAffected(), rows.Err()
}

func (t extensionTx) Exec(ctx context.Context, sql string, args ...any) (int64, error) {
	if err := t.takeRole(ctx); err != nil {
		return 0, err
	}
	n, err := t.execOne(ctx, sql, args)
	if err := t.settle(ctx, err); err != nil {
		return 0, err
	}
	return n, nil
}

//nolint:ireturn // Rows is the published cursor; a unit must never hold pgx.Rows.
func (t extensionTx) Query(ctx context.Context, sql string, args ...any) (extension.Rows, error) {
	if err := t.takeRole(ctx); err != nil {
		return nil, err
	}
	rows, err := t.tx.Query(ctx, sql, append([]any{unitStatement}, args...)...)
	if err != nil {
		return nil, t.settle(ctx, err)
	}
	return &extensionRows{rows: rows, settle: func(e error) error { return t.settle(ctx, e) }}, nil
}

//nolint:ireturn // Row is the published deferred read; the error is deferred to Scan by design.
func (t extensionTx) QueryRow(ctx context.Context, sql string, args ...any) extension.Row {
	if err := t.takeRole(ctx); err != nil {
		return extensionRow{err: err}
	}
	return extensionRow{
		row:    t.tx.QueryRow(ctx, sql, append([]any{unitStatement}, args...)...),
		settle: func(e error) error { return t.settle(ctx, e) },
	}
}

// extensionRows is pgx's cursor behind the published one. The role is settled
// once the cursor is finished, by Close or by running out of rows, and what
// settling found surfaces through Err.
type extensionRows struct {
	rows      pgx.Rows
	settle    func(error) error
	settled   bool
	settleErr error
}

func (r *extensionRows) settleOnce() {
	if r.settled {
		return
	}
	r.settled = true
	r.settleErr = r.settle(r.rows.Err())
}

func (r *extensionRows) Next() bool {
	if r.rows.Next() {
		return true
	}
	r.rows.Close()
	r.settleOnce()
	return false
}

func (r *extensionRows) Scan(dest ...any) error { return r.rows.Scan(dest...) }

func (r *extensionRows) Err() error {
	if !r.settled {
		return coreTableRefusal(r.rows.Err())
	}
	return r.settleErr
}

func (r *extensionRows) Close() {
	r.rows.Close()
	r.settleOnce()
}

// extensionRow defers one read's error to Scan and translates the empty match
// into the published sentinel, so a unit never binds to pgx.ErrNoRows. An empty
// match leaves the transaction usable, so the role is settled on that path too.
type extensionRow struct {
	row    pgx.Row
	settle func(error) error
	err    error
}

func (r extensionRow) Scan(dest ...any) error {
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
