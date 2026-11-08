// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package initiatives

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// memberStoreTx answers the read of the stored rows, and counts every delete.
type memberStoreTx struct {
	pgx.Tx
	queryErr, rowsErr, execErr error
	deletes                    int
}

func (tx *memberStoreTx) Query(context.Context, string, ...any) (pgx.Rows, error) {
	if tx.queryErr != nil {
		return nil, tx.queryErr
	}
	return storedRows{err: tx.rowsErr}, nil
}

func (tx *memberStoreTx) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	tx.deletes++
	return pgconn.CommandTag{}, tx.execErr
}

// storedRows is a result with no rows, ending on err.
type storedRows struct {
	pgx.Rows
	err error
}

func (storedRows) Close()                        {}
func (storedRows) Next() bool                    { return false }
func (r storedRows) Err() error                  { return r.err }
func (storedRows) CommandTag() pgconn.CommandTag { return pgconn.CommandTag{} }

// A save replaces only what it can read. One that could not read the stored
// rows must delete nothing, or a failed read would become an emptied initiative.
func TestASaveThatCannotReadTheStoredRowsDeletesNothing(t *testing.T) {
	down := errors.New("the database went away")
	ctx := principal.WithActor(context.Background(), principal.Principal{Type: principal.PrincipalSystem, ID: "system:test"})
	for name, tx := range map[string]*memberStoreTx{
		"the query failed":     {queryErr: down},
		"the rows broke early": {rowsErr: down},
	} {
		if err := clearReadableMembers(ctx, tx, ids.NewV7()); !errors.Is(err, down) {
			t.Errorf("%s: answered %v, want the failure", name, err)
		}
		if tx.deletes != 0 {
			t.Errorf("%s: deleted %d times after a failed read", name, tx.deletes)
		}
	}
}

// A delete that failed fails the save, so its picks are not reported replaced.
func TestAFailedClearFailsTheSave(t *testing.T) {
	down := errors.New("the database went away")
	ctx := principal.WithActor(context.Background(), principal.Principal{Type: principal.PrincipalSystem, ID: "system:test"})
	if err := clearReadableMembers(ctx, &memberStoreTx{execErr: down}, ids.NewV7()); !errors.Is(err, down) {
		t.Errorf("a failed delete answered %v, want the failure", err)
	}
}
