// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package search

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
)

// queryingTx records the one query a ranking issues. The embedded interface is
// nil deliberately: any other call is one this helper was never asked to stand
// in for, and a panic says so louder than a zero value.
type queryingTx struct {
	pgx.Tx
	queryErr error
	args     []any
}

func (q *queryingTx) Query(_ context.Context, _ string, args ...any) (pgx.Rows, error) {
	q.args = args
	return nil, q.queryErr
}

// pgx reads a leading QueryExecMode as the mode for this query alone; any other
// mode on the product pool names the statement, and a named one turns generic.
func TestTheRankingRunsAsAnUnnamedStatement(t *testing.T) {
	tx := &queryingTx{queryErr: errors.New("stop before reading rows")}
	_, err := rank(context.Background(), tx, "SELECT $1, $2", []any{"vossberg", 25})
	if err == nil {
		t.Fatal("rank swallowed the query's failure")
	}
	want := []any{pgx.QueryExecModeDescribeExec, "vossberg", 25}
	if len(tx.args) != len(want) {
		t.Fatalf("rank passed %v, want %v", tx.args, want)
	}
	for i := range want {
		if tx.args[i] != want[i] {
			t.Fatalf("rank passed %v, want %v: the exec mode first, then the bound words", tx.args, want)
		}
	}
}

func TestASpentCeilingOnTheRankingReadsAsTooBroad(t *testing.T) {
	tx := &queryingTx{queryErr: queryCanceled()}
	_, err := rank(context.Background(), tx, "SELECT 1", nil)
	if _, ok := errors.AsType[*QueryTooBroadError](err); !ok {
		t.Fatalf("err = %v, want QueryTooBroadError from a ranking stopped by its ceiling", err)
	}
}
