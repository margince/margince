// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A list colleagues work from changes only on purpose: against the version
// its author read, never while archived, and with a revision of every change.

import (
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/modules/collections"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestAListChangeAgainstAStaleVersionIsRefusedAndEveryChangeIsARevision(t *testing.T) {
	e := Setup(t)
	store := collections.NewStore(e.DB())
	rep1 := e.As(e.Rep1, []ids.UUID{e.Team1}, listPerms())
	list, err := store.CreateList(rep1, collections.CreateListInput{Name: "Renewals", EntityType: "contact"})
	if err != nil {
		t.Fatal(err)
	}
	purpose := "who renews this quarter"
	changed, err := store.UpdateList(rep1, list.ID, collections.UpdateListInput{Purpose: &purpose, IfVersion: &list.Version})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	stale := "Renewals, again"
	_, err = store.UpdateList(rep1, list.ID, collections.UpdateListInput{Name: &stale, IfVersion: &list.Version})
	if !errors.Is(err, apperrors.ErrVersionSkew) {
		t.Fatalf("a change against the version before %d answered %v, want a version conflict", changed.Version, err)
	}
	if _, err := store.UpdateList(rep1, list.ID, collections.UpdateListInput{Name: &stale}); err == nil {
		t.Fatal("a change naming no version was accepted")
	}
	revisions := e.WsCount(t, `SELECT count(*) FROM list_revision WHERE list_id = $1`, list.ID)
	if revisions != 2 {
		t.Fatalf("%d revisions, want the creation and the one change that landed", revisions)
	}
	recorded := e.WsScalar(t, `SELECT purpose FROM list_revision WHERE list_id = $1 AND version = $2`, list.ID, changed.Version)
	if recorded != purpose {
		t.Fatalf("the revision at version %d says %q, want %q", changed.Version, recorded, purpose)
	}
}

func TestAnArchivedListIsReadOnlyUntilItIsRestored(t *testing.T) {
	e := Setup(t)
	store := collections.NewStore(e.DB())
	rep1 := e.As(e.Rep1, []ids.UUID{e.Team1}, listPerms())
	contact := e.SeedContact(t, "Archived List Member", &e.Rep1)
	list, err := store.CreateList(rep1, collections.CreateListInput{Name: "Old campaign", EntityType: "contact"})
	if err != nil {
		t.Fatal(err)
	}
	archived, err := store.ArchiveList(rep1, list.ID)
	if err != nil {
		t.Fatalf("archive: %v", err)
	}
	name := "Revived"
	if _, err := store.UpdateList(rep1, list.ID, collections.UpdateListInput{Name: &name, IfVersion: &archived.Version}); !errors.Is(err, collections.ErrListArchived) {
		t.Fatalf("renaming an archived list answered %v, want it refused as archived", err)
	}
	add := collections.MemberChange{EntityType: "contact", EntityID: contact, Reason: collections.ReasonChosen}
	if _, err := store.AddMember(rep1, list.ID, add); !errors.Is(err, collections.ErrListArchived) {
		t.Fatalf("adding to an archived list answered %v, want it refused as archived", err)
	}
	if _, err := store.RestoreList(rep1, list.ID); err != nil {
		t.Fatalf("restore: %v", err)
	}
	if _, err := store.AddMember(rep1, list.ID, add); err != nil {
		t.Fatalf("adding after the restore: %v", err)
	}
	if err := store.RemoveMember(rep1, list.ID, add); err != nil {
		t.Fatalf("removing: %v", err)
	}
	if err := store.RemoveMember(rep1, list.ID, add); !errors.Is(err, collections.ErrNotMember) {
		t.Fatalf("removing a record no longer on the list answered %v, want not a member", err)
	}
}
