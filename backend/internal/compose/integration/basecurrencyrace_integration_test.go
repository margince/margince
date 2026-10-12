// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// ADR-0085 §7's freeze holds ACROSS transactions, not only inside one.
//
// The guard counts frozen rates inside the settings write's own transaction,
// which makes it correct against anything already committed and blind to a
// freeze committing beside it. Under READ COMMITTED neither statement touched a
// row the other locked, so a re-base reading "nothing is frozen" and a close
// stamping a rate could both succeed — leaving a deal whose frozen rate
// converts into a base the installation no longer uses, silently, and with
// nothing able to detect it afterwards because the count is non-zero for the
// NEW base too.
//
// What closes it is that the two now contend for one row: a caller deciding
// against the base holds it FOR SHARE, and the settings write takes it FOR
// UPDATE before running its guard. These need a real database — the conflict is
// the database's, and a unit test with one connection cannot have two
// transactions.

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/settings"
)

const baseCurrencyKey = "installation.base_currency"

// A caller deciding against the base blocks a write that would move it.
//
// Asserted by TIMEOUT rather than by racing the two and hoping: a statement
// that has to wait is the whole mechanism, so the test asks the writer to give
// up after a moment and treats "it gave up" as the conflict being real. The
// control below then shows the same statement succeeding once the reader is
// done, so the timeout is not simply a slow database.
func TestAWriteOfTheBaseWaitsForACallerDecidingAgainstIt(t *testing.T) {
	e := Setup(t)
	ctx, admin := context.Background(), e.Admin()
	reader, writer := OwnerConn(t), OwnerConn(t)

	deciding, err := reader.Begin(ctx)
	if err != nil {
		t.Fatalf("opening the deciding transaction: %v", err)
	}
	//craft:ignore swallowed-errors the deciding transaction is read-only and is rolled back explicitly below; this defer only covers an early t.Fatal
	defer func() { _ = deciding.Rollback(ctx) }()
	// THE REAL READ. Driving the SQL by hand here would assert what Postgres
	// does with FOR SHARE, which is not in doubt — what is in doubt is whether
	// the seam every freeze resolves the base through takes it.
	if _, err := identity.BaseCurrencyOf(admin, deciding); err != nil {
		t.Fatalf("resolving the base to decide against: %v", err)
	}

	rebase, err := writer.Begin(ctx)
	if err != nil {
		t.Fatalf("opening the re-base transaction: %v", err)
	}
	//craft:ignore swallowed-errors a rollback after the explicit one below is a designed no-op
	defer func() { _ = rebase.Rollback(ctx) }()
	if _, err := rebase.Exec(ctx, `SET LOCAL statement_timeout = '1500ms'`); err != nil {
		t.Fatalf("bounding the re-base's wait: %v", err)
	}
	// And the real write-side lock, for the same reason.
	err = settings.LockForWrite(admin, rebase, baseCurrencyKey)
	if err == nil {
		t.Fatal("the re-base took the base-currency row while another transaction was deciding " +
			"against it — the guard it is about to run can then read a freeze that has not " +
			"happened yet, and both commit")
	}
	if err := rebase.Rollback(ctx); err != nil {
		t.Fatalf("releasing the refused re-base: %v", err)
	}

	// The control: the same statement succeeds once nobody is deciding, so what
	// the timeout above measured was the conflict and not an unreachable row.
	if err := deciding.Rollback(ctx); err != nil {
		t.Fatalf("releasing the deciding transaction: %v", err)
	}
	free, err := writer.Begin(ctx)
	if err != nil {
		t.Fatalf("opening the control transaction: %v", err)
	}
	//craft:ignore swallowed-errors the control transaction takes a lock and asserts nothing about releasing it
	defer func() { _ = free.Rollback(ctx) }()
	done := make(chan error, 1)
	go func() { done <- settings.LockForWrite(admin, free, baseCurrencyKey) }()
	select {
	case execErr := <-done:
		if execErr != nil {
			t.Fatalf("the row could not be taken with nobody holding it: %v", execErr)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("taking the row with nobody holding it did not finish, so the timeout above " +
			"proves nothing about the conflict")
	}
}

// And the lock is on the ROW, not the whole table: a write to another setting
// runs beside a caller deciding against the base.
//
// Worth holding, because the cheap way to close this race is a lock with too
// wide a reach — one that would make every settings write wait on every read of
// any of them.
func TestDecidingAgainstTheBaseDoesNotBlockAnotherSetting(t *testing.T) {
	e := Setup(t)
	ctx, admin := context.Background(), e.Admin()
	reader, writer := OwnerConn(t), OwnerConn(t)

	deciding, err := reader.Begin(ctx)
	if err != nil {
		t.Fatalf("opening the deciding transaction: %v", err)
	}
	//craft:ignore swallowed-errors the deciding transaction is read-only and is rolled back explicitly below; this defer only covers an early t.Fatal
	defer func() { _ = deciding.Rollback(ctx) }()
	if _, err := identity.BaseCurrencyOf(admin, deciding); err != nil {
		t.Fatalf("resolving the base to decide against: %v", err)
	}

	other, err := writer.Begin(ctx)
	if err != nil {
		t.Fatalf("opening the other-setting transaction: %v", err)
	}
	//craft:ignore swallowed-errors the transaction takes a lock and asserts nothing about releasing it
	defer func() { _ = other.Rollback(ctx) }()
	if _, err := other.Exec(ctx, `SET LOCAL statement_timeout = '3s'`); err != nil {
		t.Fatalf("bounding the other write's wait: %v", err)
	}
	var key string
	err = other.QueryRow(ctx,
		`SELECT key FROM setting WHERE key <> $1 LIMIT 1`, baseCurrencyKey).Scan(&key)
	if err == pgx.ErrNoRows {
		t.Skip("this installation holds only the base-currency setting, so there is no second row to take")
	}
	if err != nil {
		t.Fatalf("finding another setting: %v", err)
	}
	if err := settings.LockForWrite(admin, other, key); err != nil {
		t.Fatalf("a write to %s waited on a caller deciding against the base currency: %v", key, err)
	}
}
