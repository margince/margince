// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package people

// The 25-link ceiling holds against two writers at once.
//
// Three writers tested `count(*) < 25` and then inserted, and a count read under
// READ COMMITTED is a snapshot: two repairs for two different attendees of one
// meeting each saw 24 and each inserted, and the meeting ended at 26.
// `uq_activity_link` enforces duplicate identity, never a total, so nothing
// caught it — the ceiling was a number every writer agreed on and none of them
// could keep.
//
// Two transactions, staged rather than raced: what is under test is that the
// second one cannot land its row, and a race would prove that only on the runs
// where it happened to lose.

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// seedActivityAtTheCeiling writes one activity already filed under `filed`
// organizations — the shape a busy meeting reaches, and the one every count
// guard is about.
func seedActivityAtTheCeiling(ctx context.Context, t *testing.T, e *dedupeEnv, filed int) ids.ActivityID {
	t.Helper()
	activityID := ids.New[ids.ActivityKind]()
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `
			INSERT INTO activity (id, kind, subject, occurred_at, source_system, source_id,
			                      source, captured_by)
			VALUES ($1, 'note', 'Filed everywhere', now(), 'gmail', $2, 'gmail:seed', 'connector:gmail')`,
			activityID, activityID.String()); err != nil {
			return err
		}
		for i := range filed {
			orgID := ids.NewV7()
			if _, err := tx.Exec(ctx, `
				INSERT INTO organization (id, display_name, source, captured_by)
				VALUES ($1, $2, 'gmail:seed', 'connector:gmail')`,
				orgID, "Filed Co "+orgID.String()[:8]); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO activity_link (activity_id, entity_type, organization_id)
				VALUES ($1, 'organization', $2)`, activityID, orgID); err != nil {
				return err
			}
			_ = i
		}
		return nil
	}); err != nil {
		t.Fatalf("seeding an activity filed under %d records: %v", filed, err)
	}
	return activityID
}

// linkAnOrganization files the activity under one more organization, in its own
// transaction, and answers what the database said.
func linkAnOrganization(ctx context.Context, e *dedupeEnv, activityID ids.ActivityID) error {
	return e.store.tx(ctx, func(tx pgx.Tx) error {
		orgID := ids.NewV7()
		if _, err := tx.Exec(ctx, `
			INSERT INTO organization (id, display_name, source, captured_by)
			VALUES ($1, 'One More', 'gmail:seed', 'connector:gmail')`, orgID); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO activity_link (activity_id, entity_type, organization_id)
			VALUES ($1, 'organization', $2)`, activityID, orgID)
		return err
	})
}

func TestTwoWritersCannotPushOneActivityPastTheCeiling(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	// One short of the ceiling: both writers below would see room.
	activityID := seedActivityAtTheCeiling(ctx, t, e, 24)

	// The first writer holds its transaction open with the row inserted but not
	// committed — the exact window the count read used to be answered in.
	held, release := make(chan struct{}), make(chan struct{})
	holding := make(chan error, 1)
	var announce sync.Once
	go func() {
		holding <- e.store.tx(ctx, func(tx pgx.Tx) error {
			orgID := ids.NewV7()
			if _, err := tx.Exec(ctx, `
				INSERT INTO organization (id, display_name, source, captured_by)
				VALUES ($1, 'The 25th', 'gmail:seed', 'connector:gmail')`, orgID); err != nil {
				return err
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO activity_link (activity_id, entity_type, organization_id)
				VALUES ($1, 'organization', $2)`, activityID, orgID); err != nil {
				return err
			}
			announce.Do(func() { close(held) })
			<-release
			return nil
		})
	}()
	select {
	case <-held:
	case err := <-holding:
		t.Fatalf("the twenty-fifth link failed, so this proves nothing about the twenty-sixth: %v", err)
	}

	// THE WINDOW ITSELF. A second writer arriving now cannot see the uncommitted
	// row, so a count is the answer from before it — and the lock is what stops
	// the count being taken at all. Its own lock_timeout reports the blocking,
	// and it can only fire while somebody holds the row: no sleep, no clock.
	//
	// This is the assertion the ceiling rests on. The refusal below would pass
	// on a shared lock too, because by then the first writer has committed and
	// the count is honest.
	blocked := e.store.tx(ctx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SET LOCAL lock_timeout = '2s'`); err != nil {
			return err
		}
		orgID := ids.NewV7()
		if _, err := tx.Exec(ctx, `
			INSERT INTO organization (id, display_name, source, captured_by)
			VALUES ($1, 'The 26th', 'gmail:seed', 'connector:gmail')`, orgID); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO activity_link (activity_id, entity_type, organization_id)
			VALUES ($1, 'organization', $2)`, activityID, orgID)
		return err
	})
	var pgErr *pgconn.PgError
	if !errors.As(blocked, &pgErr) || pgErr.Code != "55P03" {
		t.Fatalf("a second writer got %v while the twenty-fifth link was uncommitted, want a lock "+
			"timeout — an unserialized insert reads a count from before that row and files a "+
			"twenty-sixth", blocked)
	}

	close(release)
	if err := <-holding; err != nil {
		t.Fatalf("the first writer failed: %v", err)
	}

	// And once it can see the row, it is refused rather than admitted.
	err := linkAnOrganization(ctx, e, activityID)
	if !errors.As(err, &pgErr) || pgErr.Code != "23514" {
		t.Fatalf("the twenty-sixth link got %v, want a check violation", err)
	}
	if got := linkCount(ctx, t, e, activityID); got != 25 {
		t.Errorf("the activity is filed under %d records, want the ceiling", got)
	}
}

// And a writer with room still writes: without this the case above would pass
// on a trigger that refused everything.
func TestAnActivityUnderTheCeilingStillTakesALink(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	activityID := seedActivityAtTheCeiling(ctx, t, e, 3)

	if err := linkAnOrganization(ctx, e, activityID); err != nil {
		t.Fatalf("filing an activity with room under one more record: %v", err)
	}
	if got := linkCount(ctx, t, e, activityID); got != 4 {
		t.Errorf("the activity is filed under %d records, want 4", got)
	}
}

func linkCount(ctx context.Context, t *testing.T, e *dedupeEnv, activityID ids.ActivityID) int {
	t.Helper()
	var filed int
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`SELECT count(*) FROM activity_link WHERE activity_id = $1`, activityID).Scan(&filed)
	}); err != nil {
		t.Fatalf("counting the activity's links: %v", err)
	}
	return filed
}
