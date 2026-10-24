// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/contactdraft"
	"github.com/margince/margince/backend/internal/compose/draftcore"
	"github.com/margince/margince/backend/internal/compose/integration"
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

	rows, err := leadCorrespondence{store: store}.ForLead(e.Admin(), ids.LeadID{UUID: leadID})
	if err != nil {
		t.Fatal(err)
	}
	recent := contactdraft.FoldRecent(rows, now)
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
	if text := strings.Join(draftcore.LanguageEvidenceOf(rows, now), "\n"); !strings.Contains(text, "rollout") || strings.Contains(text, "deck was shared") {
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
