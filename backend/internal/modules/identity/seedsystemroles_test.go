// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

// A role set that cannot be laid down says so.
//
// SeedSystemRoles is the ONE writer of the shipped role documents — Bootstrap
// calls it to provision a workspace, and the integration harness calls it so a
// fixture seat can hold a real role rather than a hand-written policy. Both
// depend on the same thing: that a failure to insert is reported, not skipped.
//
// A swallowed error here is a workspace whose first user holds an admin role
// that was never written, or a fixture whose every seat resolves to no
// permissions while its tests read as protection. Neither announces itself —
// the caller sees a nil error and a zero role id, and the next authorization
// check is where it surfaces, a long way from the cause.
//
// The database is the true boundary, so it is the only thing faked here, and
// the fake implements one method: Bootstrap and the harness both reach this
// through a real transaction, and the path that CANNOT be driven through one is
// a server refusing the insert.

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

var errRoleInsertRefused = errors.New("permission denied for table role")

func TestSeedSystemRolesReportsARefusedInsert(t *testing.T) {
	t.Parallel()
	tx := &refusingTx{err: errRoleInsertRefused}

	adminRoleID, err := SeedSystemRoles(context.Background(), tx)
	if !errors.Is(err, errRoleInsertRefused) {
		t.Fatalf("SeedSystemRoles answered %v, want the refusal it was given — a caller told the "+
			"role set landed when it did not provisions a workspace whose admin holds nothing", err)
	}
	if adminRoleID != ids.Nil {
		t.Errorf("a failed seed still answered role id %s; the caller assigns that id to its first "+
			"user, so inventing one writes an assignment pointing at no role", adminRoleID)
	}
	if tx.rows != 1 {
		t.Errorf("the seed attempted %d insert(s) after the first was refused, want 1 — carrying on "+
			"through the set writes a partial role set the caller is never told about", tx.rows)
	}
}

// Bootstrap's role step stops at a refused insert rather than assigning an
// admin role that was never created.
//
// The wrapper's failure mode is specific and silent: SeedSystemRoles answers
// ids.Nil alongside its error, so a wrapper that carried on would run its
// INSERT INTO role_assignment with the NIL role id — an assignment pointing at
// no role, on the one account that is supposed to be able to administer the
// installation. The insert is the next statement, which is why this asserts
// the refusal reached the caller AND that nothing was assigned.
func TestBootstrapsRoleStepDoesNotAssignARoleThatWasNeverCreated(t *testing.T) {
	t.Parallel()
	tx := &refusingTx{err: errRoleInsertRefused}

	err := seedSystemRolesForBootstrap(context.Background(), tx, ids.From[ids.UserKind](ids.NewV7()))
	if !errors.Is(err, errRoleInsertRefused) {
		t.Fatalf("the bootstrap role step answered %v, want the refusal underneath it", err)
	}
	// Exec is the assignment, and refusingTx panics on it — so reaching here at
	// all is the assertion that none was attempted.
}

// refusingTx is the true-DB-boundary fake (P3): QueryRow is the only method
// SeedSystemRoles calls, and it answers a refusal. Every other pgx.Tx method
// panics, because reaching one would be this test's own bug rather than a path
// worth stubbing.
type refusingTx struct {
	err  error
	rows int
}

func (f *refusingTx) QueryRow(context.Context, string, ...any) pgx.Row {
	f.rows++
	return refusedRow{err: f.err}
}

func (f *refusingTx) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	panic("refusingTx: Exec not implemented — SeedSystemRoles assigns nothing")
}

func (f *refusingTx) Begin(context.Context) (pgx.Tx, error) {
	panic("refusingTx: Begin not implemented")
}

func (f *refusingTx) Commit(context.Context) error {
	panic("refusingTx: Commit not implemented")
}

func (f *refusingTx) Rollback(context.Context) error {
	panic("refusingTx: Rollback not implemented")
}

func (f *refusingTx) CopyFrom(context.Context, pgx.Identifier, []string, pgx.CopyFromSource) (int64, error) {
	panic("refusingTx: CopyFrom not implemented")
}

func (f *refusingTx) SendBatch(context.Context, *pgx.Batch) pgx.BatchResults {
	panic("refusingTx: SendBatch not implemented")
}

func (f *refusingTx) LargeObjects() pgx.LargeObjects {
	panic("refusingTx: LargeObjects not implemented")
}

func (f *refusingTx) Prepare(context.Context, string, string) (*pgconn.StatementDescription, error) {
	panic("refusingTx: Prepare not implemented")
}

func (f *refusingTx) Query(context.Context, string, ...any) (pgx.Rows, error) {
	panic("refusingTx: Query not implemented")
}
func (f *refusingTx) Conn() *pgx.Conn { panic("refusingTx: Conn not implemented") }

// refusedRow is the row a refused insert answers with.
type refusedRow struct{ err error }

func (r refusedRow) Scan(...any) error { return r.err }
