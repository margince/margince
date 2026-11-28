// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package database

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// DB is a pool that resolves the installation's one workspace itself. A caller
// never passes or binds one.
//
// No table carries workspace_id and no policy reads one, so a transaction is not
// scoped by it. What Tx checks is that the installation has a company at all.
// A handle on a host with no live company refuses before the body runs.
// A handle from NewAcrossWorkspaces skips that check, for cleanup that sends
// nothing out.
//
// A transaction proves the company is live with one indexed statement inside
// itself, and a handle repeats that proof at most once per InstallationRecheck.
// So an archived company stops admitting transactions within that bound, not
// never. Workspace always looks the company up fresh.
type DB struct {
	pool *pgxpool.Pool
	// installation is true on a ForInstallation handle.
	installation bool
	// workspace pins the answer on a BindTo handle and is nil on the others.
	workspace func(context.Context) (ids.WorkspaceID, error)
	// budget bounds every statement this handle's transactions run, and is zero
	// on a handle nobody bounded — see Bounded.
	budget   time.Duration
	admitted bool
	// proven holds the newest check of the company. Copies of a handle share
	// it. See stampProof.
	proven  *atomic.Pointer[proof]
	recheck time.Duration
	// now dates a check and the window it is trusted for. Nil means time.Now.
	now func() time.Time
}

// proof is one check of the installation's company. at is unix nanoseconds,
// negative when no company was live.
type proof struct {
	at      int64
	company ids.WorkspaceID
}

func (d *DB) clock() time.Time {
	if d.now == nil {
		return time.Now()
	}
	return d.now()
}

// WithClock is this handle dating its checks by now, so a test can freeze the
// recheck window.
func (d *DB) WithClock(now func() time.Time) *DB {
	if d == nil {
		return nil
	}
	dated := *d
	dated.now = now
	return &dated
}

// InstallationRecheck is how long a ForInstallation handle trusts its last
// proof that a company is live. It bounds how long an archived company keeps
// admitting transactions. The Home and capture round-trip budgets leave no room
// for a statement in every transaction.
const InstallationRecheck = 2 * time.Second

// Rechecking is this handle proving the company is live at most once per every.
// Zero proves it in every transaction.
func (d *DB) Rechecking(every time.Duration) *DB {
	if d == nil {
		return nil
	}
	rechecked := *d
	rechecked.recheck = every
	return &rechecked
}

// Bounded is this handle with a time ceiling on every statement its
// transactions run.
//
// It binds the budget to the HANDLE rather than to a call site because the
// paths that need one do not run a single statement: answering an agent's query
// plan takes a ranking lane and an exact lane, each opening its own
// transaction, and a ceiling added per call site is a ceiling the next lane
// silently does without. A store built on a bounded handle cannot grow an
// unbounded query.
//
// ZERO is the one value that cannot be a ceiling: it is the field's zero value
// on every handle nobody bounded, so it has to mean unbounded. Every other
// value is armed, which is what keeps a NEGATIVE budget from disappearing —
// BoundStatement refuses it on the first transaction rather than leaving a
// handle that quietly runs without the ceiling it was asked for.
func (d *DB) Bounded(budget time.Duration) *DB {
	// Nil-safe for the reason Pool and Tx are: CONSTRUCTION reaches this. A
	// store built from an un-injected handle is a real thing in this tree — the
	// unit tests that assert a gate answering before any query build one — and
	// bounding it must fail where it is USED, with the sentinel those tests key
	// on, rather than panicking where it is wired. A handle that runs no
	// statements has nothing to bound anyway.
	if d == nil {
		return nil
	}
	bounded := *d
	bounded.budget = budget
	return &bounded
}

// Budget is the per-statement ceiling Bounded armed, or zero on a handle
// nobody bounded. Nil-safe like Bounded.
func (d *DB) Budget() time.Duration {
	if d == nil {
		return 0
	}
	return d.budget
}

// ForInstallation returns a handle that looks the installation's live workspace
// up in the database on every use. The name is not New because the object-gate
// censuses in backend/gates resolve calls by name, and a callee named New
// collides there.
func ForInstallation(pool *pgxpool.Pool) *DB {
	return &DB{pool: pool, installation: true, proven: new(atomic.Pointer[proof]), recheck: InstallationRecheck}
}

