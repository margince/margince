// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

// What a send does with an address a rep typed. A free address becomes the
// rep's contact. A held address stays with its holder, even a hidden one.

import (
	"context"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func (e *privacyEnv) fileTyped(ctx context.Context, t *testing.T, addresses ...string) []string {
	t.Helper()
	var filed []string
	if err := e.store.tx(ctx, func(tx pgx.Tx) (err error) {
		filed, err = e.store.FileTypedRecipientsTx(ctx, tx, addresses)
		return err
	}); err != nil {
		t.Fatalf("FileTypedRecipientsTx: %v", err)
	}
	return filed
}

func (e *privacyEnv) visibleTo(ctx context.Context, t *testing.T, address string) bool {
	t.Helper()
	var found map[string]bool
	if err := e.store.tx(ctx, func(tx pgx.Tx) (err error) {
		found, err = VisibleAddresses(ctx, tx, []string{address})
		return err
	}); err != nil {
		t.Fatalf("VisibleAddresses: %v", err)
	}
	return found[address]
}

// ownersHolding lists the owner of each live contact holding address.
func (e *privacyEnv) ownersHolding(t *testing.T, address string) []ids.UUID {
	t.Helper()
	ctx := e.as(e.owner, principal.RowScopeAll)
	var owners []ids.UUID
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT c.owner_id FROM contact c
			  JOIN contact_email ce ON ce.contact_id = c.id AND ce.archived_at IS NULL
			 WHERE ce.email = $1 AND c.archived_at IS NULL`, address)
		if err != nil {
			return err
		}
		owners, err = pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
		return err
	}); err != nil {
		t.Fatalf("reading the contacts holding %s: %v", address, err)
	}
	return owners
}

// A new address becomes one contact, owned by whoever typed it, and is then an
// address that rep may write to. Typing it again files nothing.
//
// Mutation: return early from FileTypedRecipientsTx and nothing is filed.
func TestANewTypedAddressIsFiledAsTheCallersContact(t *testing.T) {
	e := setupCapturePrivacy(t)
	ctx := e.as(e.admin, principal.RowScopeAll)

	filed := e.fileTyped(ctx, t, "Fresh.Buyer@Typed.test", "fresh.buyer@typed.test")
	if !slices.Equal(filed, []string{"fresh.buyer@typed.test"}) {
		t.Fatalf("filed %v, want the one folded address once", filed)
	}
	if owners := e.ownersHolding(t, "fresh.buyer@typed.test"); !slices.Equal(owners, []ids.UUID{e.admin}) {
		t.Fatalf("owners of the filed address = %v, want the caller %s alone", owners, e.admin)
	}
	if !e.visibleTo(ctx, t, "fresh.buyer@typed.test") {
		t.Error("the caller cannot see the contact it just filed")
	}
	if again := e.fileTyped(ctx, t, "fresh.buyer@typed.test"); len(again) != 0 {
		t.Errorf("a second filing made %v, want nothing", again)
	}
}

// An address on a contact the caller cannot read is still held: filing it
// again would put a second record beside a private one. The address stays
// invisible to the caller, so the send refuses it as before.
//
// Mutation: drop the holder check from typedRecipientFileable and the create
// refuses the held address, which fails the send.
func TestATypedAddressOnAHiddenContactIsNotFiledAgain(t *testing.T) {
	e := setupCapturePrivacy(t)
	hidden := e.captureContact(t, "owner")
	e.seedAddress(t, hidden, "private@typed.test")
	ctx := e.as(e.admin, principal.RowScopeAll)

	if filed := e.fileTyped(ctx, t, "private@typed.test"); len(filed) != 0 {
		t.Fatalf("filed %v over a contact the caller cannot see, want nothing", filed)
	}
	if owners := e.ownersHolding(t, "private@typed.test"); !slices.Equal(owners, []ids.UUID{e.owner}) {
		t.Fatalf("owners of the hidden address = %v, want the original owner alone", owners)
	}
	if e.visibleTo(ctx, t, "private@typed.test") {
		t.Error("the hidden contact's address resolved for a caller who cannot read it")
	}
}

// A lead's address is the lead's. The send writes to the lead: no contact is
// filed, and the address is one the caller may write to.
//
// Mutation: drop visibleLeadAddresses from VisibleAddresses and the lead's
// address is not visible.
func TestALeadsAddressIsNotFiledAndMayBeWrittenTo(t *testing.T) {
	e := setupCapturePrivacy(t)
	ctx := e.as(e.admin, principal.RowScopeAll)
	name, email := "Prospect Lead", "lead@typed.test"
	if _, _, err := e.store.CreateLead(ctx, CreateLeadInput{FullName: &name, Email: &email, Status: "new"}); err != nil {
		t.Fatalf("creating the lead: %v", err)
	}

	if filed := e.fileTyped(ctx, t, email); len(filed) != 0 {
		t.Fatalf("filed %v over a lead, want nothing", filed)
	}
	if owners := e.ownersHolding(t, email); len(owners) != 0 {
		t.Fatalf("%d contact(s) hold the lead's address, want none", len(owners))
	}
	if !e.visibleTo(ctx, t, email) {
		t.Error("the lead's address is not one the caller may write to")
	}
}
