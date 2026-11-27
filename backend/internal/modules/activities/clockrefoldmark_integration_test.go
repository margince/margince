// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// refoldMarkTx opens a transaction that rolls back, and the system context the
// mark's readers and writers ask for.
func refoldMarkTx(t *testing.T) (context.Context, pgx.Tx) {
	t.Helper()
	e := setupPromises(t)
	ctx := principal.SystemActing(context.Background(), "system:clock-refold-test")
	tx, err := e.owner.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := tx.Rollback(context.Background()); err != nil {
			t.Errorf("rolling back the mark: %v", err)
		}
	})
	return ctx, tx
}

func storedRefoldMark(ctx context.Context, t *testing.T, tx pgx.Tx) ClockRefoldMark {
	t.Helper()
	mark, _, err := ClockRefoldStartTx(ctx, tx)
	if err != nil {
		t.Fatalf("reading the mark: %v", err)
	}
	return mark
}

func TestARefoldWithNoMarkStartsWhenTheClockMigrationRan(t *testing.T) {
	t.Parallel()
	ctx, tx := refoldMarkTx(t)
	var migrated time.Time
	if err := tx.QueryRow(ctx, `SELECT applied_at FROM schema_migrations_core WHERE version = $1`,
		clockRefoldMigration).Scan(&migrated); err != nil {
		t.Fatalf("reading when the clock migration ran: %v", err)
	}

	mark := storedRefoldMark(ctx, t, tx)
	if !mark.At.Equal(migrated) || !mark.ID.IsZero() {
		t.Fatalf("a run with no mark starts at (%s, %s), want (%s, the zero id)", mark.At, mark.ID, migrated)
	}
}

func TestTheClockRefoldMarkAdvancesAndNeverMovesBack(t *testing.T) {
	t.Parallel()
	ctx, tx := refoldMarkTx(t)
	instant := time.Date(2026, 5, 4, 9, 30, 0, 123456000, time.UTC)
	low := ids.MustParse("00000000-0000-7000-8000-000000000001")
	high := ids.MustParse("00000000-0000-7000-8000-000000000002")
	reached := ClockRefoldMark{At: instant, ID: high}

	steps := []struct {
		name     string
		to, want ClockRefoldMark
	}{
		{"the first advance is stored", ClockRefoldMark{At: instant, ID: low}, ClockRefoldMark{At: instant, ID: low}},
		{"a later id at one instant advances", reached, reached},
		{"an earlier id at one instant is refused", ClockRefoldMark{At: instant, ID: low}, reached},
		{"an earlier instant is refused", ClockRefoldMark{At: instant.Add(-time.Hour), ID: clockRefoldEnd}, reached},
		{"a later instant advances", ClockRefoldMark{At: instant.Add(time.Hour), ID: low}, ClockRefoldMark{At: instant.Add(time.Hour), ID: low}},
	}
	for _, step := range steps {
		if err := AdvanceClockRefoldMarkTx(ctx, tx, step.to); err != nil {
			t.Fatalf("%s: %v", step.name, err)
		}
		got := storedRefoldMark(ctx, t, tx)
		if !got.At.Equal(step.want.At) || got.ID != step.want.ID {
			t.Errorf("%s: the mark reads (%s, %s), want (%s, %s)", step.name, got.At, got.ID, step.want.At, step.want.ID)
		}
	}
}
