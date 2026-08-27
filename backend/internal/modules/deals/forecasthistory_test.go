// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// The forecast recorder's two silent-failure modes, which are the ones the
// integration suite cannot show: an insert that matched no row, and one the
// database refused. Both have to reach the caller as errors — a recorder that
// swallows either leaves the history short by exactly the write that failed,
// and short history is indistinguishable from a forecast that did not move.

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// aMovedAmount is a patch's after-image carrying a forecast column, so the
// recorder gets past its own "nothing moved" guard and reaches the insert.
func aMovedAmount() map[string]any { return map[string]any{amountField: int64(250_000)} }

func recordingContext() context.Context {
	return principal.WithActor(context.Background(),
		principal.Principal{Type: principal.PrincipalHuman, ID: "human:" + ids.NewV7().String()})
}

// INSERT ... SELECT reports "matched nothing" as zero rows and no error. The
// recorder runs immediately after its own write inside that write's transaction,
// so a SELECT that resolves no deal is a programming error — an ordering the
// patch seams make unrepresentable, and one that would otherwise show up as a
// reconstruction quietly missing a move rather than as a failure.
func TestAForecastRowThatMatchedNoDealIsAnError(t *testing.T) {
	tx := &forecastTx{tag: pgconn.NewCommandTag("INSERT 0 0")}
	err := recordForecastMovement(recordingContext(), tx, ids.New[ids.DealKind](), aMovedAmount())
	if err == nil {
		t.Fatal("an insert that matched no deal was accepted, so the history is short and nothing says so")
	}
	if !strings.Contains(err.Error(), "did not resolve") {
		t.Errorf("the error does not say what went wrong: %v", err)
	}
}

func TestARefusedForecastRowReachesTheCaller(t *testing.T) {
	refused := errors.New("deadlock detected")
	tx := &forecastTx{execErr: refused}
	err := recordForecastMovement(recordingContext(), tx, ids.New[ids.DealKind](), aMovedAmount())
	if !errors.Is(err, refused) {
		t.Fatalf("the database's refusal did not reach the caller: %v", err)
	}
}

// A write that moved no forecast column must not reach the database at all —
// otherwise the table answers "the forecast moved" for every edit anyone makes.
func TestAWriteThatMovedNoForecastColumnSendsNothing(t *testing.T) {
	tx := &forecastTx{tag: pgconn.NewCommandTag("INSERT 0 1")}
	if err := recordForecastMovement(recordingContext(), tx,
		ids.New[ids.DealKind](), map[string]any{dealNameColumn: "renamed"}); err != nil {
		t.Fatalf("recording a rename: %v", err)
	}
	if tx.execSQL != "" {
		t.Errorf("a rename sent a statement: %s", tx.execSQL)
	}
}

// forecastTx answers the one method the recorder uses and panics on the rest,
// so a future call reaching for the database through this fake fails loudly
// rather than being quietly answered by a zero value.
type forecastTx struct {
	execSQL string
	execErr error
	tag     pgconn.CommandTag
}

func (f *forecastTx) Exec(_ context.Context, sql string, _ ...any) (pgconn.CommandTag, error) {
	f.execSQL = sql
	if f.execErr != nil {
		return pgconn.CommandTag{}, f.execErr
	}
	return f.tag, nil
}

func (f *forecastTx) Begin(context.Context) (pgx.Tx, error) { panic("forecastTx: Begin") }
func (f *forecastTx) Commit(context.Context) error          { panic("forecastTx: Commit") }
func (f *forecastTx) Rollback(context.Context) error        { panic("forecastTx: Rollback") }
func (f *forecastTx) Conn() *pgx.Conn                       { panic("forecastTx: Conn") }
func (f *forecastTx) LargeObjects() pgx.LargeObjects        { panic("forecastTx: LargeObjects") }

func (f *forecastTx) CopyFrom(context.Context, pgx.Identifier, []string, pgx.CopyFromSource) (int64, error) {
	panic("forecastTx: CopyFrom")
}

func (f *forecastTx) SendBatch(context.Context, *pgx.Batch) pgx.BatchResults {
	panic("forecastTx: SendBatch")
}

func (f *forecastTx) Prepare(context.Context, string, string) (*pgconn.StatementDescription, error) {
	panic("forecastTx: Prepare")
}

func (f *forecastTx) Query(context.Context, string, ...any) (pgx.Rows, error) {
	panic("forecastTx: Query")
}

func (f *forecastTx) QueryRow(context.Context, string, ...any) pgx.Row {
	panic("forecastTx: QueryRow")
}
