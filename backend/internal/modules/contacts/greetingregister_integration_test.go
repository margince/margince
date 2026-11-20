// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/greetingregister"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func (e *dedupeEnv) greetingAudit(ctx context.Context, t *testing.T, id ids.ContactID) (before, after *string) {
	t.Helper()
	if err := database.WithWorkspaceTx(ctx, e.store.db.Pool(), func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT before->>'greeting_register', after->>'greeting_register' FROM audit_log
			 WHERE entity_type = 'contact' AND entity_id = $1 AND action = 'update'
			 ORDER BY occurred_at DESC, id DESC LIMIT 1`, id).Scan(&before, &after)
	}); err != nil {
		t.Fatalf("reading the audit row: %v", err)
	}
	return before, after
}

func TestARepSetsAndClearsTheGreetingRegisterAndEachEditIsAudited(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	created, err := e.store.CreateContact(ctx, CreateContactInput{FullName: "Dietmar Rietsch", Source: "manual"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.GreetingRegister != nil {
		t.Fatalf("a new contact has register %v, want none", *created.GreetingRegister)
	}
	id := ids.From[ids.ContactKind](ids.UUID(created.Id))

	register := greetingregister.Formal
	set, err := e.store.UpdateContact(ctx, id, UpdateContactInput{GreetingRegister: &register})
	if err != nil {
		t.Fatalf("set: %v", err)
	}
	if set.GreetingRegister == nil || *set.GreetingRegister != crmcontracts.ContactGreetingRegisterFormal {
		t.Fatalf("the update returned register %v, want formal", set.GreetingRegister)
	}
	before, after := e.greetingAudit(ctx, t, id)
	if before != nil || after == nil || *after != "formal" {
		t.Fatalf("audit before/after = %v/%v, want nil/formal", before, after)
	}

	read, err := e.store.GetContact(ctx, id, storekit.LiveOnly)
	if err != nil || read.GreetingRegister == nil || *read.GreetingRegister != crmcontracts.ContactGreetingRegisterFormal {
		t.Fatalf("a read returned %v (err %v), want formal", read.GreetingRegister, err)
	}

	cleared, err := e.store.UpdateContact(ctx, id, UpdateContactInput{Clear: []string{"greeting_register"}})
	if err != nil {
		t.Fatalf("clear: %v", err)
	}
	if cleared.GreetingRegister != nil {
		t.Fatalf("the update returned register %v after a clear", *cleared.GreetingRegister)
	}
	before, after = e.greetingAudit(ctx, t, id)
	if before == nil || *before != "formal" || after != nil {
		t.Fatalf("audit before/after = %v/%v after a clear, want formal/nil", before, after)
	}
}

func TestAnUnknownGreetingRegisterIsRefused(t *testing.T) {
	e := setupDedupe(t)
	ctx := e.as()
	created, err := e.store.CreateContact(ctx, CreateContactInput{FullName: "Pat Doe", Source: "manual"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	bogus := greetingregister.Register("casual")
	_, err = e.store.UpdateContact(ctx, ids.From[ids.ContactKind](ids.UUID(created.Id)), UpdateContactInput{GreetingRegister: &bogus})
	if err == nil {
		t.Fatal("an unknown greeting register was accepted")
	}
}
