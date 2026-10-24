// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A Live List's tag clause names the tags a record carries, in the why and in
// the member table alike, and hides them only from a seat without the tag grant.

import (
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/collections"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func contactListPerms(tagRead bool) principal.Permissions {
	p := RepPerms
	p.Objects = map[string]principal.ObjectGrant{
		"contact": {Read: true},
		"list":    {Create: true, Read: true},
		"tag":     {Read: tagRead},
	}
	return p
}

func linkedLabels(linked *[]crmcontracts.ListLinkedValue) []string {
	if linked == nil {
		return nil
	}
	labels := []string{}
	for _, one := range *linked {
		if one.Label != nil {
			labels = append(labels, *one.Label)
		}
	}
	return labels
}

func TestATagClauseNamesTheTagsTheRecordCarries(t *testing.T) {
	e := Setup(t)
	store := collections.NewStore(e.DB())
	wanted, err := store.CreateTag(e.Admin(), "Wanted Tag", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	other, err := store.CreateTag(e.Admin(), "Other Tag", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	bare := e.SeedContact(t, "Carries Nothing", &e.Rep1)
	both := e.SeedContact(t, "Carries Both", &e.Rep1)
	for _, tag := range []ids.TagID{wanted.ID, other.ID} {
		if _, err := store.ApplyTag(e.Admin(), tag, "contact", both); err != nil {
			t.Fatal(err)
		}
	}
	list, err := store.CreateList(e.Admin(), collections.CreateListInput{
		Name: "Wanted contacts", EntityType: "contact", ListType: "dynamic", Sharing: "workspace",
		Definition: map[string]any{"field": "tag", "op": "eq", "value": wanted.ID.String()},
	})
	if err != nil {
		t.Fatal(err)
	}
	reader := e.As(e.Rep1, []ids.UUID{e.Team1}, contactListPerms(true))

	none, err := store.ExplainView(reader, list.ID, bare)
	if err != nil {
		t.Fatal(err)
	}
	if none.Clauses.Hidden != nil || none.Clauses.Linked == nil || len(*none.Clauses.Linked) != 0 {
		t.Fatalf("a record with no tag read %+v, want an empty, not hidden, link", none.Clauses)
	}

	met, err := store.ExplainView(reader, list.ID, both)
	if err != nil {
		t.Fatal(err)
	}
	if got := linkedLabels(met.Clauses.Linked); len(got) != 2 || got[0] != "Other Tag" || got[1] != "Wanted Tag" {
		t.Fatalf("the why named %v, want both tags the record carries", got)
	}
	member := onlyMember(reader, t, store, list.ID, both)
	if got := linkedLabels((*member.Values)["tag"].Linked); len(got) != 2 || got[1] != "Wanted Tag" {
		t.Fatalf("the member table named %v, want the same tags as the why", got)
	}

	blind := e.As(e.Rep1, []ids.UUID{e.Team1}, contactListPerms(false))
	hidden, err := store.ExplainView(blind, list.ID, both)
	if err != nil {
		t.Fatal(err)
	}
	if hidden.Clauses.Hidden == nil || !*hidden.Clauses.Hidden || hidden.Clauses.Linked != nil {
		t.Fatalf("a seat without the tag grant read %+v, want hidden", hidden.Clauses)
	}
}
