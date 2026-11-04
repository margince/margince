// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

var errSignalsRead = errors.New("signals read refused")

// queryFailsTx is a database that refuses the read of a subject's signals, at
// the query or while its rows come back.
type queryFailsTx struct {
	pgx.Tx
	queryErr error
	rowsErr  error
}

func (tx queryFailsTx) Query(context.Context, string, ...any) (pgx.Rows, error) {
	if tx.queryErr != nil {
		return nil, tx.queryErr
	}
	return failingRows{err: tx.rowsErr}, nil
}

type failingRows struct {
	pgx.Rows
	err error
}

func (failingRows) Next() bool                    { return false }
func (rows failingRows) Err() error               { return rows.err }
func (failingRows) Close()                        {}
func (failingRows) CommandTag() pgconn.CommandTag { return pgconn.CommandTag{} }

// A signal read that fails stops the erasure with the cause named, so the
// erasure never reports a tombstone it did not write.
func TestTombstoneSubjectSignalsNamesTheReadThatFailed(t *testing.T) {
	t.Parallel()

	cases := map[string]queryFailsTx{
		"the query is refused":     {queryErr: errSignalsRead},
		"the rows fail on the way": {rowsErr: errSignalsRead},
	}
	for name, tx := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			err := tombstoneSubjectSignals(t.Context(), tx, ids.ContactID{}, "erasure")
			if !errors.Is(err, errSignalsRead) {
				t.Fatalf("got %v, want it to wrap the refused read", err)
			}
		})
	}
}
