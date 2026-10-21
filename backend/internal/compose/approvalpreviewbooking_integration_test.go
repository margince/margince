// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"fmt"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/shared/kernel/diffhash"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestBookingApprovalShowsTheHostTimeAndCompleteFiling(t *testing.T) {
	for _, rest := range []bool{false, true} {
		t.Run(fmt.Sprintf("rest=%t", rest), func(t *testing.T) {
			e := integration.Setup(t)
			first := e.SeedContact(t, "Dana Buyer", &e.AdminUser)
			contact := e.SeedContact(t, "Evan Reviewer", &e.AdminUser)
			payload := fmt.Sprintf(`{"attendee_emails":["buyer@example.test"],"start":"2027-01-12T09:00:00+07:00","end":"2027-01-12T10:00:00+07:00","links":[{"entity_type":"contact","entity_id":%q},{"entity_type":"contact","entity_id":%q},{"entity_type":"contact","entity_id":%q}]}`, first, contact, contact)
			if rest {
				payload = fmt.Sprintf(`{"operation":"bookMeeting","body":%s}`, payload)
			}
			raw, hash, err := diffhash.Canonical([]byte(payload))
			if err != nil {
				t.Fatal(err)
			}
			svc := NewApprovalsService(e.DB())
			id, err := svc.Stage(e.Admin(), approvals.StageInput{Kind: "book_meeting", TargetType: "contact", TargetID: first, ProposedChange: raw, DiffHash: hash})
			if err != nil {
				t.Fatal(err)
			}
			reviewer := e.As(e.Rep1, nil, integration.AdminPerms)
			proposal, err := svc.GetWire(reviewer, id)
			if err != nil {
				t.Fatal(err)
			}
			preview := proposal.Preview
			if preview == nil || preview.State != "ready" || preview.Details == nil {
				t.Fatalf("preview: %+v", preview)
			}
			details := map[string]string{}
			filed := 0
			for _, detail := range *preview.Details {
				details[detail.Field] = detail.Value.Text
				if detail.Field == "filed_under" {
					filed++
				}
				if detail.Field == "meeting_host" && (detail.Value.EntityId == nil || ids.UUID(*detail.Value.EntityId) != e.AdminUser) {
					t.Fatalf("reviewer substituted for host: %+v", detail.Value)
				}
			}
			if details["intended_attendee"] != "buyer@example.test" || filed != 2 || details["subject"] != activities.DefaultMeetingSubject || details["meeting_start"] != "2027-01-12T09:00:00+07:00" || details["meeting_end"] != "2027-01-12T10:00:00+07:00" {
				t.Fatalf("meeting details: %+v, links=%d", details, filed)
			}
			if preview.Effects == nil || len(*preview.Effects) != 1 || (*preview.Effects)[0] != "record_meeting_only" {
				t.Fatalf("effects: %+v", preview.Effects)
			}
			start := time.Date(2027, 1, 12, 9, 0, 0, 0, time.FixedZone("+07", 7*60*60))
			meeting, err := e.Activities.BookMeeting(e.Admin(), activities.BookMeetingInput{AttendeeEmails: []string{"buyer@example.test"}, Host: ids.From[ids.UserKind](e.AdminUser), Start: start, End: start.Add(time.Hour), Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: first}, {EntityType: "contact", EntityID: contact}}})
			if err != nil {
				t.Fatal(err)
			}
			if meeting.Subject == nil || *meeting.Subject != details["subject"] {
				t.Fatalf("recorded subject: %+v", meeting.Subject)
			}
			if count := e.WsCount(t, `SELECT count(*) FROM meeting_invitation`); count != 0 {
				t.Fatalf("record-only booking created %d invitations", count)
			}
		})
	}
}

func TestBookingApprovalRefusesACompanyMeeting(t *testing.T) {
	e := integration.Setup(t)
	company := e.SeedCompany(t, "Harbor", &e.AdminUser)
	raw, hash, err := diffhash.Canonical([]byte(fmt.Sprintf(`{"start":"2027-01-12T09:00:00Z","end":"2027-01-12T10:00:00Z","links":[{"entity_type":"company","entity_id":%q}]}`, company)))
	if err != nil {
		t.Fatal(err)
	}
	svc := NewApprovalsService(e.DB())
	id, err := svc.Stage(e.Admin(), approvals.StageInput{Kind: "book_meeting", TargetType: "company", TargetID: company, ProposedChange: raw, DiffHash: hash})
	if err != nil {
		t.Fatal(err)
	}
	proposal, err := svc.GetWire(e.Admin(), id)
	if err != nil {
		t.Fatal(err)
	}
	if proposal.Preview == nil || proposal.Preview.State != "incomplete" || proposal.Preview.Details != nil {
		t.Fatalf("impossible booking was ready: %+v", proposal.Preview)
	}
}
