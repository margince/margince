// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package search

// The ranking forces custom plans and gives the transaction back as it found
// it. A joined snapshot outlives the search, so every way out of the ranking
// either restores the caller's mode or fails the read; none leaves it forced.

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// scriptedTx answers the statements a ranking issues. The embedded interface is
// nil deliberately: any other call is one this helper was never asked to stand
// in for, and a panic says so louder than a zero value.
type scriptedTx struct {
	pgx.Tx
	mode     string
	readErr  error
	execErrs []error
	queryErr error
	issued   []string
}

type modeRow struct {
	mode string
	err  error
}

func (r modeRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	target, ok := dest[0].(*string)
	if !ok {
		return fmt.Errorf("scanning the plan cache mode into %T", dest[0])
	}
	*target = r.mode
	return nil
}

func (s *scriptedTx) QueryRow(_ context.Context, sql string, _ ...any) pgx.Row {
	s.issued = append(s.issued, sql)
	return modeRow{mode: s.mode, err: s.readErr}
}

func (s *scriptedTx) Exec(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	s.issued = append(s.issued, fmt.Sprintf("%s %v", sql, args))
	if len(s.execErrs) == 0 {
		return pgconn.CommandTag{}, nil
	}
	err := s.execErrs[0]
	s.execErrs = s.execErrs[1:]
	return pgconn.CommandTag{}, err
}

func (s *scriptedTx) Query(_ context.Context, sql string, _ ...any) (pgx.Rows, error) {
	s.issued = append(s.issued, sql)
	return nil, s.queryErr
}

const (
	readMode    = `SELECT current_setting('plan_cache_mode')`
	forceCustom = `SELECT set_config('plan_cache_mode', 'force_custom_plan', true) []`
)

func TestTheRankingRestoresThePlanModeItFound(t *testing.T) {
	tx := &scriptedTx{mode: "force_generic_plan"}
	ran := false
	if err := withCustomPlans(context.Background(), tx, func() error { ran = true; return nil }); err != nil {
		t.Fatal(err)
	}
	want := []string{readMode, forceCustom, `SELECT set_config('plan_cache_mode', $1, true) [force_generic_plan]`}
	if !ran || fmt.Sprint(tx.issued) != fmt.Sprint(want) {
		t.Fatalf("ran=%v, issued %q; want the ranking run between forcing and restoring %q", ran, tx.issued, want)
	}
}

func TestAPlanModeThatCannotBeReadRunsNoRanking(t *testing.T) {
	refused := errors.New("connection lost")
	tx := &scriptedTx{readErr: refused}
	err := withCustomPlans(context.Background(), tx, func() error {
		t.Fatal("the ranking ran without knowing which mode to restore")
		return nil
	})
	if !errors.Is(err, refused) || len(tx.issued) != 1 {
		t.Fatalf("err = %v, issued %q; want the read's own failure and nothing after it", err, tx.issued)
	}
}

func TestARefusedPlanModeRunsNoRanking(t *testing.T) {
	refused := errors.New("permission denied to set parameter")
	tx := &scriptedTx{mode: "auto", execErrs: []error{refused}}
	err := withCustomPlans(context.Background(), tx, func() error {
		t.Fatal("the ranking ran on a plan mode that was never set")
		return nil
	})
	if !errors.Is(err, refused) {
		t.Fatalf("err = %v, want the refusal to set the mode", err)
	}
}

// A failed statement has aborted the transaction, so a restore could only fail
// too; the ranking's own error is the one the caller must see.
func TestAFailedRankingIsReturnedWithoutARestore(t *testing.T) {
	failed := errors.New("ranking failed")
	tx := &scriptedTx{mode: "auto"}
	err := withCustomPlans(context.Background(), tx, func() error { return failed })
	if !errors.Is(err, failed) || len(tx.issued) != 2 {
		t.Fatalf("err = %v, issued %q; want the ranking's error and no restore", err, tx.issued)
	}
}

func TestARefusedRestoreFailsTheRead(t *testing.T) {
	refused := errors.New("restore refused")
	tx := &scriptedTx{mode: "auto", execErrs: []error{nil, refused}}
	err := withCustomPlans(context.Background(), tx, func() error { return nil })
	if !errors.Is(err, refused) {
		t.Fatalf("err = %v, want the refused restore: a mode left forced must not pass silently", err)
	}
}

func TestASpentCeilingOnTheRankingReadsAsTooBroad(t *testing.T) {
	tx := &scriptedTx{mode: "auto", queryErr: queryCanceled()}
	_, err := rank(context.Background(), tx, "SELECT 1", nil)
	var tooBroad *QueryTooBroadError
	if !errors.As(err, &tooBroad) {
		t.Fatalf("err = %v, want QueryTooBroadError from a ranking stopped by its ceiling", err)
	}
}
