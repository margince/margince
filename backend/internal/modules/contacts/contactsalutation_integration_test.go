// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

import (
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestSalutationFiltersKeepAnUnsetChoiceDistinctFromNone(t *testing.T) {
	e := setupPromoteConsent(t)
	for _, choice := range []string{"ms", "none", "unset"} {
		var stored *crmcontracts.Salutation
		if choice != "unset" {
			value := crmcontracts.Salutation(choice)
			stored = &value
		}
		contact, err := e.store.CreateContact(e.ctx, CreateContactInput{FullName: "Contact " + choice, Source: "manual", Salutation: stored, Emails: []ContactEmailInput{{Email: "contact-" + choice + "@example.test", EmailType: "work", IsPrimary: true}}})
		if err != nil {
			t.Fatal(err)
		}
		if (contact.Salutation == nil) != (stored == nil) || (stored != nil && *contact.Salutation != *stored) {
			t.Fatalf("contact choice: %v", contact.Salutation)
		}
		if _, _, err := e.store.CreateLead(e.ctx, CreateLeadInput{FullName: new("Lead " + choice), Email: new("lead-" + choice + "@example.test"), Source: "manual", Salutation: stored}); err != nil {
			t.Fatal(err)
		}
	}
	for _, choice := range []string{"ms", "none", "unset"} {
		contacts, _, err := e.store.ListContacts(e.ctx, ListContactsInput{Salutation: &choice})
		if err != nil || len(contacts) != 1 {
			t.Fatalf("contacts %s: count=%d err=%v", choice, len(contacts), err)
		}
		leads, _, err := e.store.ListLeads(e.ctx, ListLeadsInput{Salutation: &choice})
		if err != nil || len(leads) != 1 {
			t.Fatalf("leads %s: count=%d err=%v", choice, len(leads), err)
		}
	}
}

func TestPromotionCarriesSalutationWithoutOverwritingAnExistingChoice(t *testing.T) {
	for _, existing := range []string{"", "none", "mr"} {
		t.Run("existing-"+existing, func(t *testing.T) {
			e := setupPromoteConsent(t)
			var before *crmcontracts.Salutation
			if existing != "" {
				value := crmcontracts.Salutation(existing)
				before = &value
			}
			contact, err := e.store.CreateContact(e.ctx, CreateContactInput{FullName: "Existing buyer", Source: "manual", Salutation: before, Emails: []ContactEmailInput{{Email: "merge@example.test", EmailType: "work", IsPrimary: true}}})
			if err != nil {
				t.Fatal(err)
			}
			salutation := crmcontracts.SalutationMs
			lead := seedHistoricalLeadWithEmail(e.ctx, t, e.store, CreateLeadInput{FullName: new("Incoming buyer"), Source: "manual", Salutation: &salutation}, "merge@example.test")
			out, merged, err := e.store.PromoteLead(e.ctx, lead, PromoteLeadInput{Trigger: "human_qualify"})
			if err != nil || !merged || out.Id != contact.Id {
				t.Fatalf("promotion: merged=%v err=%v", merged, err)
			}
			want := existing
			if want == "" {
				want = "ms"
			}
			if out.Salutation == nil || string(*out.Salutation) != want {
				t.Fatalf("choice=%v, want %s", out.Salutation, want)
			}
		})
	}
}

func TestPromotionGivesANewContactTheLeadsExplicitSalutation(t *testing.T) {
	e := setupPromoteConsent(t)
	salutation := crmcontracts.SalutationMs
	lead, _, err := e.store.CreateLead(e.ctx, CreateLeadInput{FullName: new("New buyer"), Email: new("new@example.test"), Source: "manual", Salutation: &salutation})
	if err != nil {
		t.Fatal(err)
	}
	out, merged, err := e.store.PromoteLead(e.ctx, ids.From[ids.LeadKind](ids.UUID(lead.Id)), PromoteLeadInput{Trigger: "human_qualify"})
	if err != nil || merged || out.Salutation == nil || *out.Salutation != salutation {
		t.Fatalf("promoted contact=%+v, merged=%v err=%v", out, merged, err)
	}
}
