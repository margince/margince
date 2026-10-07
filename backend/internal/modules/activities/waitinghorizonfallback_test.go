// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// What the waiting horizon answers when its measurement is stopped, read
// against a transaction that records what it was asked to run. The property
// against a real server — that the transaction is still usable afterwards — is
// held by waitinghorizonfallback_integration_test.go.

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// scriptedHorizonTx answers the measurement with scan and records every
// statement it executes. execFails names a statement prefix that fails.
type scriptedHorizonTx struct {
	pgx.Tx
	scan      func(dest ...any) error
	execFails string
	executed  []string
}

func (tx *scriptedHorizonTx) Exec(_ context.Context, sql string, _ ...any) (pgconn.CommandTag, error) {
	tx.executed = append(tx.executed, sql)
	if tx.execFails != "" && strings.HasPrefix(sql, tx.execFails) {
		return pgconn.CommandTag{}, errors.New("the connection refused " + tx.execFails)
	}
	return pgconn.CommandTag{}, nil
}

func (tx *scriptedHorizonTx) QueryRow(context.Context, string, ...any) pgx.Row {
	return scriptedRow(tx.scan)
}

type scriptedRow func(dest ...any) error

func (r scriptedRow) Scan(dest ...any) error { return r(dest...) }

func failsWith(code string) func(...any) error {
	return func(...any) error { return &pgconn.PgError{Code: code, Message: "scripted"} }
}

func horizonStore() (*Store, ids.WorkspaceID) {
	ws := ids.New[ids.WorkspaceKind]()
	return NewStore(database.BindTo(nil, ws)).WithClock(func() time.Time { return horizonNoon }), ws
}

func TestATimedOutHorizonMeasurementAnswersTheCompiledHorizonAndIsRememberedOnlyBriefly(t *testing.T) {
	store, ws := horizonStore()
	tx := &scriptedHorizonTx{scan: failsWith("57014")}

	got, err := store.waitingHorizonFor(context.Background(), tx, horizonNoon)
	if err != nil || got != waitingHorizonDays {
		t.Fatalf("a timed-out measurement answered (%d, %v), want the compiled %d", got, err, waitingHorizonDays)
	}
	want := []string{
		"SAVEPOINT waiting_horizon",
		"ROLLBACK TO SAVEPOINT waiting_horizon; RELEASE SAVEPOINT waiting_horizon",
	}
	if strings.Join(tx.executed, "\n") != strings.Join(want, "\n") {
		t.Errorf("the transaction ran %q, want %q — the savepoint is what leaves it usable", tx.executed, want)
	}
	if days, ok := store.horizons.lookup(ws, horizonNoon, horizonNoon); !ok || days != waitingHorizonDays {
		t.Errorf("the compiled horizon was not remembered for the next page: (%d, %v)", days, ok)
	}
	later := horizonNoon.Add(waitingHorizonFallbackTTL)
	if days, ok := store.horizons.lookup(ws, later, later); ok {
		t.Errorf("the compiled horizon was still served as a measurement after %s (%d days)", waitingHorizonFallbackTTL, days)
	}
}

func TestAMeasuredHorizonReleasesItsSavepointAndIsRemembered(t *testing.T) {
	store, ws := horizonStore()
	tx := &scriptedHorizonTx{scan: func(dest ...any) error {
		*dest[0].(*int) = 500
		*dest[1].(*int64) = int64(days(40) / time.Minute)
		return nil
	}}

	got, err := store.waitingHorizonFor(context.Background(), tx, horizonNoon)
	if err != nil || got != 120 {
		t.Fatalf("the measurement answered (%d, %v), want 120 days", got, err)
	}
	if last := tx.executed[len(tx.executed)-1]; last != "RELEASE SAVEPOINT waiting_horizon" {
		t.Errorf("the last statement was %q, want the savepoint released", last)
	}
	if days, ok := store.horizons.lookup(ws, horizonNoon, horizonNoon); !ok || days != 120 {
		t.Errorf("the measured horizon was not remembered: (%d, %v)", days, ok)
	}
}

// A 57014 under a cancelled context is the caller going away, and any other
// fault is not a spent budget: both are returned for the caller to fail on.
func TestAHorizonMeasurementFailsLoudlyWhenItWasNotTheBudget(t *testing.T) {
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	cases := map[string]struct {
		ctx  context.Context
		code string
	}{
		"the caller went away": {ctx: cancelled, code: "57014"},
		"a missing relation":   {ctx: context.Background(), code: "42P01"},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			store, _ := horizonStore()
			tx := &scriptedHorizonTx{scan: failsWith(c.code)}
			got, err := store.waitingHorizonFor(c.ctx, tx, horizonNoon)
			var pgErr *pgconn.PgError
			if !errors.As(err, &pgErr) || pgErr.Code != c.code {
				t.Fatalf("answered (%d, %v), want the %s returned", got, err, c.code)
			}
		})
	}
}

func TestAHorizonSavepointThatCannotBeManagedIsAnError(t *testing.T) {
	cases := map[string]struct {
		scan      func(...any) error
		execFails string
	}{
		"opening":      {scan: failsWith("57014"), execFails: "SAVEPOINT"},
		"rolling back": {scan: failsWith("57014"), execFails: "ROLLBACK TO SAVEPOINT"},
		"releasing": {execFails: "RELEASE SAVEPOINT", scan: func(dest ...any) error {
			*dest[0].(*int) = 500
			*dest[1].(*int64) = 60
			return nil
		}},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			store, _ := horizonStore()
			tx := &scriptedHorizonTx{scan: c.scan, execFails: c.execFails}
			if got, err := store.waitingHorizonFor(context.Background(), tx, horizonNoon); err == nil {
				t.Fatalf("answered %d with no error, want the failed %s reported", got, name)
			}
		})
	}
}
