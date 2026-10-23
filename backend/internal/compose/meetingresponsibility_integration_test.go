// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// Whose meeting it is (storekit.MeetingResponsibilitySQL) and who is in it
// (activities.MeetingRoomColumns), against rows the real writers produced.

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/attention"
	"github.com/margince/margince/backend/internal/compose/company360"
	"github.com/margince/margince/backend/internal/compose/contact360"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/comms"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// bookHostedMeeting logs one meeting hosted by a named seat and adds the
// extra participant rows a capture or a copy would have written.
func bookHostedMeeting(t *testing.T, e *integration.Env, subject string, at time.Time, host ids.UUID, extra map[ids.UUID]string) {
	t.Helper()
	hostID := ids.From[ids.UserKind](host)
	row, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
		Kind: "meeting", Subject: &subject, OccurredAt: &at, Source: "manual", HostUserID: &hostID,
	})
	if err != nil {
		t.Fatalf("booking %q: %v", subject, err)
	}
	owner := integration.OwnerConn(t)
	for seat, role := range extra {
		query := `INSERT INTO activity_participant (activity_id, user_id, role) VALUES ($1, $2, $3)`
		args := []any{row.Id, seat, role}
		if role == "importer" {
			query = `INSERT INTO capture_import (activity_id, user_id) VALUES ($1, $2)`
			args = args[:2]
		}
		if _, err := owner.Exec(context.Background(), query, args...); err != nil {
			t.Fatalf("stamping %s on %q: %v", role, subject, err)
		}
	}
}

func myMeetingsToday(t *testing.T, e *integration.Env, reader ids.UUID) []string {
	t.Helper()
	lane := attentionMeetings{store: activities.NewStore(InstallationDB(e.Pool))}
	rows, err := lane.Today(e.As(reader, nil, integration.AdminPerms), laneDay,
		laneDay.Truncate(24*time.Hour).Add(24*time.Hour), 12, attention.TasksMine, ids.UUID{})
	if err != nil {
		t.Fatalf("reading the meeting lane: %v", err)
	}
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.Subject)
	}
	return out
}

func TestOnlyTakingPartMakesAMeetingYours(t *testing.T) {
	e := integration.Setup(t)
	bookHostedMeeting(t, e, "Quarterly review", laneDay.Add(2*time.Hour), e.Rep1, map[ids.UUID]string{
		e.Rep2: "importer",
		e.Rep3: "cc",
	})
	bookHostedMeeting(t, e, "Workshop", laneDay.Add(4*time.Hour), e.Rep1, map[ids.UUID]string{
		e.Rep3: "attendee",
	})

	if got := myMeetingsToday(t, e, e.Rep1); fmt.Sprint(got) != "[Quarterly review Workshop]" {
		t.Errorf("the host's meetings = %v, want both", got)
	}
	if got := myMeetingsToday(t, e, e.Rep2); len(got) != 0 {
		t.Errorf("the seat that only imported a copy got %v, want none", got)
	}
	if got := myMeetingsToday(t, e, e.Rep3); fmt.Sprint(got) != "[Workshop]" {
		t.Errorf("the copied-then-attending seat got %v, want only the meeting they attend", got)
	}
}

func TestTheCompanyAndContactPagesNameTheSameRoom(t *testing.T) {
	e := integration.Setup(t)
	clock := func() time.Time { return laneDay }
	company := e.SeedCompany(t, "Halden Werke", nil)
	guest := e.SeedContact(t, "Greta Guest", nil)
	cc := e.SeedContact(t, "Carl Copied", nil)
	if _, err := integration.OwnerConn(t).Exec(context.Background(), `INSERT INTO relationship (kind, contact_id, company_id, source, captured_by)
		VALUES ('employment', $1, $2, 'manual', 'human:x')`, guest, company); err != nil {
		t.Fatal(err)
	}
	subject := "Rollout planning"
	at := laneDay.Add(48 * time.Hour)
	row, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
		Kind: "meeting", Subject: &subject, OccurredAt: &at, Source: "manual",
		Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: guest}},
	})
	if err != nil {
		t.Fatalf("booking: %v", err)
	}
	owner := integration.OwnerConn(t)
	for contact, role := range map[ids.UUID]string{guest: "attendee", cc: "cc"} {
		if _, err := owner.Exec(context.Background(),
			`INSERT INTO activity_participant (activity_id, contact_id, role) VALUES ($1, $2, $3)`, row.Id, contact, role); err != nil {
			t.Fatal(err)
		}
	}

	companyPage, err := company360.NewService(e.Pool, e.Contacts, e.Deals, e.Projects,
		approvals.NewService(e.DB()), clock).Assemble(e.Admin(), ids.From[ids.CompanyKind](company))
	if err != nil {
		t.Fatalf("assembling the company: %v", err)
	}
	contactPage, err := contact360.NewService(e.Pool, e.Contacts, e.Deals, e.Projects,
		consent.NewStore(InstallationDB(e.Pool)),
		comms.NewStore(InstallationDB(e.Pool), clock, activities.NewStore(InstallationDB(e.Pool))),
		ai.NewFeedbackStore(InstallationDB(e.Pool)), clock).Assemble(e.Admin(), ids.From[ids.ContactKind](guest))
	if err != nil {
		t.Fatalf("assembling the contact: %v", err)
	}
	if companyPage.NextMeeting == nil || contactPage.NextMeeting == nil || contactPage.NextMeeting.Participants == nil {
		t.Fatalf("next meeting: company %+v, contact %+v", companyPage.NextMeeting, contactPage.NextMeeting)
	}
	var onCompany, onContact []string
	for _, p := range companyPage.NextMeeting.Participants {
		onCompany = append(onCompany, p.DisplayName)
	}
	for _, p := range *contactPage.NextMeeting.Participants {
		onContact = append(onContact, p.FullName)
	}
	if fmt.Sprint(onCompany) != fmt.Sprint(onContact) || len(onCompany) == 0 {
		t.Errorf("the company page names %v and the contact page %v for one meeting", onCompany, onContact)
	}
}
