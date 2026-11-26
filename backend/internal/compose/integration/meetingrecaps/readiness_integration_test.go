// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package meetingrecaps_test

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestMeetingRecapWaitsForBothHeldAndNotes(t *testing.T) {
	e := integration.Setup(t)
	now := time.Date(2026, time.July, 15, 14, 0, 0, 0, time.UTC)
	contact, err := e.Contacts.CreateContact(e.Admin(), contacts.CreateContactInput{
		FullName: "Zora Buyer", Source: "manual",
		Emails: []contacts.ContactEmailInput{{Email: "buyer@customer.example", EmailType: "work", IsPrimary: true}},
	})
	if err != nil {
		t.Fatal(err)
	}
	link := activities.ActivityLinkInput{EntityType: "contact", EntityID: ids.UUID(contact.Id)}
	store := activities.NewStore(database.BindTo(e.Pool, ids.From[ids.WorkspaceKind](e.WS))).WithClock(func() time.Time { return now })
	for _, tc := range []struct {
		name, status, body string
		links              []activities.ActivityLinkInput
		edit               *activities.UpdateActivityInput
	}{
		{"notes first", "booked", "Agreed the next steps.", []activities.ActivityLinkInput{link}, &activities.UpdateActivityInput{MeetingStatus: new("held")}},
		{"held first", "held", "\u200b", []activities.ActivityLinkInput{link}, &activities.UpdateActivityInput{Body: new("Agreed the next steps.")}},
		{"canceled", "canceled", "Agreed the next steps.", []activities.ActivityLinkInput{link}, nil},
		{"no customer", "held", "Agreed the next steps.", nil, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := e.Admin()
			row, _, err := store.LogActivity(ctx, activities.LogActivityInput{
				Kind: "meeting", Subject: new("Review"), Body: &tc.body, OccurredAt: new(now.Add(-time.Hour)),
				Source: "manual", Links: tc.links, MeetingStatus: &tc.status,
			})
			if err != nil {
				t.Fatal(err)
			}
			id := ids.From[ids.ActivityKind](ids.UUID(row.Id))
			if ready, err := store.ReadyForRecap(ctx, id, now); ready || err != nil {
				t.Fatalf("initial recap readiness = %v, %v; want false", ready, err)
			}
			if tc.edit == nil {
				return
			}
			if _, err := store.UpdateActivity(ctx, id, *tc.edit); err != nil {
				t.Fatal(err)
			}
			if ready, err := store.ReadyForRecap(ctx, id, now); !ready || err != nil {
				t.Fatalf("recap readiness after update = %v, %v; want true", ready, err)
			}
			if ready, err := store.ReadyForRecap(ctx, id, now.Add(-2*time.Hour)); ready || err != nil {
				t.Fatalf("future meeting recap readiness = %v, %v; want false", ready, err)
			}
		})
	}
}
