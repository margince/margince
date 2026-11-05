// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A row on a contact's page names every record it is filed under that the reader
// may see, the same links /activities answers for it.

import (
	"slices"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestANoteOnTwoContactsCarriesBothLinksOnTheContactPageAndTheList(t *testing.T) {
	e := Setup(t)
	mine := e.SeedContact(t, "Anna Weber", &e.Rep1)
	champion := e.SeedContact(t, "Ben Kraus", &e.Rep1)
	hidden := e.SeedContact(t, "Cleo Hahn", &e.Rep3)
	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, roomPerms)

	subject := "Revised quote"
	note, _, err := e.Activities.LogActivity(rep, activities.LogActivityInput{
		Kind: "note", Subject: &subject, Links: []activities.ActivityLinkInput{
			{EntityType: "contact", EntityID: mine}, {EntityType: "contact", EntityID: champion},
		},
	})
	if err != nil {
		t.Fatalf("logging the note: %v", err)
	}
	// Filed on a contact private to Rep3 as well, which neither read may name.
	LinkActivity(t, OwnerConn(t), ids.UUID(note.Id), "contact", hidden)
	e.MakeCapturePrivate(t, objContact, hidden, e.Rep3)
	want := []ids.UUID{mine, champion}

	page, err := contactRoomService(e).Assemble(rep, ids.From[ids.ContactKind](mine))
	if err != nil {
		t.Fatalf("Assemble: %v", err)
	}
	if page.Activities == nil || len(page.Activities.Data) != 1 {
		t.Fatalf("the timeline holds %v, want the one note", page.Activities)
	}
	wantContactLinks(t, "the contact page", page.Activities.Data[0], want)

	contactType := "contact"
	listed, _, err := e.Activities.ListActivities(rep, activities.ListActivitiesInput{EntityType: &contactType, EntityID: &champion})
	if err != nil || len(listed) != 1 {
		t.Fatalf("ListActivities = %d rows, %v; want the one note", len(listed), err)
	}
	wantContactLinks(t, "/activities", listed[0], want)
}

func wantContactLinks(t *testing.T, read string, row crmcontracts.Activity, want []ids.UUID) {
	t.Helper()
	var got []ids.UUID
	if row.Links != nil {
		for _, link := range *row.Links {
			if link.EntityType == crmcontracts.ActivityLinkEntityTypeContact {
				got = append(got, ids.UUID(link.EntityId))
			}
		}
	}
	slices.SortFunc(got, func(a, b ids.UUID) int { return slices.Compare(a[:], b[:]) })
	sorted := slices.Clone(want)
	slices.SortFunc(sorted, func(a, b ids.UUID) int { return slices.Compare(a[:], b[:]) })
	if !slices.Equal(got, sorted) {
		t.Errorf("%s links the note to contacts %v, want %v", read, got, sorted)
	}
}
