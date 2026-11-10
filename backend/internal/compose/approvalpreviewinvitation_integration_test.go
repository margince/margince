// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"fmt"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/shared/kernel/diffhash"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestInvitationApprovalShowsDeliveryAndHostDefaults(t *testing.T) {
	for _, rest := range []bool{false, true} {
		t.Run(fmt.Sprintf("rest=%t", rest), func(t *testing.T) {
			e := integration.Setup(t)
			contact := e.SeedContact(t, "Dana Buyer", &e.AdminUser)
			_, err := e.Activities.SaveSchedulingProfile(e.Admin(), crmcontracts.SchedulingProfile{Provider: "gcal", CalendarId: "primary", DurationMinutes: 30, NoticeMinutes: 120, HorizonDays: 30, Title: "Discovery", VideoCall: new(false), EmailReminder: new(true)})
			if err != nil {
				t.Fatal(err)
			}
			payload := fmt.Sprintf(`{"contact_id":%q,"attendee_email":"buyer@example.test","subject":"Discovery","description":"Discuss the delivery plan.","location":"Office","start":"2027-01-12T09:00:00+07:00","end":"2027-01-12T09:30:00+07:00"}`, contact)
			if rest {
				payload = fmt.Sprintf(`{"operation":"createMeetingInvitation","body":%s}`, payload)
			}
			raw, hash, err := diffhash.Canonical([]byte(payload))
			if err != nil {
				t.Fatal(err)
			}
			svc := NewApprovalsService(e.DB())
			id, err := svc.Stage(e.Admin(), approvals.StageInput{Kind: "invite_meeting", TargetType: "contact", TargetID: contact, ProposedChange: raw, DiffHash: hash})
			if err != nil {
				t.Fatal(err)
			}
			proposal, err := svc.GetWire(e.Admin(), id)
			if err != nil {
				t.Fatal(err)
			}
			preview := proposal.Preview
			if preview == nil || preview.State != "ready" || preview.Details == nil || preview.Effects == nil {
				t.Fatalf("preview: %+v", preview)
			}
			details := map[string]string{}
			for _, detail := range *preview.Details {
				details[detail.Field] = detail.Value.Text
				if detail.Field == "meeting_host" && (detail.Value.EntityId == nil || ids.UUID(*detail.Value.EntityId) != e.AdminUser) {
					t.Fatalf("host: %+v", detail.Value)
				}
			}
			if details["to"] != "buyer@example.test" || details["invitation_description"] != "Discuss the delivery plan." || details["meeting_location"] != "Office" || details["video_call"] != "false" || details["email_reminder"] != "true" {
				t.Fatalf("invitation details: %+v", details)
			}
			if len(*preview.Effects) != 1 || (*preview.Effects)[0] != "authorize_calendar_invitation" {
				t.Fatalf("effects: %+v", *preview.Effects)
			}
			if count := e.WsCount(t, `SELECT count(*) FROM meeting_invitation`); count != 0 {
				t.Fatalf("preview queued %d invitations", count)
			}
		})
	}
}

func TestInvitationApprovalUsesTheAgentsHostAndShowsEveryAttendee(t *testing.T) {
	e := integration.Setup(t)
	e.GrantRole(t, e.Rep1, "admin")
	passport := e.SeedPassport(t, integration.OwnerConn(t), "invitation preview")
	live, err := identity.NewService(e.Pool).AuthenticateAgentByID(sightContext(e), ids.From[ids.PassportKind](passport))
	if err != nil {
		t.Fatal(err)
	}
	ctx := principal.WithActor(sightContext(e), live.Principal())
	contact := e.SeedContact(t, "Guest", &e.Rep1)
	if _, err := e.Activities.SaveSchedulingProfile(ctx, crmcontracts.SchedulingProfile{Provider: "gcal", CalendarId: "primary", DurationMinutes: 30, NoticeMinutes: 120, HorizonDays: 30, Title: "Discovery"}); err != nil {
		t.Fatal(err)
	}
	raw, hash, err := diffhash.Canonical([]byte(fmt.Sprintf(`{"contact_id":%q,"attendee_email":"guest@example.test","attendee_emails":["another-guest@external.test"],"subject":"Discovery","start":"2027-01-12T09:00:00Z","end":"2027-01-12T09:30:00Z"}`, contact)))
	if err != nil {
		t.Fatal(err)
	}
	svc := NewApprovalsService(e.DB())
	id, err := svc.Stage(ctx, approvals.StageInput{Kind: operationInviteMeeting, TargetType: "contact", TargetID: contact, ProposedChange: raw, DiffHash: hash})
	if err != nil {
		t.Fatal(err)
	}
	proposal, err := svc.GetWire(e.Admin(), id)
	if err != nil {
		t.Fatal(err)
	}
	if proposal.Preview == nil || proposal.Preview.State != "ready" || proposal.Preview.Details == nil {
		t.Fatalf("agent invitation preview incomplete: %+v", proposal.Preview)
	}
	host, other := false, false
	for _, detail := range *proposal.Preview.Details {
		if detail.Field == "meeting_host" && detail.Value.EntityId != nil && ids.UUID(*detail.Value.EntityId) == e.Rep1 {
			host = true
		}
		if detail.Value.Text == "another-guest@external.test" {
			other = true
		}
	}
	if !host || !other {
		t.Fatalf("preview omitted the proposing host or further attendee: host=%v attendee=%v", host, other)
	}
	extra, err := e.Contacts.CreateContact(e.Admin(), contacts.CreateContactInput{FullName: "Private guest", Source: "manual", OwnerID: new(ids.From[ids.UserKind](e.Rep1)), Emails: []contacts.ContactEmailInput{{Email: "another-guest@external.test", EmailType: "work", IsPrimary: true}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.Contacts.UpdateContact(e.As(e.Rep1, nil, integration.AdminPerms), ids.From[ids.ContactKind](ids.UUID(extra.Id)), contacts.UpdateContactInput{Visibility: new("owner")}); err != nil {
		t.Fatal(err)
	}
	hidden, err := svc.GetWire(e.Admin(), id)
	if err != nil {
		t.Fatal(err)
	}
	if hidden.Preview == nil || hidden.Preview.State != approvals.PreviewNeedsWiderSight || hidden.Preview.Details != nil {
		t.Fatalf("preview exposed an attendee private to its host: %+v", hidden.Preview)
	}
}
