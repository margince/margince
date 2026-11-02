// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The record an email about a task goes to, read off the activity and off
// reply-recipient for the same row, so the verb and the To cannot part.

import (
	"context"
	"maps"
	"net/http"
	"slices"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// leadReader is the room reader with the lead grant as well.
func (s *standingEnv) leadReader() context.Context {
	perms := roomPerms
	perms.Objects = map[string]principal.ObjectGrant{"lead": {Read: true}}
	maps.Copy(perms.Objects, roomPerms.Objects)
	return s.As(s.Rep1, []ids.UUID{s.Team1}, perms)
}

func (s *standingEnv) leadAt(t *testing.T, name, address string) activities.ActivityLinkInput {
	t.Helper()
	lead := SeedIDRow(t, OwnerConn(t), `INSERT INTO lead (id, owner_id, full_name, email, status, source, captured_by)
		VALUES ($1, $2, $3, NULLIF($4, ''), 'contacted', 'manual', 'human:x')`, s.Rep1, name, address)
	return activities.ActivityLinkInput{EntityType: "lead", EntityID: lead}
}

// closeLead disqualifies a lead after a task was filed under it, which is how a
// task comes to sit on a closed lead: a closed lead takes no new links.
func (s *standingEnv) closeLead(t *testing.T, lead activities.ActivityLinkInput) {
	t.Helper()
	s.WsExec(t, `UPDATE lead SET status = 'disqualified', archived_at = now() WHERE id = $1`, lead.EntityID)
}

func (s *standingEnv) companyLink(t *testing.T, name string) activities.ActivityLinkInput {
	t.Helper()
	return activities.ActivityLinkInput{EntityType: "company", EntityID: s.SeedCompany(t, name, &s.Rep1)}
}

// targetOf reads the row's email_target and the To reply-recipient fills.
func (s *standingEnv) targetOf(t *testing.T, row ids.UUID) (*crmcontracts.ActivityEmailTarget, bool, string) {
	t.Helper()
	ctx, h := s.leadReader(), s.replyHandlers()
	code, body := serve(ctx, t, func(w http.ResponseWriter, r *http.Request) { h.GetActivity(w, r, crmcontracts.Id(row)) })
	if code != http.StatusOK {
		t.Fatalf("GET /activities/{id} answered %d", code)
	}
	one := readBody[crmcontracts.Activity](t, body)
	return one.EmailTarget, one.OffersEmail, filledTo(ctx, t, h, row)
}

func TestATaskEmailsItsContactElseItsLiveLeadElseItsCompany(t *testing.T) {
	s := setupStanding(t)
	type want struct {
		kind    crmcontracts.ActivityEmailTargetEntityType
		id      ids.UUID
		refusal crmcontracts.ActivityEmailTargetRefusal
		to      string
	}
	contact := s.contactAt(t, "Anna Weber", "anna@buyer.test")
	live := s.leadAt(t, "Lena Lead", "lena@prospect.test")
	other := s.leadAt(t, "Lars Lead", "lars@prospect.test")
	closed := s.leadAt(t, "Old Lead", "old@prospect.test")
	bare := s.leadAt(t, "Nora Noaddress", "")
	company := s.companyLink(t, "Acme")
	// Every task is filed first, then the closed lead is closed.
	rows := map[string]ids.UUID{}
	type taskCase struct {
		name  string
		links []activities.ActivityLinkInput
		want  *want
	}
	var cases []taskCase
	for _, tc := range []taskCase{
		{
			"a contact",
			[]activities.ActivityLinkInput{contact, live, company},
			&want{crmcontracts.ActivityEmailTargetEntityTypeContact, contact.EntityID, "", "anna@buyer.test"},
		},
		{
			"one live lead",
			[]activities.ActivityLinkInput{live},
			&want{crmcontracts.ActivityEmailTargetEntityTypeLead, live.EntityID, "", "lena@prospect.test"},
		},
		{
			"a live and a closed lead, and a company",
			[]activities.ActivityLinkInput{live, closed, company},
			&want{crmcontracts.ActivityEmailTargetEntityTypeLead, live.EntityID, "", "lena@prospect.test"},
		},
		{"two live leads", []activities.ActivityLinkInput{live, other, company}, nil},
		{
			"a closed lead and no company",
			[]activities.ActivityLinkInput{closed},
			&want{crmcontracts.ActivityEmailTargetEntityTypeLead, closed.EntityID, crmcontracts.ActivityEmailTargetRefusalLeadClosed, ""},
		},
		{
			"a lead with no address",
			[]activities.ActivityLinkInput{bare},
			&want{crmcontracts.ActivityEmailTargetEntityTypeLead, bare.EntityID, crmcontracts.ActivityEmailTargetRefusalNoAddress, ""},
		},
		{
			"a closed lead and a company",
			[]activities.ActivityLinkInput{closed, company},
			&want{crmcontracts.ActivityEmailTargetEntityTypeCompany, company.EntityID, "", ""},
		},
		{
			"a company",
			[]activities.ActivityLinkInput{company},
			&want{crmcontracts.ActivityEmailTargetEntityTypeCompany, company.EntityID, "", ""},
		},
	} {
		rows[tc.name] = s.writtenAbout(t, "task", tc.links...)
		cases = append(cases, tc)
	}
	s.closeLead(t, closed)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			target, offers, to := s.targetOf(t, rows[tc.name])
			if tc.want == nil {
				if target != nil || offers || to != "" {
					t.Errorf("target = %+v, offers %v, To %q; want none", target, offers, to)
				}
				return
			}
			if target == nil {
				t.Fatalf("no target, want %+v", *tc.want)
			}
			refusal := crmcontracts.ActivityEmailTargetRefusal("")
			if target.Refusal != nil {
				refusal = *target.Refusal
			}
			got := want{target.EntityType, ids.UUID(target.EntityId), refusal, to}
			if got != *tc.want {
				t.Errorf("got %+v, want %+v", got, *tc.want)
			}
			if offers != (to != "") {
				t.Errorf("offers_email = %v while To is %q", offers, to)
			}
		})
	}
}

// A caller without the lead grant is told about no lead.
func TestALeadTheReaderMayNotReadIsNoTarget(t *testing.T) {
	s := setupStanding(t)
	row := s.writtenAbout(t, "task", s.leadAt(t, "Lena Lead", "lena@prospect.test"))
	ctx, h := s.reader(), s.replyHandlers()
	code, body := serve(ctx, t, func(w http.ResponseWriter, r *http.Request) { h.GetActivity(w, r, crmcontracts.Id(row)) })
	if code != http.StatusOK {
		t.Fatalf("GET /activities/{id} answered %d", code)
	}
	if target := readBody[crmcontracts.Activity](t, body).EmailTarget; target != nil {
		t.Errorf("a reader with no lead grant was given target %+v", *target)
	}
}

// A draft about a task on a lead goes to the lead, by the rule Write email uses.
func TestADraftAboutATaskOnALeadIsAddressedToTheLead(t *testing.T) {
	s := setupStanding(t)
	row := s.writtenAbout(t, "task", s.leadAt(t, "Lena Lead", "lena@prospect.test"))
	if got := draftedTo(s.leadReader(), t, s.replyHandlers(), row); !slices.Equal(got, []string{"lena@prospect.test"}) {
		t.Errorf("a task on a lead drafts to %v, want lena@prospect.test", got)
	}
}
