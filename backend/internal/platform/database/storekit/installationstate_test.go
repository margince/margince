// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// stateTx answers the three statements the installation state code sends, from
// a map that stands for the table. The SQL itself is run by the module tests.
type stateTx struct {
	fakeTx
	rows    map[string][]byte
	queries []string
}

func (s *stateTx) Exec(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	if s.execErr != nil {
		return pgconn.CommandTag{}, s.execErr
	}
	key, keyed := args[0].(string)
	value, encoded := args[1].(string)
	if !keyed || !encoded {
		return pgconn.CommandTag{}, errors.New("stateTx: the key and the value are sent as text")
	}
	if _, stored := s.rows[key]; !stored || strings.Contains(sql, "DO UPDATE") {
		s.rows[key] = []byte(value)
	}
	return pgconn.NewCommandTag("INSERT 0 1"), nil
}

func (s *stateTx) QueryRow(_ context.Context, sql string, args ...any) pgx.Row {
	s.queries = append(s.queries, sql)
	key, keyed := args[0].(string)
	raw, stored := s.rows[key]
	return stateRow{raw: raw, found: keyed && stored}
}

type stateRow struct {
	raw   []byte
	found bool
}

func (r stateRow) Scan(dest ...any) error {
	if !r.found {
		return pgx.ErrNoRows
	}
	target, ok := dest[0].(*[]byte)
	if !ok {
		return errors.New("stateRow: the value is scanned as bytes")
	}
	*target = r.raw
	return nil
}

type sweepPlace struct {
	After *ids.UUID `json:"after"`
	Round int       `json:"round"`
}

const stateUnderTest = InstallationStateKey("a_pass_under_test")

func TestAStateNobodyStoredReadsAsNotFound(t *testing.T) {
	tx := &stateTx{rows: map[string][]byte{}}
	place, found, err := InstallationStateTx[sweepPlace](context.Background(), tx, stateUnderTest)
	if err != nil || found || place != (sweepPlace{}) {
		t.Fatalf("an absent state read as (%+v, %t, %v), want the zero shape and not found", place, found, err)
	}
}

func TestAStoredStateReadsBackInItsWritersShape(t *testing.T) {
	tx := &stateTx{rows: map[string][]byte{}}
	ctx := context.Background()
	after := ids.MustParse("00000000-0000-7000-8000-00000000000a")
	for _, stored := range []sweepPlace{{After: &after, Round: 2}, {Round: 3}} {
		if err := StoreInstallationStateTx(ctx, tx, stateUnderTest, stored); err != nil {
			t.Fatalf("storing %+v: %v", stored, err)
		}
		got, found, err := InstallationStateTx[sweepPlace](ctx, tx, stateUnderTest)
		if err != nil || !found {
			t.Fatalf("reading back %+v: found=%t, %v", stored, found, err)
		}
		if got.Round != stored.Round || (got.After == nil) != (stored.After == nil) ||
			(got.After != nil && *got.After != *stored.After) {
			t.Errorf("stored %+v and read %+v", stored, got)
		}
	}
}

func TestLockingAStateStoresTheInitialShapeOnlyWhenNoneIsStored(t *testing.T) {
	tx := &stateTx{rows: map[string][]byte{}}
	ctx := context.Background()

	first, err := LockInstallationStateTx(ctx, tx, stateUnderTest, sweepPlace{Round: 1})
	if err != nil || first.Round != 1 {
		t.Fatalf("the first lock answered (%+v, %v), want the initial round 1", first, err)
	}
	second, err := LockInstallationStateTx(ctx, tx, stateUnderTest, sweepPlace{Round: 9})
	if err != nil || second.Round != 1 {
		t.Fatalf("a lock over a stored state answered (%+v, %v), want the stored round 1", second, err)
	}
	for _, query := range tx.queries {
		if !strings.Contains(query, "FOR UPDATE") {
			t.Errorf("a locking read did not take the row lock: %s", query)
		}
	}
}

func TestAStateThatDoesNotFitItsShapeIsAnErrorNotAZeroShape(t *testing.T) {
	tx := &stateTx{rows: map[string][]byte{string(stateUnderTest): []byte(`{"round":"two"}`)}}
	if _, found, err := InstallationStateTx[sweepPlace](context.Background(), tx, stateUnderTest); err == nil || found {
		t.Fatalf("a state of another shape read as found=%t with no error", found)
	}
}

func TestAFailedStateWriteIsReported(t *testing.T) {
	refused := errors.New("the connection is gone")
	tx := &stateTx{rows: map[string][]byte{}}
	tx.execErr = refused
	ctx := context.Background()
	if err := StoreInstallationStateTx(ctx, tx, stateUnderTest, sweepPlace{}); !errors.Is(err, refused) {
		t.Errorf("a failed store answered %v, want the cause", err)
	}
	if _, err := LockInstallationStateTx(ctx, tx, stateUnderTest, sweepPlace{}); !errors.Is(err, refused) {
		t.Errorf("a failed lock answered %v, want the cause", err)
	}
}
