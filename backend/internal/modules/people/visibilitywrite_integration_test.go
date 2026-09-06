// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package people

// Visibility as an ORDINARY field, over a real Postgres.
//
// It used to move one way only. POST /people/{id}/publish widened a contact
// and nothing narrowed one, on the reasoning that a colleague may already have
// acted on seeing it. That reasoning assumed a human made the disclosure, and
// the common case is not a human: the sender classifier publishes a contact it
// judges a real counterparty with nobody approving it. So a machine made a
// decision no human could undo — the row's own owner included.
//
// These tests hold the new rule to the database rather than to a rendered
// predicate: anybody the write gate admits moves the field, in either
// direction, at any time.

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// visibilityOf reads the column straight, because what is under test is what
// the write left in the row.
func (e *privacyEnv) visibilityOf(t *testing.T, id ids.PersonID) string {
	t.Helper()
	ctx := e.as(e.owner, principal.RowScopeOwn)
	var got string
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT visibility FROM person WHERE id = $1`, id).Scan(&got)
	}); err != nil {
		t.Fatalf("reading visibility of %s: %v", id, err)
	}
	return got
}

func (e *privacyEnv) ownerOf(t *testing.T, id ids.PersonID) *ids.UUID {
	t.Helper()
	ctx := e.as(e.owner, principal.RowScopeOwn)
	var got *ids.UUID
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT owner_id FROM person WHERE id = $1`, id).Scan(&got)
	}); err != nil {
		t.Fatalf("reading owner of %s: %v", id, err)
	}
	return got
}

func visibility(v string) *string { return &v }

// TestAWorkspaceContactCanBeMadePrivateAgain is the direction that did not
// exist, and the whole reason for the change.
func TestAWorkspaceContactCanBeMadePrivateAgain(t *testing.T) {
	e := setupCapturePrivacy(t)
	published := e.capturePerson(t, "workspace")
	teammate := e.as(e.teammate, principal.RowScopeTeam)
	if !e.canRead(teammate, t, published) {
		t.Fatal("a workspace contact is not readable by a teammate — the fixture is wrong")
	}

	if _, err := e.store.UpdatePerson(e.as(e.owner, principal.RowScopeOwn), published,
		UpdatePersonInput{Visibility: visibility("owner")}); err != nil {
		t.Fatalf("making a published contact private: %v", err)
	}

	if got := e.visibilityOf(t, published); got != "owner" {
		t.Errorf("visibility = %q, want owner", got)
	}
	if e.canRead(teammate, t, published) {
		t.Error("a teammate still reads a contact its owner has just made private")
	}
	if e.listsIt(teammate, t, published) {
		t.Error("the list still returns a contact its owner has just made private")
	}
}

// TestAPrivateContactCanBePublishedThroughThePatch is the other direction
// through the ordinary field, rather than through the owner's own verb.
func TestAPrivateContactCanBePublishedThroughThePatch(t *testing.T) {
	e := setupCapturePrivacy(t)
	captured := e.capturePerson(t, "owner")
	teammate := e.as(e.teammate, principal.RowScopeTeam)
	if e.canRead(teammate, t, captured) {
		t.Fatal("a captured contact is readable by a teammate — the fixture is wrong")
	}

	if _, err := e.store.UpdatePerson(e.as(e.owner, principal.RowScopeOwn), captured,
		UpdatePersonInput{Visibility: visibility("workspace")}); err != nil {
		t.Fatalf("publishing a captured contact: %v", err)
	}

	if got := e.visibilityOf(t, captured); got != "workspace" {
		t.Errorf("visibility = %q, want workspace", got)
	}
	if !e.canRead(teammate, t, captured) {
		t.Error("a teammate cannot read a contact that has just been published")
	}
}

