// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package comms

// What the sweep reads. The rows here are written directly rather than through
// StageControllerTx because the subject is the READER: the writer has its own
// tests, and a reader test that went through it would prove the writer twice
// and the predicate once.

import (
	"context"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// plantControllerPayload writes one controller delivery holding link material.
func plantControllerPayload(t *testing.T, e *storeEnv, ref string, expires *time.Time) ids.UUID {
	t.Helper()
	id := ids.NewV7()
	var storedRef any = ref
	if ref == "" {
		storedRef = nil
	}
	if _, err := e.owner.Exec(context.Background(), `
		INSERT INTO comms_outbound
		  (id, activity_id, provider, recipients, cc, references_chain,
		   message_id, subject, body, status, sender_kind,
		   template_key, template_version, payload_ref, payload_expires_at)
		VALUES ($1, $2, 'operator_relay', '["subject@example.test"]'::jsonb,
		        '[]'::jsonb, '[]'::jsonb, $5,
		        'Your details', 'body {{confirmation-link}}', 'pending',
		        'controller', 'record_confirmation', 1, $3, $4)`,
		id, e.activity, storedRef, expires,
		"<ctrl-"+id.String()+"@margince.test>"); err != nil {
		t.Fatalf("planting a controller payload: %v", err)
	}
	return id
}

// TestTheSweepFindsOnlyMaterialPastItsExpiry is the predicate.
//
// Under-reading is the failure that matters: material this pass does not see is
// a live link to somebody's mailbox that stays live, and nothing else is
// looking — the happy paths retired theirs already. Over-reading costs a
// destroyed link the send path would have destroyed anyway.
func TestTheSweepFindsOnlyMaterialPastItsExpiry(t *testing.T) {
	e := setupStore(t)
	now := e.clockValue

	past := now.Add(-time.Hour)
	future := now.Add(time.Hour)
	expired := plantControllerPayload(t, e, "vault-ref-expired", &past)
	plantControllerPayload(t, e, "vault-ref-live", &future)
	// Already retired by the send path: no reference, so nothing to destroy.
	plantControllerPayload(t, e, "", &past)

	found, _, err := e.store.ExpiredControllerPayloads(context.Background(), now, time.Time{}, 100)
	if err != nil {
		t.Fatalf("reading expired material: %v", err)
	}
	if len(found) != 1 || found[0] != expired {
		t.Fatalf("swept %v, want exactly the expired row %s — a row this pass "+
			"does not see is a live link nobody else is looking at", found, expired)
	}
}

// TestTheSweepReadsTheReferenceItMustDestroy holds the order the retire depends
// on: the vault entry is destroyed BEFORE the column naming it is cleared, so
// the reference has to be readable first.
func TestTheSweepReadsTheReferenceItMustDestroy(t *testing.T) {
	e := setupStore(t)
	past := e.clockValue.Add(-time.Hour)
	id := plantControllerPayload(t, e, "vault-ref-readable", &past)

	ref, err := e.store.PayloadRefFor(context.Background(), id)
	if err != nil {
		t.Fatalf("reading the payload reference: %v", err)
	}
	if ref != "vault-ref-readable" {
		t.Errorf("reference = %q, want the planted one", ref)
	}

	if err := e.store.ClearPayloadRef(context.Background(), id); err != nil {
		t.Fatalf("clearing the reference: %v", err)
	}
	// Cleared, so a second pass finds nothing to destroy and says so with an
	// empty string rather than an error — the sweep treats that as "retired
	// between the read and here", which is a normal race and not a fault.
	after, err := e.store.PayloadRefFor(context.Background(), id)
	if err != nil {
		t.Fatalf("re-reading after the clear: %v", err)
	}
	if after != "" {
		t.Errorf("reference = %q after clearing, want empty", after)
	}
}

// TestTheSweepTakesTheOldestMaterialFirst holds the ordering.
//
// A bounded pass that cannot drain its backlog should shorten the LONGEST
// exposure, not an arbitrary one.
func TestTheSweepTakesTheOldestMaterialFirst(t *testing.T) {
	e := setupStore(t)
	now := e.clockValue
	oldest := now.Add(-72 * time.Hour)
	newer := now.Add(-time.Hour)

	middle := now.Add(-24 * time.Hour)
	// Planted newest-first, so insert order is the REVERSE of expiry order: a
	// sequential scan returning rows as written would fail this, which is what
	// makes the ORDER BY the thing under test rather than an accident.
	third := plantControllerPayload(t, e, "vault-ref-newer", &newer)
	second := plantControllerPayload(t, e, "vault-ref-middle", &middle)
	first := plantControllerPayload(t, e, "vault-ref-oldest", &oldest)

	found, _, err := e.store.ExpiredControllerPayloads(context.Background(), now, time.Time{}, 3)
	if err != nil {
		t.Fatalf("reading expired material: %v", err)
	}
	want := []ids.UUID{first, second, third}
	if len(found) != 3 || found[0] != want[0] || found[1] != want[1] || found[2] != want[2] {
		t.Errorf("swept %v, want oldest-first %v", found, want)
	}
}
