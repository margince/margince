// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A Shortlist membership says somebody chose a contact for a purpose, and its
// note says why: the subject's access export carries both, and their erasure
// takes both away.

import (
	"testing"

	"github.com/margince/margince/backend/internal/modules/collections"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestASubjectsShortlistsReachTheirAccessExportAndTheirErasure(t *testing.T) {
	e := Setup(t)
	store := collections.NewStore(e.DB())
	subject := e.SeedContact(t, "Erasable Subject", &e.Rep1)
	list, err := store.CreateList(e.Admin(), collections.CreateListInput{Name: "Speakers", EntityType: "contact"})
	if err != nil {
		t.Fatal(err)
	}
	note := "asked about the keynote"
	if _, err := store.AddMember(e.Admin(), list.ID, collections.MemberChange{
		EntityType: "contact", EntityID: subject, Note: &note, Reason: collections.ReasonChosen,
	}); err != nil {
		t.Fatal(err)
	}

	pkg, err := privacy.AssembleSAR(e.Admin(), e.DB(), ids.From[ids.ContactKind](subject))
	if err != nil {
		t.Fatalf("assemble the access export: %v", err)
	}
	if len(pkg.ListMemberships) != 1 || pkg.ListMemberships[0]["note"] != note {
		t.Fatalf("the export's Shortlists = %v, want the one with its note", pkg.ListMemberships)
	}
	if len(pkg.ListMembershipHistory) != 1 {
		t.Fatalf("the export's Shortlist history = %v, want the one addition", pkg.ListMembershipHistory)
	}

	if err := privacy.NewEraser(e.DB()).EraseContact(e.Admin(), subject, "test"); err != nil {
		t.Fatalf("erase: %v", err)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM list_member WHERE entity_id = $1`, subject); n != 0 {
		t.Fatalf("%d memberships outlived the erasure", n)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM list_member_event WHERE entity_id = $1`, subject); n != 0 {
		t.Fatalf("%d membership events outlived the erasure", n)
	}
}

func TestARemovedMembershipsNoteReachesTheAccessExport(t *testing.T) {
	e := Setup(t)
	store := collections.NewStore(e.DB())
	subject := e.SeedContact(t, "Removed Subject", &e.Rep1)
	list, err := store.CreateList(e.Admin(), collections.CreateListInput{Name: "Panel", EntityType: "contact"})
	if err != nil {
		t.Fatal(err)
	}
	note := "spoke on the panel"
	change := collections.MemberChange{EntityType: "contact", EntityID: subject, Note: &note, Reason: collections.ReasonChosen}
	if _, err := store.AddMember(e.Admin(), list.ID, change); err != nil {
		t.Fatal(err)
	}
	change.Note = nil
	if _, err := store.RemoveMember(e.Admin(), list.ID, change); err != nil {
		t.Fatal(err)
	}

	pkg, err := privacy.AssembleSAR(e.Admin(), e.DB(), ids.From[ids.ContactKind](subject))
	if err != nil {
		t.Fatalf("assemble the access export: %v", err)
	}
	kept := 0
	for _, row := range pkg.ListMembershipHistory {
		if row["action"] == "removed" && row["member_note"] == note {
			kept++
		}
	}
	if kept != 1 {
		t.Fatalf("the export's Shortlist history = %v, want the removal with the note it kept", pkg.ListMembershipHistory)
	}
}
