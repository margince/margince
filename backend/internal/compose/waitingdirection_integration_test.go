// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/attention"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestWaitingDirectionUsesExistingRelationshipAndRelevanceFacts(t *testing.T) {
	for _, tc := range []struct {
		name, label     string
		relationships   []string
		nonBuyer        bool
		contactLinked   bool
		openDeal        bool
		customerContext bool
	}{
		{"unknown buyer", "commitment", nil, false, false, false, false},
		{"supplier", "commitment", []string{"supplier"}, true, false, false, false},
		{"supplier with a deal", "commitment", []string{"supplier"}, false, true, true, false},
		{"supplier contact", "commitment", []string{"supplier"}, true, true, false, false},
		{"customer and supplier contact", "commitment", []string{"customer", "supplier"}, false, true, false, false},
		{"customer and supplier", "commitment", []string{"customer", "supplier"}, false, false, false, false},
		{"newsletter or private message", "noise", []string{"customer"}, true, false, false, false},
		{"supplier contact in customer context", "commitment", []string{"supplier"}, false, true, false, true},
		{"noise with a deal", "noise", []string{"customer"}, true, false, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := integration.Setup(t)
			// Addressed to one seat, so somebody answers for the request.
			var adminAddress string
			if err := integration.OwnerConn(t).QueryRow(context.Background(), "SELECT email FROM app_user WHERE id=$1", e.AdminUser).Scan(&adminAddress); err != nil {
				t.Fatal(err)
			}
			at := time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC)
			company := e.SeedCompany(t, "Example counterpart", &e.Rep1)
			if tc.relationships != nil {
				if _, err := e.Contacts.UpdateCompany(e.Admin(), ids.From[ids.CompanyKind](company), contacts.UpdateCompanyInput{RelationshipTypes: &tc.relationships}); err != nil {
					t.Fatal(err)
				}
			}
			link := activities.ActivityLinkInput{EntityType: "company", EntityID: company}
			if tc.contactLinked {
				contact := e.SeedContact(t, "Example sender", &e.Rep1)
				contactID, companyID, primary := ids.From[ids.ContactKind](contact), ids.From[ids.CompanyKind](company), true
				if _, err := e.Contacts.CreateRelationship(e.Admin(), contacts.CreateRelationshipInput{Kind: "employment", ContactID: &contactID, CompanyID: &companyID, IsCurrentPrimary: &primary}); err != nil {
					t.Fatal(err)
				}
				link = activities.ActivityLinkInput{EntityType: "contact", EntityID: contact}
			}
			links := []activities.ActivityLinkInput{link}
			if tc.customerContext {
				customer := e.SeedCompany(t, "Customer context", &e.Rep1)
				roles := []string{"customer"}
				if _, err := e.Contacts.UpdateCompany(e.Admin(), ids.From[ids.CompanyKind](customer), contacts.UpdateCompanyInput{RelationshipTypes: &roles}); err != nil {
					t.Fatal(err)
				}
				links = append(links, activities.ActivityLinkInput{EntityType: "company", EntityID: customer})
			}
			if tc.openDeal {
				pipeline, open, _ := integration.DealFixture(t, e)
				companyID := ids.From[ids.CompanyKind](company)
				deal, err := e.Deals.CreateDeal(e.Admin(), deals.CreateDealInput{Name: "Example sale", PipelineID: pipeline, StageID: open, CompanyID: &companyID})
				if err != nil {
					t.Fatal(err)
				}
				links = append(links, activities.ActivityLinkInput{EntityType: "deal", EntityID: ids.UUID(deal.Id)})
			}
			subject, body, direction := "Please confirm the schedule", "Please confirm the schedule.", "inbound"
			message, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
				Kind: "email", Subject: &subject, Body: &body, Direction: &direction, OccurredAt: &at,
				Source: "manual", ThreadKey: "schedule@example.test", Links: links, EmailTo: []string{adminAddress},
			})
			if err != nil {
				t.Fatal(err)
			}
			id := ids.UUID(message.Id)
			judge := principal.WithActor(e.Admin(), principal.Principal{Type: principal.PrincipalSystem, ID: "system:owed_verdict", Permissions: principal.Permissions{RowScope: principal.RowScopeAll}})
			if applied, err := e.Activities.SetCaptureLabel(judge, id, tc.label); err != nil || !applied {
				t.Fatalf("label: %t %v", applied, err)
			}
			if applied, err := e.Activities.SetOwedVerdict(judge, id, activities.OwedVerdictAsksUs, "test-rules", at); err != nil || !applied {
				t.Fatalf("request: %t %v", applied, err)
			}
			waiting, cut, err := (attentionWaiting{store: e.Activities, contacts: e.Contacts}).Unanswered(e.Admin(), at.Add(time.Hour), attention.WaitingFor{})
			if err != nil || cut {
				t.Fatalf("waiting: truncated=%t err=%v", cut, err)
			}
			for _, row := range waiting {
				if row.ActivityID == id {
					if tc.label == "noise" {
						t.Fatal("noise became a reply obligation despite its conflicting automatic request verdict")
					}
					if row.NonBuyer != tc.nonBuyer || !row.ConfirmedRequest {
						t.Fatalf("direction changed the request verdict: %+v", row)
					}
					return
				}
			}
			if tc.label != "noise" {
				t.Fatal("the request disappeared rather than retaining its direction")
			}
		})
	}
}
