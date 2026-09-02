// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// What the record-tags read hides, and what it deliberately does not.
//
// A first draft of this file asserted that a rep reading another rep's person
// gets not-found. That is NOT this product's rule: customer identity is
// workspace-readable (auth/tableclass_test.go pins it), so every seat reads
// every person and the WRITE arm is what keeps a row its owner's. The test
// failed, a control showed the shared row-scope helper admitting the same
// record, and the rule turned out to be the deliberate one.
//
// What actually hides a record is capture privacy: a captured person answers
// to its owner alone until somebody promotes or shares it. That is the case
// worth pinning here, because it is the one where this read could leak.

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/margince/margince/backend/internal/modules/collections"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// ownPersonAndTagPerms reads its own person rows and the vocabulary — narrow
// enough that a refusal is about the record rather than about the tags.
func ownPersonAndTagPerms() principal.Permissions {
	return principal.Permissions{
		Objects: map[string]principal.ObjectGrant{
			"person": {Read: true},
			"tag":    {Read: true},
		},
		RowScope: principal.RowScopeOwn,
	}
}

// An owner-private captured person is invisible to another seat, and the tags
// on it must be too — otherwise this read becomes the side channel that says a
// private contact exists and what somebody labelled them.
//
// Not-found, never forbidden: a refusal saying "you may not read this" would
// confirm the record to a caller who cannot see it.
func TestRecordTagsHidesAnOwnerPrivateCapture(t *testing.T) {
	e := Setup(t)
	store := collections.NewStore(e.DB())

	person := e.SeedPerson(t, "A Private Capture", &e.Rep1)
	makeCapturePrivate(t, e, person, e.Rep1)

	ctx := e.As(e.Rep3, nil, ownPersonAndTagPerms())
	_, err := store.RecordTagsFor(ctx, "person", person)
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("reading an owner-private capture answered %v, want not-found — "+
			"the tags on a private contact are as private as the contact", err)
	}
}

// The owner of that same private capture still reads it, which is what makes
// the refusal above about privacy rather than about the grants.
func TestRecordTagsAnswersForThePrivateCapturesOwner(t *testing.T) {
	e := Setup(t)
	store := collections.NewStore(e.DB())

	person := e.SeedPerson(t, "A Private Capture", &e.Rep1)
	makeCapturePrivate(t, e, person, e.Rep1)

	ctx := e.As(e.Rep1, nil, ownPersonAndTagPerms())
	read, err := store.RecordTagsFor(ctx, "person", person)
	if err != nil {
		t.Fatalf("the owner reading their own private capture answered %v", err)
	}
	if read.Withheld {
		t.Error("a caller holding tag.read got a withheld answer")
	}
}

// makeCapturePrivate marks a person as an owner-private capture, the state a
// mail or business-card import produces before anybody promotes it.
func makeCapturePrivate(t *testing.T, e *Env, person ids.UUID, owner ids.UUID) {
	t.Helper()
	err := e.DB().Tx(e.Admin(), func(tx pgx.Tx) error {
		_, execErr := tx.Exec(e.Admin(),
			`UPDATE person SET visibility = 'owner', owner_id = $2 WHERE id = $1`,
			person, owner)
		return execErr
	})
	if err != nil {
		t.Fatalf("making the capture private: %v", err)
	}
}
