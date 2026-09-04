// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// Who may author the tag VOCABULARY.
//
// Renaming, retiring, restoring or folding a word changes what every record
// carrying it is filed under — including records the caller cannot see or
// write through any other route. So `tag.update` alone was never the whole
// question, and a caller bounded to their own or their team's rows holding it
// could rewrite the workspace's vocabulary.
//
// Not reachable through the seed: every role holding the grant today is
// row_scope=all. It becomes reachable the moment an admin grants `tag` to a
// bounded role, which the contract documents as intentional.

import (
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/modules/collections"
	"github.com/margince/margince/backend/internal/modules/people"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// boundedTagAuthor holds the vocabulary grants in FULL and sees only its own
// rows — the caller an admin creates by granting `tag` to a team-scoped role,
// and the one this gate exists for. A refusal can only come from the scope.
func boundedTagAuthor() principal.Permissions {
	return principal.Permissions{
		Objects: map[string]principal.ObjectGrant{
			"tag":          {Create: true, Read: true, Update: true, Delete: true},
			"person":       {Create: true, Read: true, Update: true, Delete: true},
			"organization": {Create: true, Read: true, Update: true, Delete: true},
		},
		RowScope: principal.RowScopeOwn,
	}
}

func TestABoundedSeatCannotAuthorTheVocabulary(t *testing.T) {
	e := Setup(t)
	store := collections.NewStore(e.DB())

	word, err := store.CreateTag(e.Admin(), "BoundedScopeWord", nil, nil)
	if err != nil {
		t.Fatalf("coining the word: %v", err)
	}
	bounded := e.As(e.AdminUser, nil, boundedTagAuthor())
	other, err := store.CreateTag(e.Admin(), "BoundedScopeOther", nil, nil)
	if err != nil {
		t.Fatalf("coining the second word: %v", err)
	}

	name := "Renamed"
	for _, c := range []struct {
		verb string
		run  func() error
	}{
		{"rename", func() error {
			_, err := store.UpdateTag(bounded, word.ID, collections.TagUpdate{Name: &name}, 0)
			return err
		}},
		{"retire", func() error { _, err := store.ArchiveTag(bounded, word.ID); return err }},
		{"restore", func() error { _, err := store.RestoreTag(bounded, word.ID); return err }},
		{"fold", func() error {
			_, err := store.MergeTags(bounded, other.ID, word.ID, 0)
			return err
		}},
	} {
		t.Run(c.verb, func(t *testing.T) {
			if err := c.run(); !errors.Is(err, apperrors.ErrPermissionDenied) {
				t.Errorf("a bounded seat holding tag.update could %s a word → %v, "+
					"want permission denied — a vocabulary write reaches every "+
					"record carrying the word, including ones this seat cannot see",
					c.verb, err)
			}
		})
	}
}

// The grant half of the same gate. It predates this change and nothing held
// it: every case above and below holds the SCOPE, so a gate that dropped the
// object check would have passed all of them.
func TestASeatWithoutTheGrantCannotAuthorTheVocabulary(t *testing.T) {
	e := Setup(t)
	store := collections.NewStore(e.DB())

	word, err := store.CreateTag(e.Admin(), "NoGrantWord", nil, nil)
	if err != nil {
		t.Fatalf("coining the word: %v", err)
	}
	// Unbounded, and holding tag READ only — so a refusal can only come from
	// the grant, which is the mirror of the cases above.
	reader := e.As(e.AdminUser, nil, principal.Permissions{
		Objects:  map[string]principal.ObjectGrant{"tag": {Read: true}},
		RowScope: principal.RowScopeAll,
	})

	name := "NoGrantRenamed"
	if _, err := store.UpdateTag(reader, word.ID,
		collections.TagUpdate{Name: &name}, 0); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("a seat holding only tag.read renamed a word → %v, want permission denied", err)
	}
	if _, err := store.ArchiveTag(reader, word.ID); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("a seat holding only tag.read retired a word → %v, want permission denied", err)
	}
}

// The other half: the gate refuses a SCOPE, not the grant. An unbounded seat
// holding it authors as it always did — without this, a check that refused
// everybody would pass the case above perfectly.
func TestAnUnboundedSeatStillAuthorsTheVocabulary(t *testing.T) {
	e := Setup(t)
	store := collections.NewStore(e.DB())

	word, err := store.CreateTag(e.Admin(), "UnboundedScopeWord", nil, nil)
	if err != nil {
		t.Fatalf("coining the word: %v", err)
	}
	name := "UnboundedScopeRenamed"
	if _, err := store.UpdateTag(e.Admin(), word.ID,
		collections.TagUpdate{Name: &name}, 0); err != nil {
		t.Fatalf("an unbounded seat renaming a word: %v", err)
	}
	if _, err := store.ArchiveTag(e.Admin(), word.ID); err != nil {
		t.Fatalf("an unbounded seat retiring a word: %v", err)
	}
}

// And the narrowing stops at the vocabulary. Tagging a RECORD is a different
// question with its own answer — write authority over the record being tagged
// (#3755) — and a bounded seat's own records are within their honest reach.
func TestABoundedSeatStillTagsItsOwnRecords(t *testing.T) {
	e := Setup(t)
	store := collections.NewStore(e.DB())

	word, err := store.CreateTag(e.Admin(), "BoundedScopeApplies", nil, nil)
	if err != nil {
		t.Fatalf("coining the word: %v", err)
	}
	bounded := e.As(e.AdminUser, nil, boundedTagAuthor())
	// Created BY the bounded seat, so the row is theirs by construction rather
	// than by a seeded owner column somebody has to keep in step.
	mine, err := e.People.CreatePerson(bounded, people.CreatePersonInput{
		FullName: "Own Record", Source: "manual",
	})
	if err != nil {
		t.Fatalf("the bounded seat creating its own record: %v", err)
	}

	if _, err := store.ApplyTag(bounded, word.ID, "person",
		ids.UUID(mine.Id)); err != nil {
		t.Fatalf("a bounded seat tagging its OWN record: %v — the vocabulary "+
			"gate must not reach a per-record write", err)
	}
}
