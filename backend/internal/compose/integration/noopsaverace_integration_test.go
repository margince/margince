// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A save of the values a record held decides it changed nothing only under
// the record's lock. A save that arrives while another write holds that lock
// waits, then compares with what the other write committed.

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
)

// changeWhileASaveWaits runs change in a transaction of its own, starts save,
// waits until save is blocked on the lock change holds, and commits change.
// It answers save's error.
func changeWhileASaveWaits(t *testing.T, change func(context.Context, pgx.Tx) error, waitsOn string, save func() error) error {
	t.Helper()
	ctx := context.Background()
	tx, err := freshConnection(t).Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := change(ctx, tx); err != nil {
		t.Fatalf("the change that holds the lock: %v", err)
	}
	saved := make(chan error, 1)
	returned := make(chan struct{})
	go func() {
		defer close(returned)
		saved <- save()
	}()
	waitUntilAStatementWaits(t, waitsOn, returned,
		"the save returned while another write held its lock, so it compared without the lock",
		"the save never queued on the lock the change holds")
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("committing the change: %v", err)
	}
	return <-saved
}

func TestARepeatedWorkingHoursSaveComparesUnderTheSeatLock(t *testing.T) {
	e := Setup(t)
	seat := e.As(e.Rep1, nil, AdminPerms)
	svc := identity.NewService(e.Pool)
	window := identity.WorkingHours{StartMinute: 8 * 60, EndMinute: 16 * 60, Days: []int{1, 2, 3, 4, 5}}
	if _, err := svc.SaveMyWorkingHours(seat, window); err != nil {
		t.Fatalf("the first save: %v", err)
	}
	err := changeWhileASaveWaits(t, func(ctx context.Context, tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE app_user SET work_start_minute = 600 WHERE id = $1`, e.Rep1)
		return err
	}, `SELECT 1 FROM app_user WHERE id = $1 FOR UPDATE`, func() error {
		_, err := svc.SaveMyWorkingHours(seat, window)
		return err
	})
	if err != nil {
		t.Fatalf("the save that waited: %v", err)
	}
	var start int
	if err := database.WithTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `SELECT work_start_minute FROM app_user WHERE id = $1`, e.Rep1).Scan(&start)
	}); err != nil {
		t.Fatal(err)
	}
	if start != window.StartMinute {
		t.Errorf("after the waiting save the seat starts at minute %d, want the %d it saved", start, window.StartMinute)
	}
}

func TestARepeatedDomainDecisionComparesUnderTheAdmissionLock(t *testing.T) {
	e := Setup(t)
	const domain, reason = "racing.test", "They became a client."
	if _, err := e.Contacts.SetDomainAdmission(e.Admin(), domain, "admitted", reason); err != nil {
		t.Fatalf("the first decision: %v", err)
	}
	err := changeWhileASaveWaits(t, func(ctx context.Context, tx pgx.Tx) error {
		if err := storekit.LockWriteIdentity(ctx, tx, "domain_admission", domain); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `UPDATE company_domain_disposition SET admission_reason = 'Changed meanwhile.' WHERE domain = $1`, domain)
		return err
	}, `%pg_advisory_xact_lock%`, func() error {
		_, err := e.Contacts.SetDomainAdmission(e.Admin(), domain, "admitted", reason)
		return err
	})
	if err != nil {
		t.Fatalf("the decision that waited: %v", err)
	}
	var stored string
	if err := database.WithTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `SELECT admission_reason FROM company_domain_disposition WHERE domain = $1`, domain).Scan(&stored)
	}); err != nil {
		t.Fatal(err)
	}
	if stored != reason {
		t.Errorf("after the waiting decision the reason reads %q, want the %q it saved", stored, reason)
	}
}
