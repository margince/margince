// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"maps"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestContactCreationPreviewIncludesThePromotedLeadsHistoryAndConsent(t *testing.T) {
	e := integration.Setup(t)
	perms := integration.AdminPerms
	perms.Objects = maps.Clone(perms.Objects)
	perms.Objects["consent_config"] = principal.ObjectGrant{Read: true, Create: true}
	perms.Objects["custom_field"] = principal.ObjectGrant{Read: true}
	ctx := e.As(e.AdminUser, nil, perms)
	actor, ok := principal.Actor(ctx)
	if !ok {
		t.Fatal("missing actor")
	}
	consents := consent.NewStore(e.DB())
	store := e.Contacts.WithStopCarrier(consents).WithSatelliteCarriers(consents, nil)
	lead, _, err := store.CreateLead(ctx, contacts.CreateLeadInput{FullName: new("Existing Buyer"), Email: new("carry@example.test"), Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	leadID := ids.From[ids.LeadKind](ids.UUID(lead.Id))
	activity, _, err := e.Activities.LogActivity(ctx, activities.LogActivityInput{Kind: "note", Subject: new("Buyer discussion"), Source: "manual", Links: []activities.ActivityLinkInput{{EntityType: "lead", EntityID: leadID.UUID}}})
	if err != nil {
		t.Fatal(err)
	}
	purpose, err := consents.CreatePurpose(ctx, "transactional", "Business correspondence", false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := consents.Record(ctx, consent.RecordInput{LeadID: leadID, PurposeID: purpose.ID, NewState: "withdrawn", Source: new("manual"), PolicyText: new("No further correspondence"), PolicyVersion: new("consent-v1")}); err != nil {
		t.Fatal(err)
	}
	in := contacts.CreateContactInput{FullName: "Confirmed Buyer", Source: "manual", Emails: []contacts.ContactEmailInput{{Email: "carry@example.test", EmailType: "work"}}}
	preview, err := store.PreviewContactCreationForActor(ctx, in, actor)
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Promotions) != 1 {
		t.Fatalf("promotions: %+v", preview.Promotions)
	}
	scope := preview.Promotions[0].SourceCarry
	if len(scope.Activities) != 1 || scope.Activities[0].ID != ids.UUID(activity.Id) || scope.Activities[0].AlreadyOnContact {
		t.Fatalf("activity scope: %+v", scope.Activities)
	}
	if len(scope.Consent) != 1 || scope.Consent[0].PurposeID != purpose.ID.UUID || scope.Consent[0].SourceState != "withdrawn" {
		t.Fatalf("consent scope: %+v", scope.Consent)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM activity_link WHERE activity_id=$1 AND lead_id=$2`, activity.Id, leadID); n != 1 {
		t.Fatal("preview moved activity")
	}
	contact, err := store.CreateContact(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM activity_link WHERE activity_id=$1 AND contact_id=$2`, activity.Id, contact.Id); n != 1 {
		t.Fatal("writer did not move activity")
	}
	states, _, err := consents.ContactConsent(ctx, ids.From[ids.ContactKind](ids.UUID(contact.Id)))
	if err != nil || len(states) != 1 || states[0].State != "withdrawn" {
		t.Fatalf("writer consent: %+v,%v", states, err)
	}
}
