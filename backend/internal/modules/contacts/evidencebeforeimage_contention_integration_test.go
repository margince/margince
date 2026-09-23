// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

// What a correction's audit before-image describes when somebody else wrote to
// the same sidecar row first.
//
// It has to be the row the correction actually replaced. The image is the only
// record that answers "what did it say before I fixed it", and an image taken
// from a read that happened before the lock answers with a value the fix never
// saw — silently, because both writes succeed and neither reports anything.
//
// The interleaving is frozen by hand rather than raced for: a transaction holds
// the row and changes it, the correction is released only once a backend is
// provably waiting on that lock, and the holder commits then. No sleep, no
// clock, no guess — the same probe the phone-lane contention tests use.

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// beginHeldSidecarWrite opens a transaction that has locked one sidecar row and
// changed its value, and hands it back still open — so the test owns the moment
// a correction can see it.
//
// Its rollback is registered here rather than left to the caller: a transaction
// still open on a failure path holds the row lock and a pooled connection with
// it, and the run that meant to fail loudly would hang instead.
func (e *dedupeEnv) beginHeldSidecarWrite(
	ctx context.Context, t *testing.T, table string, id ids.UUID, value string,
) (pgx.Tx, int) {
	t.Helper()
	tx, err := e.store.db.Pool().Begin(ctx)
	if err != nil {
		t.Fatalf("opening the in-flight write's transaction: %v", err)
	}
	t.Cleanup(func() {
		err := tx.Rollback(context.Background())
		if err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			t.Errorf("releasing the in-flight write's transaction: %v", err)
		}
	})
	var pid int
	if err := tx.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&pid); err != nil {
		t.Fatalf("reading the in-flight write's backend pid: %v", err)
	}
	if _, err := tx.Exec(ctx,
		`UPDATE `+pgx.Identifier{table}.Sanitize()+
			` SET value = $2, version = version + 1 WHERE id = $1`, id, value); err != nil {
		t.Fatalf("the in-flight write on %s: %v", table, err)
	}
	return tx, pid
}

// correctionInBackground runs one correction on its own connection — the second
// request, arriving while the first write is still uncommitted. The channel is
// buffered so the goroutine can always finish and hand its result back, even on
// a path where the test fails before reading it.
func correctionInBackground(correct func() error) <-chan error {
	done := make(chan error, 1)
	go func() { done <- correct() }()
	return done
}

// sidecarRowID names one sidecar row the way the test's own fixture addresses
// it, so a test never asserts against a row it located differently from the
// writer under test.
func sidecarRowID(ctx context.Context, t *testing.T, e *dedupeEnv, query string, args ...any) ids.UUID {
	t.Helper()
	var id ids.UUID
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, query, args...).Scan(&id)
	}); err != nil {
		t.Fatalf("locating the sidecar row: %v", err)
	}
	return id
}

// A fact correction records the value it replaced, not the one it was shown.
func TestACorrectedFactsBeforeImageIsTheRowTheCorrectionReplaced(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	companyID := evidenceCompany(ctx, t, e)
	factID := sidecarRowID(ctx, t, e,
		`SELECT id FROM company_fact WHERE company_id = $1 AND field = 'phone' AND value_key = ''`,
		companyID)

	// A second site read lands on the same claim while a rep is typing.
	const readAgain = "+49 30 555000"
	held, pid := e.beginHeldSidecarWrite(ctx, t, tableCompanyFact, factID, readAgain)

	typed := "+49 30 1234 567"
	done := correctionInBackground(func() error {
		_, err := e.store.UpdateCompanyFact(ctx, companyID, "phone:", FactWriteInput{Value: &typed})
		return err
	})
	blocked, err := waitUntilBlockedBy(t, held, pid, done)
	if !blocked {
		// Not a flaky setup — a correction that never waited is the defect
		// itself, a write that read and patched straight through an in-flight
		// one. It is reported here rather than judged below, because the
		// before-image of a write that never contended proves nothing.
		t.Fatalf("the correction never waited on the held row (it returned %v): there was no "+
			"interleaving to observe, so this run proved nothing", err)
	}
	if err := held.Commit(ctx); err != nil {
		t.Fatalf("committing the in-flight write: %v", err)
	}
	if err := <-done; err != nil {
		t.Fatalf("the correction must not fail behind a concurrent write: %v", err)
	}

	before, after := auditImagesHolding(ctx, t, e.store, tableCompanyFact, factID, auditKeyValue)
	wantImage(t, before, "before", auditKeyValue, readAgain)
	wantImage(t, after, "after", auditKeyValue, typed)
	if got := factValue(ctx, t, e, companyID, "phone", ""); got != typed {
		t.Errorf("the fact now reads %q, want the correction %q", got, typed)
	}
}

// The same question of the other sidecar, because it is the other locating
// read: a profile field is addressed by (company_id, field) and a fact by its
// value key, so each resolves its row its own way and each has to lock before
// it reads.
func TestACorrectedProfileFieldsBeforeImageIsTheRowTheCorrectionReplaced(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	companyID := evidenceCompany(ctx, t, e)
	fieldID := sidecarRowID(ctx, t, e,
		`SELECT id FROM company_profile_field WHERE company_id = $1 AND field = 'icp'`, companyID)

	const readAgain = "Utilities and grid operators"
	held, pid := e.beginHeldSidecarWrite(ctx, t, "company_profile_field", fieldID, readAgain)

	typed := "Mid-market industrial manufacturers"
	done := correctionInBackground(func() error {
		_, err := e.store.UpdateCompanyProfileField(ctx, companyID, "icp",
			ProfileFieldWriteInput{Value: &typed})
		return err
	})
	blocked, err := waitUntilBlockedBy(t, held, pid, done)
	if !blocked {
		t.Fatalf("the correction never waited on the held row (it returned %v): there was no "+
			"interleaving to observe, so this run proved nothing", err)
	}
	if err := held.Commit(ctx); err != nil {
		t.Fatalf("committing the in-flight write: %v", err)
	}
	if err := <-done; err != nil {
		t.Fatalf("the correction must not fail behind a concurrent write: %v", err)
	}

	before, _ := auditImagesHolding(ctx, t, e.store, "company_profile_field", fieldID, auditKeyValue)
	wantImage(t, before, "before", auditKeyValue, readAgain)
}
