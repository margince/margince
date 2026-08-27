// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contracts

// The three ways the update shape can fail, which the integration suite cannot
// show because each needs the database to refuse a statement that normally
// succeeds.
//
// All three matter for the same reason. An update is a domain row, an audit row
// and an event that land in ONE transaction, so a step that fails and does not
// say so leaves the transaction to commit the parts that worked: the row moves
// and the trail does not, or the row and the trail move and no consumer hears.
// Each failure has to reach the caller, and each has to say which step it was —
// "something went wrong writing a contract" does not tell an operator whether
// to look at the row, the audit table or the outbox.

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// updatingContext carries what every real write path binds before reaching the
// store: the actor the audit row is filed under, and the correlation id that
// links that row to the event. Emit refuses without the second, so a context
// missing it fails the shape one step earlier than the step under test.
func updatingContext() context.Context {
	ctx := principal.WithActor(context.Background(),
		principal.Principal{Type: principal.PrincipalHuman, ID: "human:" + ids.NewV7().String()})
	return principal.WithCorrelationID(ctx, ids.NewV7())
}

// aContractPatch carries one column, so the patch is not empty and the shape
// runs all the way through rather than returning early.
func aContractPatch() *storekit.Patch {
	patch := storekit.NewPatch()
	patch.Set("title", nil, "renamed")
	return patch
}

func TestARefusedContractPatchReachesTheCaller(t *testing.T) {
	refused := errors.New("deadlock detected")
	tx := &failingTx{failOn: contractTable, err: refused}
	err := applyContractUpdate(updatingContext(), tx, ids.New[ids.ContractKind](),
		aContractPatch(), nil, "contract update")
	if !errors.Is(err, refused) {
		t.Fatalf("the database's refusal did not reach the caller: %v", err)
	}
	if !strings.Contains(err.Error(), "write contract update") {
		t.Errorf("the error does not say which step failed: %v", err)
	}
}

// The audit row is the half a reader would not notice missing. A patch that
// lands while its audit insert fails silently is a change with no trail, and
// the row itself looks exactly like one that was recorded properly.
func TestARefusedAuditRowStopsTheContractUpdate(t *testing.T) {
	refused := errors.New("audit_log is full")
	tx := &failingTx{failOn: "audit_log", err: refused}
	err := applyContractUpdate(updatingContext(), tx, ids.New[ids.ContractKind](),
		aContractPatch(), nil, "contract update")
	if !errors.Is(err, refused) {
		t.Fatalf("a refused audit row did not reach the caller: %v", err)
	}
	if !strings.Contains(err.Error(), "audit contract update") {
		t.Errorf("the error does not say which step failed: %v", err)
	}
}

// The event is the half no consumer would notice missing, because a consumer
// that never receives a message cannot tell it apart from one that was never
// sent.
func TestARefusedEventStopsTheContractUpdate(t *testing.T) {
	refused := errors.New("outbox unavailable")
	tx := &failingTx{failOn: "event_outbox", err: refused}
	err := applyContractUpdate(updatingContext(), tx, ids.New[ids.ContractKind](),
		aContractPatch(), nil, "contract update")
	if !errors.Is(err, refused) {
		t.Fatalf("a refused event did not reach the caller: %v", err)
	}
	if !strings.Contains(err.Error(), "emit contract.updated") {
		t.Errorf("the error does not say which step failed: %v", err)
	}
}

// A CHECK the database refuses is not an infrastructure failure — it is the
// caller having asked for something the schema forbids, and it has to arrive as
// the field-level refusal a caller can act on rather than as a wrapped driver
// error. The shape owns that translation for both verbs, so it is tested where
// the translation lives.
func TestARefusedCheckBecomesAFieldLevelRefusal(t *testing.T) {
	tx := &failingTx{failOn: contractTable, err: &pgconn.PgError{
		Code: "23514", ConstraintName: "contract_term_order",
	}}
	err := applyContractUpdate(updatingContext(), tx, ids.New[ids.ContractKind](),
		aContractPatch(), nil, "contract update")
	var refusal *ContractCheckError
	if !errors.As(err, &refusal) {
		t.Fatalf("a refused CHECK did not become a field-level refusal: %v", err)
	}
	if refusal.Field != "ends_on" {
		t.Errorf("the refusal names %q, so a caller is pointed at the wrong field", refusal.Field)
	}
}

