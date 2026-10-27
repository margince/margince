// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

import (
	"fmt"
	"testing"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestPromotionContactPreviewMatchesTheCreatedContact(t *testing.T) {
	e := setupPromoteConsent(t)
	lead, _, err := e.store.CreateLead(e.ctx, CreateLeadInput{
		FullName: new("Dana Buyer"), Email: new("dana@example.test"),
		Phone: new("+84912345678"), Title: new("Director"), Source: "manual",
		OwnerID: new(ids.From[ids.UserKind](e.user)),
	})
	if err != nil {
		t.Fatal(err)
	}
	id := ids.From[ids.LeadKind](ids.UUID(lead.Id))
	change, err := e.store.PreviewLeadContactChange(e.ctx, id, PromoteLeadInput{Trigger: "human_qualify"})
	if err != nil {
		t.Fatal(err)
	}
	if change.Create == nil || !change.ContactID.IsZero() {
		t.Fatalf("expected new contact: %+v", change)
	}
	preview := *change.Create
	contact, merged, err := e.store.PromoteLead(e.ctx, id, PromoteLeadInput{Trigger: "human_qualify"})
	if err != nil || merged {
		t.Fatalf("promotion: merged=%t, %v", merged, err)
	}
	if contact.FullName != preview.FullName || contact.Title == nil || preview.Title == nil || *contact.Title != *preview.Title {
		t.Fatalf("contact identity differs: %+v, %+v", contact, preview)
	}
	if contact.OwnerId == nil || preview.OwnerID == nil || ids.UUID(*contact.OwnerId) != preview.OwnerID.UUID || preview.ConvertedFromLeadID == nil || *preview.ConvertedFromLeadID != id.UUID {
		t.Fatalf("contact owner or lineage differs: %+v, %+v", contact, preview)
	}
	if contact.Emails == nil || len(*contact.Emails) != 1 || len(preview.Emails) != 1 || string((*contact.Emails)[0].Email) != preview.Emails[0].Email || (*contact.Emails)[0].IsPrimary != preview.Emails[0].IsPrimary {
		t.Fatalf("contact emails differ: %+v, %+v", contact.Emails, preview.Emails)
	}
	if contact.Phones == nil || len(*contact.Phones) != 1 || len(preview.Phones) != 1 || (*contact.Phones)[0].Phone != preview.Phones[0].Phone || (*contact.Phones)[0].IsPrimary != preview.Phones[0].IsPrimary {
		t.Fatalf("contact phones differ: %+v, %+v", contact.Phones, preview.Phones)
	}
}

func TestPromotionContactPreviewMatchesTheFillOnlyMerge(t *testing.T) {
	for _, held := range []bool{false, true} {
		t.Run(fmt.Sprintf("existing=%t", held), func(t *testing.T) {
			e := setupPromoteConsent(t)
			in := CreateContactInput{FullName: "Existing buyer", Emails: []ContactEmailInput{{Email: "merge@example.test", EmailType: "work", IsPrimary: true}}, Source: "manual"}
			if held {
				in.Title = new("Original title")
				in.Phones = []ContactPhoneInput{{Phone: "+84912345678", PhoneType: "work", IsPrimary: true}}
			}
			existing, err := e.store.CreateContact(e.ctx, in)
			if err != nil {
				t.Fatal(err)
			}
			id := seedHistoricalLeadWithEmail(e.ctx, t, e.store, CreateLeadInput{
				FullName: new("Incoming buyer"), Phone: new("+84912345678"), Title: new("Director"), Source: "manual",
			}, "merge@example.test")
			change, err := e.store.PreviewLeadContactChange(e.ctx, id, PromoteLeadInput{Trigger: "human_qualify"})
			if err != nil {
				t.Fatal(err)
			}
			if change.Create != nil || change.ContactID.UUID != ids.UUID(existing.Id) || change.Patch == nil {
				t.Fatalf("merge preview: %+v", change)
			}
			_, changesTitle := change.Patch.After()["title"]
			wantPhones := 1
			if held {
				wantPhones = 0
			}
			if changesTitle == held || len(change.Phones) != wantPhones {
				t.Fatalf("fill-only preview: %+v, phones %+v", change.Patch.After(), change.Phones)
			}
			current, err := e.store.GetContact(e.ctx, change.ContactID, storekit.LiveOnly)
			if err != nil {
				t.Fatal(err)
			}
			if current.Version == nil || int64(*current.Version) != change.ContactVersion || *current.Version != *existing.Version {
				t.Fatalf("preview changed the contact: %+v", current)
			}
			contact, merged, err := e.store.PromoteLead(e.ctx, id, PromoteLeadInput{Trigger: "human_qualify"})
			if err != nil || !merged {
				t.Fatalf("promotion: merged=%t, %v", merged, err)
			}
			wantTitle := "Director"
			if held {
				wantTitle = "Original title"
			}
			if contact.FullName != "Existing buyer" || contact.Title == nil || *contact.Title != wantTitle || contact.Phones == nil || len(*contact.Phones) != 1 || (*contact.Phones)[0].Phone != "+84912345678" || (*contact.Phones)[0].IsPrimary != held {
				t.Fatalf("merge differs from preview: %+v", contact)
			}
		})
	}
}
