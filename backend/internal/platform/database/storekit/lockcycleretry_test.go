// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestALockCycleVictimIsRunAgainUntilItCommits(t *testing.T) {
	for _, code := range []string{"40P01", "40001"} {
		t.Run(code, func(t *testing.T) {
			runs := 0
			err := RetryLockCycles(context.Background(), func() error {
				runs++
				if runs < 3 {
					return fmt.Errorf("logging: %w", &pgconn.PgError{Code: code})
				}
				return nil
			})
			if err != nil || runs != 3 {
				t.Fatalf("got %v after %d runs, want success on the third", err, runs)
			}
		})
	}
}

func TestTheLastLockCycleIsReturnedOnceTheAttemptsAreSpent(t *testing.T) {
	runs := 0
	err := RetryLockCycles(context.Background(), func() error {
		runs++
		return &pgconn.PgError{Code: "40P01"}
	})
	if !IsLockCycle(err) || runs != 3 {
		t.Fatalf("got %v after %d runs, want the deadlock after exactly 3", err, runs)
	}
}

// Anything else is the write's own answer, and running it again would repeat
// a refusal or double a success.
func TestAnyOtherOutcomeIsNotRetried(t *testing.T) {
	for _, outcome := range []error{nil, errors.New("refused"), &pgconn.PgError{Code: "23505"}} {
		runs := 0
		got := RetryLockCycles(context.Background(), func() error {
			runs++
			return outcome
		})
		if runs != 1 || !errors.Is(got, outcome) {
			t.Errorf("outcome %v: ran %d times and returned %v, want one run returning it", outcome, runs, got)
		}
	}
}

func TestACancelledCallerStopsWaitingForTheNextAttempt(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	runs := 0
	err := RetryLockCycles(ctx, func() error {
		runs++
		cancel()
		return &pgconn.PgError{Code: "40P01"}
	})
	if runs != 1 || !IsLockCycle(err) {
		t.Fatalf("ran %d times and returned %v, want one run and its deadlock", runs, err)
	}
}