// The shape's own success, which is what makes the four refusals above mean
// something: the same fake, refusing nothing, must reach the end AND send all
// three writes. A shape that quietly sent two of them would pass every refusal
// test above, because a write that never happens cannot be refused.
func TestTheContractUpdateShapeSendsAllThreeWrites(t *testing.T) {
	tx := &failingTx{failOn: "a string no statement contains"}
	if err := applyContractUpdate(updatingContext(), tx, ids.New[ids.ContractKind](),
		aContractPatch(), nil, "contract update"); err != nil {
		t.Fatalf("the update shape refused a transaction that accepted everything: %v", err)
	}
	for _, table := range []string{contractTable, "audit_log", "event_outbox"} {
		sent := false
		for _, sql := range tx.seen {
			if strings.Contains(sql, table) {
				sent = true
			}
		}
		if !sent {
			t.Errorf("nothing was written to %s: an update is a row, a trail and an event, and "+
				"a shape that sends two of the three commits a change nobody can see", table)
		}
	}
}

// failingTx refuses the first statement naming failOn and answers every other
// one as though it had worked, so each test breaks exactly one step of the
// shape and the steps before it are reached for real.
//
// It panics on the methods the shape does not use: a future step reaching for
// the database another way fails loudly here rather than being answered by a
// zero value that makes the test pass for the wrong reason.
type failingTx struct {
	failOn string
	err    error
	seen   []string
}

func (f *failingTx) matches(sql string) bool {
	f.seen = append(f.seen, sql)
	return strings.Contains(sql, f.failOn)
}

func (f *failingTx) Exec(_ context.Context, sql string, _ ...any) (pgconn.CommandTag, error) {
	if f.matches(sql) {
		return pgconn.CommandTag{}, f.err
	}
	return pgconn.NewCommandTag("UPDATE 1"), nil
}

func (f *failingTx) QueryRow(_ context.Context, sql string, _ ...any) pgx.Row {
	if f.matches(sql) {
		return errorRow{err: f.err}
	}
	return newIDRow()
}

func (f *failingTx) Begin(context.Context) (pgx.Tx, error) { panic("failingTx: Begin") }
func (f *failingTx) Commit(context.Context) error          { panic("failingTx: Commit") }
func (f *failingTx) Rollback(context.Context) error        { panic("failingTx: Rollback") }
func (f *failingTx) Conn() *pgx.Conn                       { panic("failingTx: Conn") }
func (f *failingTx) LargeObjects() pgx.LargeObjects        { panic("failingTx: LargeObjects") }

func (f *failingTx) CopyFrom(context.Context, pgx.Identifier, []string, pgx.CopyFromSource) (int64, error) {
	panic("failingTx: CopyFrom")
}

func (f *failingTx) SendBatch(context.Context, *pgx.Batch) pgx.BatchResults {
	panic("failingTx: SendBatch")
}

func (f *failingTx) Prepare(context.Context, string, string) (*pgconn.StatementDescription, error) {
	panic("failingTx: Prepare")
}

func (f *failingTx) Query(context.Context, string, ...any) (pgx.Rows, error) {
	panic("failingTx: Query")
}

// errorRow hands the refusal back through Scan, which is where a QueryRow
// reports one.
type errorRow struct{ err error }

func (r errorRow) Scan(...any) error { return r.err }

// idRow answers a scan for a generated identifier, which is what the audit
// insert asks for.
type idRow struct{ id ids.UUID }

func newIDRow() idRow { return idRow{id: ids.NewV7()} }

func (r idRow) Scan(into ...any) error {
	for _, target := range into {
		if slot, isUUID := target.(*ids.UUID); isUUID {
			*slot = r.id
		}
	}
	return nil
}
