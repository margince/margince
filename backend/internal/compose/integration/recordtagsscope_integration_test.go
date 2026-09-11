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

// A RETIRED WORD COMES OFF THE RECORD IT IS STILL SITTING ON.
//
// Retiring a tag takes it out of the vocabulary; it does not take it off the
// records already carrying it, and RecordTagsFor returns those assignments
// deliberately — flagged and sorted after the live ones, because a retired word
// is history a reader is owed. RemoveTag refused them as NOT FOUND, so the
// surface handed a caller a tag and then denied it existed when they tried to
// take it off, and nothing else could take it off either: retiring the word is
// what put it in that state.
//
// Removing is the opposite case from applying, which is why it does not share
// the archived check: applying a retired word coins it again, and taking one
// off is the cleanup retiring it left behind.
func TestARetiredTagCanStillBeTakenOffARecord(t *testing.T) {
	e := Setup(t)
	store := collections.NewStore(e.DB())
	ctx := e.As(e.Rep1, nil, curatePersonAndTagPerms())

	person := e.SeedPerson(t, "Still Carries A Retired Word", &e.Rep1)
	tag, err := store.CreateTag(ctx, "Retired Conference", nil, nil)
	if err != nil {
		t.Fatalf("coining the word: %v", err)
	}
	if _, err := store.ApplyTag(ctx, tag.ID, "person", person); err != nil {
		t.Fatalf("putting the word on the record: %v", err)
	}
	if _, err := store.ArchiveTag(ctx, tag.ID); err != nil {
		t.Fatalf("retiring the word: %v", err)
	}

	// The read still reports it, which is what makes the refusal a contradiction
	// rather than merely a limitation.
	carried, err := store.RecordTagsFor(ctx, "person", person)
	if err != nil {
		t.Fatalf("reading the record's tags: %v", err)
	}
	if !carriesTag(carried, tag.ID) {
		t.Fatalf("the record stopped reporting the retired word, so this case no longer "+
			"describes the surface: %+v", carried)
	}

	if err := store.RemoveTag(ctx, tag.ID, "person", person); err != nil {
		t.Fatalf("taking the retired word off the record answered %v — the read hands it back "+
			"and nothing can remove it, so the record is stuck with it", err)
	}

	left, err := store.RecordTagsFor(ctx, "person", person)
	if err != nil {
		t.Fatalf("re-reading the record's tags: %v", err)
	}
	if carriesTag(left, tag.ID) {
		t.Errorf("the retired word is still on the record after removal: %+v", left)
	}
}

// curatePersonAndTagPerms may write the person and curate the vocabulary.
//
// RowScopeAll because retiring a word is a workspace-wide act — the vocabulary
// gate refuses a bounded seat outright — and this case is about what happens
// AFTER the word is retired, not about who may retire one.
func curatePersonAndTagPerms() principal.Permissions {
	return principal.Permissions{
		Objects: map[string]principal.ObjectGrant{
			"person": {Read: true, Create: true, Update: true},
			"tag":    {Read: true, Create: true, Update: true, Delete: true},
		},
		RowScope: principal.RowScopeAll,
	}
}

// carriesTag reports whether a record's tag list names this tag.
func carriesTag(carried collections.RecordTags, tag ids.TagID) bool {
	for _, got := range carried.Data {
		if got.TagID == tag.UUID {
			return true
		}
	}
	return false
}