// NewAcrossWorkspaces returns a handle whose transactions need no live
// workspace. Only cleanup that sends nothing out may use it.
// Archiving a company does not un-store its data, so the retention floor binds.
// Workspace answers ErrNoWorkspace on this handle.
func NewAcrossWorkspaces(pool *pgxpool.Pool) *DB {
	return &DB{pool: pool}
}

// BindTo returns a handle pinned to one workspace. Test setup uses it for a
// suite that seeds a second workspace. The gate
// TestNoProductionCodePinsADatabaseHandleToAWorkspace keeps production code on
// ForInstallation.
func BindTo(pool *pgxpool.Pool, ws ids.WorkspaceID) *DB {
	return &DB{pool: pool, workspace: func(context.Context) (ids.WorkspaceID, error) { return ws, nil }}
}

// Workspace reports which workspace this handle binds, for the callers that
// need to name it rather than run in it — a job asserting it was wired to the
// tenant its args declare, or a store naming the row its own transaction will
// write.
//
// Nil-safe for the reason Tx, Pool and Bounded are, and with Tx's exact answer:
// a store built from an un-injected handle is a real thing in this tree, and a
// caller that asks this before opening a transaction must fail the way it would
// have failed opening one. Returning the sentinel rather than panicking is what
// keeps "resolve the workspace, then run" from being more fragile than "run".
func (d *DB) Workspace(ctx context.Context) (ids.WorkspaceID, error) {
	if d == nil {
		return ids.WorkspaceID{}, fmt.Errorf("%w: no database handle was injected; "+
			"construct this store through compose, which binds the installation's pool",
			ErrNoWorkspace)
	}
	switch {
	case d.installation:
		return d.installationWorkspace(ctx)
	case d.workspace != nil:
		return d.workspace(ctx)
	}
	return ids.WorkspaceID{}, fmt.Errorf("%w: this handle spans every workspace and names none", ErrNoWorkspace)
}

// Pool exposes the underlying pool for the paths that do not run a
// transaction — the outbox relay's listener, the health probe.
//
// Nil-safe on a nil handle, because construction reaches it: a store built
// from an un-injected handle would otherwise panic where it is WIRED rather
// than where it is used, and the unit tests that build a handler with no
// database at all — to assert a gate that answers before any query — are
// exactly the callers that would crash.
func (d *DB) Pool() *pgxpool.Pool {
	if d == nil {
		return nil
	}
	return d.pool
}

// Tx runs fn inside one transaction, with WithTx's contract. This handle
// resolves the installation's workspace itself, and refuses before any SQL runs
// when it cannot.
func (d *DB) Tx(ctx context.Context, fn func(pgx.Tx) error) error {
	return d.transact(ctx, pgx.TxOptions{}, fn)
}

// TxIsolated is Tx at a chosen isolation level, for the reads whose answer is
// COMPOSED from several statements.
//
// The level belongs at BEGIN and not inside the closure, and that is the whole
// reason this exists rather than a `SET TRANSACTION ISOLATION LEVEL` at the top
// of a caller: Postgres refuses the statement once a query has taken a
// snapshot, and Tx already runs one of its own on a bounded handle (the
// statement-timeout set_config). A caller that pinned itself would work on an
// unbounded handle and fail on a bounded one — the same code, correct or broken
// depending on how compose wired it.
func (d *DB) TxIsolated(ctx context.Context, level pgx.TxIsoLevel, fn func(pgx.Tx) error) error {
	return d.transact(ctx, pgx.TxOptions{IsoLevel: level}, fn)
}

