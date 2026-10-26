// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/contactdraft"
	"github.com/margince/margince/backend/internal/compose/draftcore"
	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Six notices newer than the lead's own message neither push it out of the
// draft nor stand in for it. The lead's timeline keeps every record.
func TestTheLeadDraftQuotesTheLeadNotTheNoticesAfterIt(t *testing.T) {
	e := integration.Setup(t)
	lead, _, err := e.Contacts.CreateLead(e.Admin(), contacts.CreateLeadInput{
		FullName: new("Noticed prospect"), LinkedInURL: new("https://www.linkedin.com/in/noticed-prospect"), Source: "manual",
	})
	if err != nil {
		t.Fatal(err)
	}
	leadID := ids.UUID(lead.Id)
	store := activities.NewStore(e.DB())
	now := clocktest.Now(t)
	inbound := "inbound"
	logMail := func(from, subject, body string, at time.Time) string {
		t.Helper()
		logged, _, err := store.LogActivity(e.Admin(), activities.LogActivityInput{
			Kind: "email", Direction: &inbound, Subject: &subject, Body: &body, OccurredAt: &at, Source: "manual",
			EmailFrom: from, EmailTo: []string{"rep@ws.example"},
			Links: []activities.ActivityLinkInput{{EntityType: leadLinkType, EntityID: leadID}},
		})
		if err != nil {
			t.Fatal(err)
		}
		return logged.Id.String()
	}
	human := logMail("prospect@buyer.example", "Our rollout", "We plan the rollout for spring.", now.AddDate(0, 0, -10))
	notices := map[string]bool{}
	for i := range contactdraft.DraftInputActivities {
		notices[logMail("drive-shares-dm-noreply@google.com", "Shared with you", "A deck was shared with you.",
			now.AddDate(0, 0, -5).Add(time.Duration(i)*time.Hour))] = true
	}

	rows, err := leadCorrespondence{store: store}.ForLead(e.Admin(), ids.LeadID{UUID: leadID}, now)
	if err != nil {
		t.Fatal(err)
	}
	recent := contactdraft.FoldRecent(rows, now, time.UTC)
	quoted := false
	for _, row := range recent {
		if notices[row.ID] {
			t.Errorf("the draft quotes a sending system's notice %s", row.ID)
		}
		quoted = quoted || row.ID == human
	}
	if !quoted {
		t.Errorf("the lead's own message is missing from the draft's correspondence: %+v", recent)
	}
	if text := strings.Join(draftcore.LanguageEvidenceOf(rows, now, time.UTC), "\n"); !strings.Contains(text, "rollout") || strings.Contains(text, "deck was shared") {
		t.Errorf("the drafting language reads %q, want the lead's words only", text)
	}

	entityType, all := leadLinkType, 50
	history, _, err := store.ListActivities(e.Admin(), activities.ListActivitiesInput{EntityType: &entityType, EntityID: &leadID, Limit: &all})
	if err != nil {
		t.Fatal(err)
	}
	if len(history) != len(notices)+1 {
		t.Errorf("the lead's timeline holds %d records, want all %d", len(history), len(notices)+1)
	}
}

// More rows than the page holds, newer than the lead's message, must not push
// it out. They are a mail withheld from this reader, a note and an email dated ahead,
// and canceled and no-show meetings.
func TestFilteredRowsNewerThanTheLeadsMessageDoNotEmptyTheDraftHistory(t *testing.T) {
	e := integration.Setup(t)
	lead, _, err := e.Contacts.CreateLead(e.Admin(), contacts.CreateLeadInput{
		FullName: new("Crowded prospect"), LinkedInURL: new("https://www.linkedin.com/in/crowded-prospect"), Source: "manual",
	})
	if err != nil {
		t.Fatal(err)
	}
	leadID := ids.UUID(lead.Id)
	store := activities.NewStore(e.DB())
	now := clocktest.Now(t)
	author := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AdminPerms)
	reader := e.As(e.Rep3, []ids.UUID{e.Team2}, integration.AdminPerms)
	links := []activities.ActivityLinkInput{{EntityType: leadLinkType, EntityID: leadID}}
	log := func(in activities.LogActivityInput) crmcontracts.Activity {
		t.Helper()
		in.Source, in.Links = "manual", links
		logged, _, err := store.LogActivity(author, in)
		if err != nil {
			t.Fatal(err)
		}
		return logged
	}
	outbound, subject, body := "outbound", "Our offer", "We sent you the offer on Monday."
	older := now.AddDate(0, 0, -10)
	ours := log(activities.LogActivityInput{
		Kind: "email", Direction: &outbound, Subject: &subject, Body: &body, OccurredAt: &older,
		EmailFrom: "rep@ws.example", EmailTo: []string{"prospect@buyer.example"},
	})

	filtered := 4*contactdraft.DraftInputActivities + 2
	for i := range filtered {
		at := now.AddDate(0, 0, -5).Add(time.Duration(i) * time.Hour)
		switch i % 5 {
		case 0:
			private := "Private note " + strconv.Itoa(i)
			mail := log(activities.LogActivityInput{
				Kind: "email", Direction: &outbound, Subject: &private, Body: &private, OccurredAt: &at,
				EmailFrom: "rep@ws.example", EmailTo: []string{"other@buyer.example"},
			})
			if _, err := store.SetAudience(author, ids.From[ids.ActivityKind](ids.UUID(mail.Id)), activities.SetAudienceInput{Audience: "participants"}); err != nil {
				t.Fatal(err)
			}
		case 1:
			ahead, text := now.AddDate(0, 0, 3+i), "Planned for later"
			log(activities.LogActivityInput{Kind: "note", Body: &text, OccurredAt: &ahead})
		case 2:
			canceled, title := "canceled", "Called-off call"
			log(activities.LogActivityInput{Kind: "meeting", Subject: &title, MeetingStatus: &canceled, OccurredAt: &at})
		case 3:
			noShow, title := "no_show", "Missed call"
			log(activities.LogActivityInput{Kind: "meeting", Subject: &title, MeetingStatus: &noShow, OccurredAt: &at})
		default:
			ahead, later := now.AddDate(0, 0, 3+i), "Scheduled send"
			log(activities.LogActivityInput{
				Kind: "email", Direction: &outbound, Subject: &later, Body: &later, OccurredAt: &ahead,
				EmailFrom: "rep@ws.example", EmailTo: []string{"prospect@buyer.example"},
			})
		}
	}

	rows, err := leadCorrespondence{store: store}.ForLead(reader, ids.LeadID{UUID: leadID}, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Id.String() != ours.Id.String() {
		t.Fatalf("the draft reads %d rows, want only our earlier mail %s", len(rows), ours.Id)
	}
	if recent := contactdraft.FoldRecent(rows, now, time.UTC); len(recent) != 1 || recent[0].Said == "" {
		t.Errorf("the draft folds to %+v, want our earlier words kept", recent)
	}
}