// TestNarrowingAContactNamesAnOwnerForIt is the trap the field would otherwise
// set. person.owner_id is nullable while visibility independently says
// 'owner', and the row-scope arm reads the pair as
// `visibility <> 'owner' OR owner_id = me` — so an owner-scoped row naming
// nobody satisfies neither arm and is readable by NOBODY, the caller included.
//
// The owner is filled from the acting user. An ownerless row cannot be reached
// through the ordinary write gate at all — OwnerPredicate treats an unowned row
// as nobody's to change until somebody claims it (rowscope.go) — so this drives
// the case that IS reachable: a row owned by somebody else, narrowed by a
// colleague who may write it, keeps its own owner rather than being handed to
// the caller.
func TestNarrowingAContactNamesAnOwnerForIt(t *testing.T) {
	e := setupCapturePrivacy(t)
	published := e.capturePerson(t, "workspace")

	// The teammate narrows a contact the OWNER owns. The row already names an
	// owner, so nothing is filled in and the contact stays the owner's — not
	// the narrowing colleague's.
	if _, err := e.store.UpdatePerson(e.as(e.teammate, principal.RowScopeTeam), published,
		UpdatePersonInput{Visibility: visibility("owner")}); err != nil {
		t.Fatalf("narrowing a colleague's contact: %v", err)
	}

	owner := e.ownerOf(t, published)
	if owner == nil {
		t.Fatal("a contact made private carries no owner — nobody can read it")
	}
	if *owner != e.owner {
		t.Errorf("owner = %s, want the row's existing owner %s — narrowing must not reassign it",
			*owner, e.owner)
	}
	// And the owner still reads their own row, which is what the fill exists
	// to guarantee.
	if !e.canRead(e.as(e.owner, principal.RowScopeOwn), t, published) {
		t.Error("the owner cannot read their own contact after it was made private")
	}
}

// TestAWriteGrantHolderMovesVisibility states the gate: write access, not
// ownership. The owner's own verb (POST /people/{id}/publish) keeps its
// stricter ownership test, because it also publishes the correspondence.
func TestAWriteGrantHolderMovesVisibility(t *testing.T) {
	e := setupCapturePrivacy(t)
	published := e.capturePerson(t, "workspace")

	// The teammate holds `person: update` and reads the row under team scope,
	// which is what EnsureWritable admits.
	if _, err := e.store.UpdatePerson(e.as(e.teammate, principal.RowScopeTeam), published,
		UpdatePersonInput{Visibility: visibility("owner")}); err != nil {
		t.Fatalf("a colleague who may write the record could not narrow it: %v", err)
	}
	if got := e.visibilityOf(t, published); got != "owner" {
		t.Errorf("visibility = %q, want owner", got)
	}
}

// TestCapturePrivacyStillHidesAContactFromTheNewDoor is the security question
// the change had to answer, and the reason it is safe.
//
// The write gate is `person: update` plus EnsureWritable, which is weaker than
// the ownership test POST /people/{id}/publish applies on purpose: capture
// privacy is the importing user's alone, and an admin reading a colleague's
// unpromoted captured contacts is the disclosure that boundary exists to
// prevent (founder decision, rowscope.go).
//
// The new door does not weaken it, because the two gates compose. A
// capture-private row is invisible to every other seat under the row-scope
// arm, so EnsureWritable cannot find it and the patch answers NOT FOUND — the
// same existence-hiding answer the owner's verb gives. Neither a teammate nor
// an admin can publish somebody else's private contact through this field.
func TestCapturePrivacyStillHidesAContactFromTheNewDoor(t *testing.T) {
	e := setupCapturePrivacy(t)
	captured := e.capturePerson(t, "owner")

	for _, reader := range []struct {
		name  string
		user  ids.UUID
		scope principal.RowScope
	}{
		{"a teammate", e.teammate, principal.RowScopeTeam},
		{"an admin", e.admin, principal.RowScopeAll},
	} {
		_, err := e.store.UpdatePerson(e.as(reader.user, reader.scope), captured,
			UpdatePersonInput{Visibility: visibility("workspace")})
		if !errors.Is(err, apperrors.ErrNotFound) {
			t.Errorf("%s publishing somebody else's capture-private contact: err = %v, want not found",
				reader.name, err)
		}
	}
	if got := e.visibilityOf(t, captured); got != "owner" {
		t.Errorf("visibility = %q, want owner — the contact was published by somebody who is not its owner", got)
	}
}