// transact is what both of them are: the handle's own admission, the statement
// budget, and one transaction.
//
// ONE body, because the two differ in a transaction OPTION and in nothing else.
// Written out twice, the refusal a nil handle owes its callers, the workspace
// resolution that must happen before any SQL, and the budget wrapper would each
// be two places — and a change to the transaction contract would reach the
// isolated read only if whoever made it went looking for a second copy.
func (d *DB) transact(ctx context.Context, opts pgx.TxOptions, fn func(pgx.Tx) error) error {
	if d == nil {
		// A store built without a handle, answered with the sentinel that
		// already means "no workspace could be bound, and no SQL ran".
		//
		// It returns rather than panicking because that is how this degraded
		// before the collapse, and the sentinel is kept because callers key on
		// it: the gate tests distinguish "the read gate denied me" from "the
		// gate admitted me and the probe reached a database it could not bind"
		// by exactly this error, and that distinction is the thing they exist
		// to protect.
		return fmt.Errorf("%w: no database handle was injected; "+
			"construct this store through compose, which binds the installation's pool",
			ErrNoWorkspace)
	}
	// Resolved for its refusal, not for its value: nothing in the transaction
	// keys on the workspace, but a handle that cannot name the installation's
	// one workspace has not been reached through a bootstrapped install, and
	// that is a fault the caller must see before any SQL runs.
	if d.workspace != nil {
		if _, err := d.workspace(ctx); err != nil {
			return fmt.Errorf("pg: resolving the installation's workspace: %w", err)
		}
	}
	_, joined := snapshotOf(ctx)
	if _, proven := d.provenCompany(); d.installation && !proven {
		body := fn
		fn = func(tx pgx.Tx) error {
			started := d.clock().UnixNano()
			live, err := InstallationWorkspaceTx(ctx, tx)
			if err != nil {
				stampProof(d.proven, proof{at: -d.clock().UnixNano()})
				return fmt.Errorf("pg: resolving the installation's workspace: %w", err)
			}
			// A joined snapshot can be older than this check, so its answer
			// would stamp a moment it does not describe.
			if !joined {
				stampProof(d.proven, proof{at: started, company: ids.From[ids.WorkspaceKind](live)})
			}
			return body(tx)
		}
	}
	// The ambient snapshot, joined here for the reason WithTx joins it. Most
	// stores reach this seam. A join spelled only in WithTx would leave a
	// composed read holding two connections per request, the snapshot plus one
	// per handle-based lane. At MaxConns that is how a morning's concurrent reads
	// wait on each other instead of the database.
	//
	// The budget is NOT applied to a joined transaction. BoundStatement is SET
	// LOCAL, so a per-handle ceiling would silently re-time every later lane in
	// somebody else's snapshot; the transaction's own opener sets its budget.
	if tx, joined := snapshotOf(ctx); joined {
		return guardedTransaction(ctx, tx, fn)
	}
	if d.budget != 0 {
		bounded := fn
		fn = func(tx pgx.Tx) error {
			if err := BoundStatement(ctx, tx, d.budget); err != nil {
				return err
			}
			return bounded(tx)
		}
	}
	return runTxWith(ctx, d.pool, opts, fn)
}

// provenCompany answers the company the last check found live, while that
// check is inside the recheck window.
func (d *DB) provenCompany() (ids.WorkspaceID, bool) {
	last := d.proven.Load()
	if last == nil || last.at <= 0 || d.clock().Sub(time.Unix(0, last.at)) >= d.recheck {
		return ids.WorkspaceID{}, false
	}
	return last.company, true
}

// installationWorkspace answers the proven company inside the window and looks
// it up otherwise.
func (d *DB) installationWorkspace(ctx context.Context) (ids.WorkspaceID, error) {
	if company, ok := d.provenCompany(); ok {
		return company, nil
	}
	started := d.clock().UnixNano()
	live, err := InstallationWorkspace(ctx, d.pool)
	if err != nil {
		stampProof(d.proven, proof{at: -d.clock().UnixNano()})
		return ids.WorkspaceID{}, err
	}
	company := ids.From[ids.WorkspaceKind](live)
	stampProof(d.proven, proof{at: started, company: company})
	return company, nil
}

// stampProof records a check, and only ever moves the stamp forward by its
// absolute time. A slow transaction that proved the company live cannot
// overwrite a newer failure or a newer success.
func stampProof(held *atomic.Pointer[proof], next proof) {
	for {
		current := held.Load()
		if current != nil && abs64(current.at) >= abs64(next.at) {
			return
		}
		if held.CompareAndSwap(current, &next) {
			return
		}
	}
}

func abs64(n int64) int64 {
	if n < 0 {
		return -n
	}
	return n
}

// WithReadAdmission opts expensive reads into the pool's shared allowance.
func (d *DB) WithReadAdmission() *DB {
	if d == nil {
		return nil
	}
	admitted := *d
	admitted.admitted = true
	return &admitted
}

// Read passes the admitted context onward so nested reads share its allowance.
func (d *DB) Read(ctx context.Context, fn func(context.Context, pgx.Tx) error) error {
	read := func(ctx context.Context) error {
		return d.Tx(ctx, func(tx pgx.Tx) error { return fn(ctx, tx) })
	}
	if d == nil || !d.admitted {
		return read(ctx)
	}
	return WithReadAdmission(ctx, d.pool, read)
}
