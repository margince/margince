// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"errors"
	"maps"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestPromotionPreviewNamesMovedAndDuplicateActivityLinks(t *testing.T) {
	e := integration.Setup(t)
	perms := integration.AdminPerms
	perms.Objects = maps.Clone(perms.Objects)
	perms.Objects["custom_field"] = principal.ObjectGrant{Read: true}
	ctx := e.As(e.AdminUser, nil, perms)
	contact, err := e.Contacts.CreateContact(ctx, contacts.CreateContactInput{FullName: "Dana Buyer", Emails: []contacts.ContactEmailInput{{Email: "dana@example.test", EmailType: "work", IsPrimary: true}}, Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	leadID := seedHistoricalLeadAtContactEmail(t, e, ctx,
		contacts.CreateLeadInput{FullName: new("Dana Buyer"), Source: "manual"}, "dana@example.test")
	expected := map[ids.UUID]bool{}
	for _, duplicate := range []bool{false, true} {
		links := []activities.ActivityLinkInput{{EntityType: "lead", EntityID: leadID.UUID}}
		if duplicate {
			links = append(links, activities.ActivityLinkInput{EntityType: "contact", EntityID: ids.UUID(contact.Id)})
		}
		activity, _, err := e.Activities.LogActivity(ctx, activities.LogActivityInput{Kind: "note", Subject: new("Qualification discussion"), Body: new("Buyer confirmed the project scope."), Source: "manual", Links: links})
		if err != nil {
			t.Fatal(err)
		}
		expected[ids.UUID(activity.Id)] = duplicate
	}
	in := contacts.PromoteLeadInput{Trigger: "human_qualify"}
	preview, err := e.Contacts.PreviewLeadContactChange(ctx, leadID, in)
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Activities) != len(expected) {
		t.Fatalf("incomplete activity scope: %+v", preview.Activities)
	}
	for _, link := range preview.Activities {
		duplicate, found := expected[link.ID]
		if !found || duplicate != link.AlreadyOnContact {
			t.Fatalf("wrong activity effect: %+v", link)
		}
	}
	perms.Objects = maps.Clone(perms.Objects)
	perms.Objects["activity"] = principal.ObjectGrant{}
	withheld, err := e.Contacts.PreviewLeadContactChange(e.As(e.AdminUser, nil, perms), leadID, in)
	if !errors.Is(err, apperrors.ErrPermissionDenied) || len(withheld.Activities) != 0 || withheld.Create != nil {
		t.Fatalf("unreadable activity scope: %+v, %v", withheld, err)
	}
	out, merged, err := e.Contacts.PromoteLead(ctx, leadID, in)
	if err != nil || !merged || out.Id != contact.Id {
		t.Fatalf("promotion: %+v, merged=%t, %v", out, merged, err)
	}
	for activity := range expected {
		if n := e.WsCount(t, `SELECT count(*) FROM activity_link WHERE activity_id = @activity AND contact_id = @contact`, pgx.NamedArgs{"activity": activity, "contact": contact.Id}); n != 1 {
			t.Fatalf("contact link count: %d", n)
		}
		if n := e.WsCount(t, `SELECT count(*) FROM activity_link WHERE activity_id = @activity AND lead_id = @lead`, pgx.NamedArgs{"activity": activity, "lead": leadID}); n != 0 {
			t.Fatalf("lead link count: %d", n)
		}
	}
}
