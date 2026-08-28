// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package people

// What a domain admission records about the decision it replaced, and what
// makes that record true under a second writer.
//
// The image chooses which audit door the write takes — "there was no decision"
// or "the decision was this" — so a stale read does not merely lose detail: it
// makes the row CLAIM nothing was there. The prior state is therefore read by
// the upsert itself, under a lock on the domain, and both halves are pinned
// here.

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// holdDomainAdmissionLock opens a transaction holding one domain's admission
// write identity and hands it back still open, so the test owns the moment a
// second decision may proceed. Taken through the production helper: a change to
// the key can never leave this transaction holding something no writer waits on.
//
// Its rollback is registered here — a transaction left open on a failure path
// holds the lock and a pooled connection with it, and the run that meant to fail
// loudly would hang instead.
func (e *dedupeEnv) holdDomainAdmissionLock(ctx context.Context, t *testing.T, domain string) (pgx.Tx, int) {
	t.Helper()
	tx, err := e.store.db.Pool().Begin(ctx)
	if err != nil {
		t.Fatalf("opening the lock holder's transaction: %v", err)
	}
	t.Cleanup(func() {
		err := tx.Rollback(context.Background())
		if err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			t.Errorf("releasing the lock holder's transaction: %v", err)
		}
	})
	var pid int
	if err := tx.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&pid); err != nil {
		t.Fatalf("reading the lock holder's backend pid: %v", err)
	}
	if err := lockDomainAdmissionTx(ctx, tx, domain); err != nil {
		t.Fatalf("taking the admission write identity for %s: %v", domain, err)
	}
	return tx, pid
}

// A decision waits for whoever is already deciding about the same domain. The
// prior admission is what chooses the audit door, so two decisions that read it
// concurrently would both find nothing and both record a first decision — and
// one of those rows would be claiming there was nothing to replace when there
// was.
func TestASecondAdmissionDecisionWaitsOnTheDomainLock(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	const domain = "contended.example"

	holder, pid := e.holdDomainAdmissionLock(ctx, t, domain)

	done := make(chan error, 1)
	go func() {
		_, err := e.store.SetDomainAdmission(ctx, domain, DomainSuppressed, "a tool we use, not a customer")
		done <- err
	}()

	mustBlockOn(t, holder, pid, done)

	if err := holder.Rollback(ctx); err != nil {
		t.Fatalf("releasing the lock holder: %v", err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("the decision failed once the lock was free: %v", err)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("the decision never finished after the lock was released")
	}
}

// The two doors, on one domain. The first decision replaced no admission, no
// reason and nobody answerable for one, and says so by writing SQL NULL; the
// second moved all three and names what they were.
func TestALaterAdmissionDecisionRecordsTheOneItReplaced(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	const domain = "mckinsey.example"

	first, err := e.store.SetDomainAdmission(ctx, domain, DomainSuppressed, "a newsletter we never asked for")
	if err != nil {
		t.Fatalf("the first decision: %v", err)
	}
	if beforeJSON := admissionBeforeImage(ctx, t, e, first.ID, 1); beforeJSON != nil {
		t.Errorf("the first decision recorded %s as what it replaced, want SQL NULL — nothing was there", beforeJSON)
	}

	if _, err := e.store.SetDomainAdmission(ctx, domain, DomainAdmitted, "they became a client"); err != nil {
		t.Fatalf("the second decision: %v", err)
	}
	beforeJSON := admissionBeforeImage(ctx, t, e, first.ID, 2)
	if beforeJSON == nil {
		t.Fatal("the second decision recorded SQL NULL, want the suppression it overwrote")
	}
	var before map[string]any
	if err := json.Unmarshal(beforeJSON, &before); err != nil {
		t.Fatalf("the before image is not an object: %v", err)
	}
	wantImage(t, before, "before", "admission", DomainSuppressed)
	wantImage(t, before, "before", "admission_reason", "a newsletter we never asked for")
	wantImage(t, before, "before", "admission_source", AdmissionSourceHuman)
}

// admissionBeforeImage returns the raw before column of the nth-oldest audit
// row for one disposition, oldest first, so a test can tell an absent image
// from an empty one.
func admissionBeforeImage(ctx context.Context, t *testing.T, e *dedupeEnv, dispositionID ids.UUID, nth int) []byte {
	t.Helper()
	var beforeJSON []byte
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT before FROM audit_log
			 WHERE entity_type = $1 AND entity_id = $2 AND action = 'update'
			 ORDER BY occurred_at, id
			 LIMIT 1 OFFSET $3`, entityOrganization, dispositionID, nth-1).Scan(&beforeJSON)
	}); err != nil {
		t.Fatalf("reading decision %d's audit row: %v", nth, err)
	}
	return beforeJSON
}
