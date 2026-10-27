// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"maps"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestPromotionConsentPreviewMatchesTheRecordedOutcome(t *testing.T) {
	for _, operation := range []string{"promotion", "merge"} {
		t.Run(operation, func(t *testing.T) { testConsentCarryPreview(t, operation) })
	}
}

func testConsentCarryPreview(t *testing.T, operation string) {
	for _, example := range []struct{ lead, contact, after string }{
		{"granted", "", "granted"}, {"withdrawn", "granted", "withdrawn"}, {"granted", "withdrawn", "withdrawn"},
	} {
		t.Run(example.lead+"/"+example.contact, func(t *testing.T) {
			e := integration.Setup(t)
			perms := integration.AdminPerms
			perms.Objects = maps.Clone(perms.Objects)
			perms.Objects["consent_config"] = principal.ObjectGrant{Read: true, Create: true}
			perms.Objects["custom_field"] = principal.ObjectGrant{Read: true}
			ctx := e.As(e.AdminUser, nil, perms)
			consents := consent.NewStore(e.DB())
			store := e.Contacts.WithStopCarrier(consents).WithSatelliteCarriers(consents, nil)
			purpose, err := consents.CreatePurpose(ctx, "transactional", "Business correspondence", false)
			if err != nil {
				t.Fatal(err)
			}
			contact, err := store.CreateContact(ctx, contacts.CreateContactInput{FullName: "Dana Buyer", Emails: []contacts.ContactEmailInput{{Email: "dana@example.test", EmailType: "work", IsPrimary: true}}, Source: "manual"})
			if err != nil {
				t.Fatal(err)
			}
			contactID := ids.From[ids.ContactKind](ids.UUID(contact.Id))
			var leadID ids.LeadID
			var sourceID ids.ContactID
			if operation == "promotion" {
				leadID = seedHistoricalLeadAtContactEmail(t, e, ctx,
					contacts.CreateLeadInput{FullName: new("Dana Buyer"), Source: "manual"}, "dana@example.test")
			} else {
				source, err := store.CreateContact(ctx, contacts.CreateContactInput{FullName: "Dana Duplicate", Source: "manual"})
				if err != nil {
					t.Fatal(err)
				}
				sourceID = ids.From[ids.ContactKind](ids.UUID(source.Id))
			}
			record := consent.RecordInput{LeadID: leadID, ContactID: sourceID, PurposeID: purpose.ID, NewState: example.lead, Source: new("manual"), PolicyText: new("Correspondence consent"), PolicyVersion: new("consent-v1")}
			if _, err := consents.Record(ctx, record); err != nil {
				t.Fatal(err)
			}
			if example.contact != "" {
				record.LeadID, record.ContactID, record.NewState = ids.LeadID{}, contactID, example.contact
				if _, err := consents.Record(ctx, record); err != nil {
					t.Fatal(err)
				}
			}
			var changes []contacts.ConsentCarryChange
			if operation == "promotion" {
				preview, err := store.PreviewLeadContactChange(ctx, leadID, contacts.PromoteLeadInput{Trigger: "human_qualify"})
				if err != nil {
					t.Fatal(err)
				}
				changes = preview.Consent
			} else {
				preview, err := store.PreviewMergeSurvivorship(ctx, "contact", sourceID.UUID, contactID.UUID, nil, nil)
				if err != nil {
					t.Fatal(err)
				}
				changes = preview.Consent.States
				if preview.Consent.ProofsRehomed != 1 {
					t.Fatalf("proofs to move: %d", preview.Consent.ProofsRehomed)
				}
			}
			if len(changes) != 1 {
				t.Fatalf("consent scope: %+v", changes)
			}
			change := changes[0]
			if change.PurposeID != purpose.ID.UUID || change.PurposeLabel != "Business correspondence" || change.SourceState != example.lead || change.TargetAfter != example.after {
				t.Fatalf("consent preview: %+v", change)
			}
			if (example.contact == "" && change.TargetBefore != nil) || (example.contact != "" && (change.TargetBefore == nil || *change.TargetBefore != example.contact)) {
				t.Fatalf("before: %+v", change)
			}
			if operation == "promotion" {
				if _, _, err := store.PromoteLead(ctx, leadID, contacts.PromoteLeadInput{Trigger: "human_qualify"}); err != nil {
					t.Fatal(err)
				}
			} else {
				if _, err := store.MergeContact(ctx, sourceID, contactID, nil); err != nil {
					t.Fatal(err)
				}
			}
			states, _, err := consents.ContactConsent(ctx, contactID)
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, state := range states {
				if state.PurposeID == purpose.ID {
					found = true
					if state.State != change.TargetAfter {
						t.Fatalf("recorded state: %+v", state)
					}
				}
			}
			if !found {
				t.Fatal("carried consent missing")
			}
			expected := 1
			if example.contact != "" {
				expected++
			}
			if example.lead == "withdrawn" && example.contact == "granted" {
				expected++
			}
			if operation == "promotion" {
				_, proof, err := consents.LeadConsent(ctx, leadID)
				if err != nil || len(proof) != 1 {
					t.Fatalf("lead proof was moved or duplicated: %+v, %v", proof, err)
				}
			} else {
				_, proof, err := consents.ContactConsent(ctx, contactID)
				if err != nil || len(proof) != expected {
					t.Fatalf("merged proof = %+v, %v; want %d", proof, err, expected)
				}
			}
		})
	}
}
