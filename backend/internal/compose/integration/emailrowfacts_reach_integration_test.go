// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestEmailAudienceReachSkipsOneStatementForNonTeamPages(t *testing.T) {
	e := Setup(t)
	author := onFullSeat(e.As(e.Rep1, []ids.UUID{e.Team1}, activityLifecyclePerms))
	contact := e.SeedContact(t, "Audience cost contact", &e.Rep1)
	id := logEmailActivity(author, t, e, contact, "Audience cost message", "Recorded content")
	counts := map[string]int64{}
	for _, audience := range []string{"workspace", "participants", "selected", "workspace"} {
		input := activities.SetAudienceInput{Audience: audience}
		if audience == "selected" {
			input.Members = []activities.AudienceMember{{SubjectType: "user", SubjectID: e.Rep3}}
		}
		if _, err := e.Activities.SetAudience(author, id, input); err != nil {
			t.Fatal(err)
		}
		rows, cost := audienceTimeline(author, t, e, contact)
		if len(rows) != 1 || rows[0].EmailSummary == nil {
			t.Fatalf("timeline=%+v", rows)
		}
		want := crmcontracts.EmailAccessStatus(audience)
		if got := rows[0].EmailSummary.DisplayStatus; got != want {
			t.Fatalf("audience %s: badge=%s", audience, got)
		}
		if previous, seen := counts[audience]; seen && previous != cost {
			t.Fatalf("repeat %s count=%d, first=%d", audience, cost, previous)
		}
		counts[audience] = cost
	}
	if counts["participants"] == 0 || counts["selected"] == 0 {
		t.Fatalf("meter did not observe reads: %v", counts)
	}
	for _, audience := range []string{"participants", "selected"} {
		if counts[audience]+1 != counts["workspace"] {
			t.Fatalf("want one fewer statement for %s: %v", audience, counts)
		}
	}
}

func TestMixedEmailAudienceBadgesMatchPresentation(t *testing.T) {
	e := Setup(t)
	author := onFullSeat(e.As(e.Rep1, []ids.UUID{e.Team1}, activityLifecyclePerms))
	colleague := onFullSeat(e.As(e.Rep2, []ids.UUID{e.Team1}, activityLifecyclePerms))
	contact := e.SeedContact(t, "Mixed audiences contact", &e.Rep1)
	want := map[ids.UUID]crmcontracts.EmailAccessStatus{}
	for _, audience := range []string{"workspace", "participants", "selected"} {
		id := logEmailActivity(author, t, e, contact, audience+" message", "Recorded content")
		input := activities.SetAudienceInput{Audience: audience}
		if audience == "selected" {
			input.Members = []activities.AudienceMember{{SubjectType: "user", SubjectID: e.Rep3}}
		}
		if _, err := e.Activities.SetAudience(author, id, input); err != nil {
			t.Fatal(err)
		}
		want[id.UUID] = crmcontracts.EmailAccessStatus(audience)
	}
	for readerIndex, reader := range []context.Context{author, colleague} {
		rows, _ := audienceTimeline(reader, t, e, contact)
		if len(rows) != len(want) {
			t.Fatalf("timeline has %d rows, want %d", len(rows), len(want))
		}
		for _, row := range rows {
			presentation, err := e.Activities.GetEmailPresentation(reader, ids.From[ids.ActivityKind](ids.UUID(row.Id)), nil)
			if err != nil {
				t.Fatal(err)
			}
			expected := want[ids.UUID(row.Id)]
			if readerIndex == 1 && expected != crmcontracts.EmailAccessStatusWorkspace {
				expected = crmcontracts.EmailAccessStatusWithheld
			}
			if presentation.Access.DisplayStatus != expected {
				t.Fatalf("reader %d badge=%s, want %s", readerIndex, presentation.Access.DisplayStatus, expected)
			}
			if row.EmailSummary == nil || row.EmailSummary.DisplayStatus != presentation.Access.DisplayStatus {
				t.Fatalf("row=%+v presentation=%+v", row.EmailSummary, presentation.Access)
			}
			if presentation.Access.DisplayStatus == crmcontracts.EmailAccessStatusWithheld && row.EmailSummary.Subject != nil {
				t.Fatal("withheld summary exposes a subject")
			}
		}
	}
}

func audienceTimeline(reader context.Context, t *testing.T, e *Env, contact ids.UUID) ([]crmcontracts.Activity, int64) {
	t.Helper()
	ctx, meter := database.WithStatementMeter(reader)
	rows, _, err := e.Activities.ListActivities(ctx, activities.ListActivitiesInput{
		EntityType: new("contact"), EntityID: &contact,
	})
	if err != nil {
		t.Fatal(err)
	}
	return rows, meter.Cost().Statements
}
