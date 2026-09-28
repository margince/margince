// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

import (
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A contact that started as its address and is renamed to a name another
// contact already carries is put to a human as a duplicate — the pair a create
// could not see, because at create time the name was "sahner".
func TestRenamingAContactToAnExistingNameQueuesThePair(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	first, err := e.store.CreateContact(ctx, CreateContactInput{
		FullName: "Gerhard Sahner", Source: "manual",
		Emails: []ContactEmailInput{{Email: "info@gerhard-sahner.test", EmailType: "work", IsPrimary: true}},
	})
	if err != nil {
		t.Fatalf("create the first record: %v", err)
	}
	second, err := e.store.CreateContact(ctx, CreateContactInput{
		FullName: "sahner", Source: "manual",
		Emails: []ContactEmailInput{{Email: "sahner@innoscripta.test", EmailType: "work", IsPrimary: true}},
	})
	if err != nil {
		t.Fatalf("create the second record: %v", err)
	}
	secondID := ids.UUID(second.Id)
	if pairs := openCandidatePairs(ctx, t, e, "contact", secondID); len(pairs) != 0 {
		t.Fatalf("got %d pairs before the rename, want 0 — the test needs a pair only the rename can reveal", len(pairs))
	}

	name := "Gerhard Sahner"
	if _, err := e.store.UpdateContact(ctx, ids.From[ids.ContactKind](secondID), UpdateContactInput{FullName: &name}); err != nil {
		t.Fatalf("rename: %v", err)
	}

	pairs := openCandidatePairs(ctx, t, e, "contact", secondID)
	if len(pairs) != 1 || pairs[0].OtherID != ids.UUID(first.Id).String() {
		t.Fatalf("got pairs %+v, want exactly the pair with the first record", pairs)
	}
	var source string
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT source FROM dedupe_candidate WHERE $1 IN (left_contact_id, right_contact_id)`,
			secondID).Scan(&source)
	}); err != nil {
		t.Fatalf("read the pair's source: %v", err)
	}
	if source != contactRenameRecheckSource {
		t.Fatalf("source %q, want %q so the queue says a rename found it", source, contactRenameRecheckSource)
	}
}

// Re-sending the same name, or renaming to a name nobody shares, files nothing
// and never pairs the contact with itself.
func TestARenameNobodySharesLeavesTheQueueEmpty(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	c, err := e.store.CreateContact(ctx, CreateContactInput{
		FullName: "Ottilie Brandauer", Source: "manual",
		Emails: []ContactEmailInput{{Email: "ottilie@brandauer.test", EmailType: "work", IsPrimary: true}},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	id := ids.UUID(c.Id)
	for _, name := range []string{"Ottilie Brandauer", "Ottilie Brandauer-Kern"} {
		n := name
		if _, err := e.store.UpdateContact(ctx, ids.From[ids.ContactKind](id), UpdateContactInput{FullName: &n}); err != nil {
			t.Fatalf("rename to %q: %v", name, err)
		}
	}
	if pairs := openCandidatePairs(ctx, t, e, "contact", id); len(pairs) != 0 {
		t.Fatalf("got %d pairs, want none — a rename nobody shares, and never a pair with itself", len(pairs))
	}
}
