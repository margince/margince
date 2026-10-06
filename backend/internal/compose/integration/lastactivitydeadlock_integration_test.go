// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// Concurrent writers that reach the same last-activity clocks queue rather
// than deadlock (#6282).
//
// Each case is deterministic: a barrier transaction holds a row both writers
// need, so both are parked mid-write before it lets go, and the interleaving
// that used to cycle is the only one left. The writes go through the Tx doors
// on a transaction the test opens, so the store-level retry cannot hide a
// cycle: what is asserted is the lock order, not that a second attempt won.

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The issue's own example: each activity names one company directly and one
// contact employed at the OTHER company, so the direct lock and the one reached
// through the employer land in opposite orders unless the whole set is taken
// up front.
func TestActivitiesCrossingAnEmployerQueueRatherThanDeadlock(t *testing.T) {
	e := Setup(t)
	first, second := e.SeedCompany(t, "Cross One", nil), e.SeedCompany(t, "Cross Two", nil)
	atFirst, atSecond := e.SeedContact(t, "Works At One", nil), e.SeedContact(t, "Works At Two", nil)
	employForLockOrder(t, e, atFirst, first)
	employForLockOrder(t, e, atSecond, second)

	barrier := holdRows(t, `SELECT 1 FROM contact WHERE id = ANY($1) FOR UPDATE`, []ids.UUID{atFirst, atSecond})
	errs := runConcurrently(t, e, barrier,
		func(ctx context.Context, tx pgx.Tx) error {
			return logNoteTx(ctx, e, tx, noteLink("company", second), noteLink("contact", atFirst))
		},
		func(ctx context.Context, tx pgx.Tx) error {
			return logNoteTx(ctx, e, tx, noteLink("company", first), noteLink("contact", atSecond))
		},
	)
	assertNoLockCycle(t, errs)
}

// Two activities on one company: the attach probe's FOR SHARE is held by both
// before either reaches the trigger's FOR UPDATE, and neither upgrade can wait
// out the other's share.
func TestActivitiesOnOneCompanyQueueRatherThanDeadlock(t *testing.T) {
	e := Setup(t)
	company := e.SeedCompany(t, "Hot Account", nil)

	barrier := holdRows(t, `SELECT 1 FROM company WHERE id = ANY($1) FOR SHARE`, []ids.UUID{company})
	note := func(ctx context.Context, tx pgx.Tx) error {
		return logNoteTx(ctx, e, tx, noteLink("company", company))
	}
	assertNoLockCycle(t, runConcurrently(t, e, barrier, note, note))
}

// The employment half of the import: two contacts starting at one company.
// Each writer shares the company in its endpoint probe and then needs it FOR
// UPDATE in the relationship trigger — the same upgrade, on the other path.
func TestEmploymentsAtOneCompanyQueueRatherThanDeadlock(t *testing.T) {
	e := Setup(t)
	company := e.SeedCompany(t, "Hiring Account", nil)
	joiners := []ids.UUID{e.SeedContact(t, "Joiner One", nil), e.SeedContact(t, "Joiner Two", nil)}

	barrier := holdRows(t, `SELECT 1 FROM company WHERE id = ANY($1) FOR SHARE`, []ids.UUID{company})
	hire := func(contact ids.UUID) func(context.Context, pgx.Tx) error {
		return func(ctx context.Context, tx pgx.Tx) error {
			contactID, companyID := ids.From[ids.ContactKind](contact), companyIDOf(company)
			_, err := e.Contacts.CreateRelationshipTx(ctx, tx, contacts.CreateRelationshipInput{
				Kind: "employment", ContactID: &contactID, CompanyID: &companyID, Source: "manual",
			})
			return err
		}
	}
	assertNoLockCycle(t, runConcurrently(t, e, barrier, hire(joiners[0]), hire(joiners[1])))
}

func noteLink(entityType string, id ids.UUID) activities.ActivityLinkInput {
	return activities.ActivityLinkInput{EntityType: entityType, EntityID: id}
}

func employForLockOrder(t *testing.T, e *Env, contact, company ids.UUID) {
	t.Helper()
	contactID, companyID := ids.From[ids.ContactKind](contact), companyIDOf(company)
	if _, err := e.Contacts.CreateRelationship(e.Admin(), contacts.CreateRelationshipInput{
		Kind: "employment", ContactID: &contactID, CompanyID: &companyID, IsCurrentPrimary: BoolPtr(true), Source: "manual",
	}); err != nil {
		t.Fatalf("employing the seed contact: %v", err)
	}
}

func logNoteTx(ctx context.Context, e *Env, tx pgx.Tx, links ...activities.ActivityLinkInput) error {
	subject := "crossed"
	_, _, err := e.Activities.LogActivityTx(ctx, tx, activities.LogActivityInput{
		Kind: "note", Subject: &subject, Source: "manual", Links: links,
	})
	return err
}

// holdRows opens the barrier: an owner transaction holding rows every writer needs.
func holdRows(t *testing.T, lockSQL string, rows []ids.UUID) pgx.Tx {
	t.Helper()
	conn := OwnerConn(t)
	tx, err := conn.Begin(context.Background())
	if err != nil {
		t.Fatalf("opening the barrier: %v", err)
	}
	if _, err := tx.Exec(context.Background(), lockSQL, rows); err != nil {
		t.Fatalf("taking the barrier's locks: %v", err)
	}
	return tx
}

// runConcurrently starts every writer in its own transaction, waits until all
// of them are parked on a row lock, releases the barrier and returns each
// writer's outcome, commit included.
func runConcurrently(t *testing.T, e *Env, barrier pgx.Tx, writers ...func(context.Context, pgx.Tx) error) []error {
	t.Helper()
	errs := make([]error, len(writers))
	var wg sync.WaitGroup
	for i, write := range writers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs[i] = runWriter(e, write)
		}()
	}
	waitForParkedWriters(t, len(writers))
	if err := barrier.Rollback(context.Background()); err != nil {
		t.Fatalf("releasing the barrier: %v", err)
	}
	wg.Wait()
	return errs
}

func runWriter(e *Env, write func(context.Context, pgx.Tx) error) error {
	ctx := e.Admin()
	tx, err := e.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	//craft:ignore swallowed-errors rollback after commit is a designed no-op; on the error path the write's error supersedes it
	defer func() { _ = tx.Rollback(context.Background()) }()
	if err := write(ctx, tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// waitForParkedWriters polls until n backends in this database wait on a lock.
func waitForParkedWriters(t *testing.T, n int) {
	t.Helper()
	probe := OwnerConn(t)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	for {
		if ctx.Err() != nil {
			t.Fatalf("fewer than %d writers ever parked behind the barrier, so the interleaving under test never happened", n)
		}
		if _, err := probe.Exec(ctx, `SELECT pg_stat_clear_snapshot()`); err != nil {
			t.Fatalf("clearing the stats snapshot: %v", err)
		}
		var parked int
		if err := probe.QueryRow(ctx, `
			SELECT count(*)::int FROM pg_stat_activity
			 WHERE datname = current_database() AND cardinality(pg_blocking_pids(pid)) > 0`).Scan(&parked); err != nil {
			t.Fatalf("counting parked writers: %v", err)
		}
		if parked >= n {
			return
		}
	}
}

func assertNoLockCycle(t *testing.T, errs []error) {
	t.Helper()
	for i, err := range errs {
		if storekit.IsDeadlock(err) {
			t.Errorf("writer %d was aborted as a deadlock victim: the writers took the rows they share in "+
				"different orders — %v", i, err)
		} else if err != nil {
			t.Errorf("writer %d: %v", i, err)
		}
	}
}
