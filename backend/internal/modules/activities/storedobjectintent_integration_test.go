// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

// The intent ledger over real Postgres, which is the only place its claims mean
// anything: the whole mechanism is about what survives a transaction that
// failed, and a fake cannot fail a transaction.
//
// Two acceptance criteria from the issue, asked directly:
//
//   - a transaction that fails after a put leaves no object behind once the
//     cleanup has run;
//   - the cleanup cannot delete an object a live attachment row references,
//     proven by putting one in its way.

import (
	"context"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// provisional plants one ledger row as a writer would, dated `age` ago.
//
// Written through the OWNER connection so the test states the ledger's state
// directly: what these cases are about is what the reaper is offered, not how a
// key got there, and the writers' own half is covered where they are called.
func provisional(t *testing.T, e *sendEnv, key string, age time.Duration) {
	t.Helper()
	if _, err := e.owner.Exec(context.Background(), `
		INSERT INTO stored_object_intent (storage_key, recorded_at)
		VALUES ($1, now() - $2::interval)`, key, age.String()); err != nil {
		t.Fatalf("planting a provisional key: %v", err)
	}
}

// systemStore is the reaper's own view: the sweep runs under the system
// principal, which is what ListOrphanedObjects demands.
func systemStore(e *sendEnv) (*Store, context.Context) {
	ctx := principal.WithWorkspaceID(context.Background(), e.ws)
	ctx = principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalSystem, ID: "system:stored-object-reap",
	})
	return NewStore(database.BindTo(e.pool, ids.From[ids.WorkspaceKind](e.ws))), ctx
}

func TestAKeyWhoseRowNeverArrivedIsOfferedToTheReaper(t *testing.T) {
	e := setupSend(t)
	provisional(t, e, "ws/attachment/orphan", 2*ProvisionalObjectGrace)
	store, ctx := systemStore(e)

	orphans, err := store.ListOrphanedObjects(ctx, time.Now().UTC().Add(-ProvisionalObjectGrace), 50)
	if err != nil {
		t.Fatalf("ListOrphanedObjects: %v", err)
	}

	if len(orphans) != 1 || orphans[0].StorageKey != "ws/attachment/orphan" {
		t.Fatalf("got %+v, want the one key whose row never arrived — an object with no row is one an "+
			"erasure cannot reach, because it finds an attachment's bytes by reading storage_key off the row",
			orphans)
	}
}

func TestAKeyStillInsideTheGracePeriodIsLeftAlone(t *testing.T) {
	e := setupSend(t)
	provisional(t, e, "ws/attachment/inflight", time.Minute)
	store, ctx := systemStore(e)

	orphans, err := store.ListOrphanedObjects(ctx, time.Now().UTC().Add(-ProvisionalObjectGrace), 50)
	if err != nil {
		t.Fatalf("ListOrphanedObjects: %v", err)
	}

	// The window between the put and the row is a handful of statements. A
	// reaper that did not wait would delete the object of an upload still in
	// flight, which is a worse failure than the orphan it is chasing.
	for _, o := range orphans {
		if o.StorageKey == "ws/attachment/inflight" {
			t.Fatal("an upload still inside the grace period was offered to the reaper")
		}
	}
}

func TestALiveAttachmentRowKeepsItsBytesOutOfTheReapersReach(t *testing.T) {
	e := setupSend(t)
	// A key that is BOTH provisional past the grace period and referenced by a
	// live row — which is what a clear that never ran looks like. The ledger
	// alone would offer it up; the join is what refuses.
	const key = "ws/attachment/live"
	seedAttachmentRow(t, e, key)
	provisional(t, e, key, 2*ProvisionalObjectGrace)
	store, ctx := systemStore(e)

	orphans, err := store.ListOrphanedObjects(ctx, time.Now().UTC().Add(-ProvisionalObjectGrace), 50)
	if err != nil {
		t.Fatalf("ListOrphanedObjects: %v", err)
	}

	for _, o := range orphans {
		if o.StorageKey == key {
			t.Fatal("an object a live attachment row references was offered to the reaper — " +
				"a reader can still open that document, and the reap would delete its bytes")
		}
	}
}

func TestRetiringAnOrphanTakesItOutOfEveryLaterPass(t *testing.T) {
	e := setupSend(t)
	const key = "ws/attachment/retired"
	provisional(t, e, key, 2*ProvisionalObjectGrace)
	store, ctx := systemStore(e)

	if err := store.RetireOrphanedObject(ctx, key); err != nil {
		t.Fatalf("RetireOrphanedObject: %v", err)
	}

	orphans, err := store.ListOrphanedObjects(ctx, time.Now().UTC().Add(-ProvisionalObjectGrace), 50)
	if err != nil {
		t.Fatalf("ListOrphanedObjects: %v", err)
	}
	for _, o := range orphans {
		if o.StorageKey == key {
			t.Fatal("a retired key came back, so every pass would delete bytes that are already gone")
		}
	}
}

// seedAttachmentRow writes the live row whose bytes must stay out of reach.
func seedAttachmentRow(t *testing.T, e *sendEnv, key string) {
	t.Helper()
	activityID := ids.NewV7()
	ctx := context.Background()
	if _, err := e.owner.Exec(ctx, `
		INSERT INTO activity (id, kind, subject, occurred_at, source, captured_by)
		VALUES ($1, 'note', 'a note with a file', now(), 'human', 'human:test')`,
		activityID); err != nil {
		t.Fatalf("seeding the activity: %v", err)
	}
	if _, err := e.owner.Exec(ctx, `
		INSERT INTO attachment (id, entity_type, entity_id, filename, byte_size,
			storage_key, checksum, source, captured_by)
		VALUES ($1, 'activity', $2, 'live.pdf', 12, $3, 'sum', 'human', 'human:test')`,
		ids.NewV7(), activityID, key); err != nil {
		t.Fatalf("seeding the attachment row: %v", err)
	}
}
