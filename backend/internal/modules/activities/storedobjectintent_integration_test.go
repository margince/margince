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

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// systemStore is the reaper's own view: the sweep runs under the system principal,
// which is what the reference check demands.
func systemStore(e *sendEnv) (*Store, context.Context) {
	ctx := principal.WithWorkspaceID(context.Background(), e.ws)
	ctx = principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalSystem, ID: "system:stored-object-reap",
	})
	return NewStore(database.BindTo(e.pool, ids.From[ids.WorkspaceKind](e.ws))), ctx
}

// A key no attachment row carries is one the sweep may collect.
//
// This module answers only the reference half now: whether a key is old enough is
// the ledger's question, and which module to ask is the sweep's. What stays here is
// the half that needs this module's own table — an object with no row is one an
// erasure cannot reach, because it finds an attachment's bytes by reading
// storage_key off the row.
func TestAKeyWhoseRowNeverArrivedIsUnreferenced(t *testing.T) {
	e := setupSend(t)
	store, ctx := systemStore(e)

	unreferenced, err := store.UnreferencedAttachmentKeys(ctx, []string{"ws/attachment/orphan"})
	if err != nil {
		t.Fatalf("UnreferencedAttachmentKeys: %v", err)
	}
	if len(unreferenced) != 1 || unreferenced[0] != "ws/attachment/orphan" {
		t.Fatalf("got %v, want the one key whose row never arrived", unreferenced)
	}
}

// A live attachment row keeps its bytes out of the reaper's reach.
//
// THE REFERENCE CHECK IS THE SECOND LOCK. The first is that a key was declared at
// all; this exists because a clear that never ran leaves a live file's key
// provisional, and a reaper trusting the ledger alone would delete the bytes of a
// document somebody can still open.
func TestALiveAttachmentRowKeepsItsBytesOutOfTheReapersReach(t *testing.T) {
	e := setupSend(t)
	const key = "ws/attachment/live"
	seedAttachmentRow(t, e, key)
	store, ctx := systemStore(e)

	// Read the orphan alongside it, so an empty answer cannot pass for a working
	// check: the reference check has to tell the two apart, not refuse both.
	unreferenced, err := store.UnreferencedAttachmentKeys(ctx, []string{key, "ws/attachment/orphan"})
	if err != nil {
		t.Fatalf("UnreferencedAttachmentKeys: %v", err)
	}
	for _, k := range unreferenced {
		if k == key {
			t.Fatal("an object a live attachment row references was offered to the reaper — " +
				"a reader can still open that document, and the reap would delete its bytes")
		}
	}
	if len(unreferenced) != 1 {
		t.Fatalf("got %v, want only the unreferenced key: a check that answers nothing for "+
			"everything passes the assertion above while proving nothing", unreferenced)
	}
}

// Nothing is asked of the database for an empty list.
//
// The sweep groups provisional keys by kind, so a kind with none of its own reaches
// this with an empty slice every pass.
func TestNoKeysIsNoQuestion(t *testing.T) {
	e := setupSend(t)
	store, ctx := systemStore(e)

	unreferenced, err := store.UnreferencedAttachmentKeys(ctx, nil)
	if err != nil {
		t.Fatalf("UnreferencedAttachmentKeys: %v", err)
	}
	if len(unreferenced) != 0 {
		t.Fatalf("got %v for no keys at all", unreferenced)
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

// This check answers for ATTACHMENTS, and says so about everything else.
//
// A knowledge document's key has no attachment row, so this correctly calls it
// unreferenced — and deleting it would destroy a live document. That is not a defect
// here; it is why the sweep routes each key to the module that owns its kind instead
// of asking one module about every key. The gate holding that routing is
// compose's TestEveryStoredObjectKindHasAnOwner.
//
// Pinned at the source, because the hazard is invisible from the sweep: a wrong
// route produces a confident "nobody references this" rather than an error.
func TestTheAttachmentCheckAnswersOnlyForAttachments(t *testing.T) {
	e := setupSend(t)
	store, ctx := systemStore(e)

	unreferenced, err := store.UnreferencedAttachmentKeys(ctx, []string{"ws/knowledge/a-live-document"})
	if err != nil {
		t.Fatalf("UnreferencedAttachmentKeys: %v", err)
	}
	if len(unreferenced) != 1 {
		t.Fatalf("got %v; this check reads the attachment table alone, so another kind's key is "+
			"unreferenced TO IT — the routing is what keeps that answer away from the reaper",
			unreferenced)
	}
}
