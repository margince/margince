// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
)

// recordingTx stands in for the database boundary: it records what happened to
// the savepoint and fails the step a case names.
type recordingTx struct {
	pgx.Tx
	beginErr, commitErr, rollbackErr error
	committed, rolledBack            bool
}

func (r *recordingTx) Begin(context.Context) (pgx.Tx, error) {
	if r.beginErr != nil {
		return nil, r.beginErr
	}
	return r, nil
}

func (r *recordingTx) Commit(context.Context) error {
	r.committed = true
	return r.commitErr
}

func (r *recordingTx) Rollback(context.Context) error {
	r.rolledBack = true
	return r.rollbackErr
}

func TestASavepointIsKeptOnlyWhenTheWorkSaysSo(t *testing.T) {
	boom, stuck := errors.New("work failed"), errors.New("rollback failed")
	for name, tc := range map[string]struct {
		tx         *recordingTx
		keep       bool
		workErr    error
		wantKept   bool
		wantErr    error
		wantCommit bool
		wantRoll   bool
	}{
		"kept":                         {tx: &recordingTx{}, keep: true, wantKept: true, wantCommit: true},
		"withdrawn":                    {tx: &recordingTx{}, wantRoll: true},
		"failed work is rolled back":   {tx: &recordingTx{}, workErr: boom, wantErr: boom, wantRoll: true},
		"a rollback that fails speaks": {tx: &recordingTx{rollbackErr: stuck}, workErr: boom, wantErr: stuck, wantRoll: true},
		"a withdrawal that fails":      {tx: &recordingTx{rollbackErr: stuck}, wantErr: stuck, wantRoll: true},
		"a release that fails":         {tx: &recordingTx{commitErr: stuck}, keep: true, wantErr: stuck, wantCommit: true},
		"a savepoint that cannot open": {tx: &recordingTx{beginErr: stuck}, keep: true, wantErr: stuck},
	} {
		t.Run(name, func(t *testing.T) {
			kept, err := Savepoint(t.Context(), tc.tx, func(pgx.Tx) (bool, error) { return tc.keep, tc.workErr })
			if kept != tc.wantKept {
				t.Errorf("kept = %t, want %t", kept, tc.wantKept)
			}
			if tc.wantErr == nil && err != nil {
				t.Errorf("err = %v, want none", err)
			}
			if tc.wantErr != nil && !errors.Is(err, tc.wantErr) {
				t.Errorf("err = %v, want it to wrap %v", err, tc.wantErr)
			}
			if tc.tx.committed != tc.wantCommit || tc.tx.rolledBack != tc.wantRoll {
				t.Errorf("committed %t, rolled back %t; want %t and %t",
					tc.tx.committed, tc.tx.rolledBack, tc.wantCommit, tc.wantRoll)
			}
		})
	}
}
