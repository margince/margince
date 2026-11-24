// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A meeting whose subject names travel logistics owes no outcome, in each
// language, unless it links a deal. The outcome filter and the read's stamp
// give the same answer for every subject.
func TestATravelSubjectOwesNoOutcomeUnlessItLinksADeal(t *testing.T) {
	e := setupSend(t)
	helper := seedCustomer(t, e)
	pipeline, stage, deal := ids.NewV7(), ids.NewV7(), ids.NewV7()
	for _, seed := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO pipeline (id, name) VALUES ($1, $2)`, []any{pipeline, "Pipeline " + pipeline.String()}},
		{`INSERT INTO stage (id, pipeline_id, name, "position") VALUES ($1, $2, 'Qualified', 1)`, []any{stage, pipeline}},
		{`INSERT INTO deal (id, name, status, owner_id, pipeline_id, stage_id, source, captured_by)
			VALUES ($1, 'Airline deal', 'open', $2, $3, $4, 'manual', 'system')`, []any{deal, e.rep, pipeline, stage}},
	} {
		if _, err := e.owner.Exec(t.Context(), seed.sql, seed.args...); err != nil {
			t.Fatalf("seeding the deal: %v", err)
		}
	}
	yesterday := clocktest.Now(t).AddDate(0, 0, -1)
	log := func(subject string, links ...ActivityLinkInput) ids.ActivityID {
		t.Helper()
		out, _, err := meetingStore(e).LogActivity(meetingCtx(e), LogActivityInput{
			Kind: string(crmcontracts.ActivityKindMeeting), Subject: &subject, OccurredAt: &yesterday, Source: "manual",
			Links: append([]ActivityLinkInput{{EntityType: "contact", EntityID: helper}}, links...),
		})
		if err != nil {
			t.Fatalf("logging %q: %v", subject, err)
		}
		return ids.From[ids.ActivityKind](ids.UUID(out.Id))
	}
	cases := []struct {
		subject string
		deal    bool
		owed    bool
	}{
		{"Airport drop-off", false, false},
		{"Pickup from the station", false, false},
		{"Flug nach Hamburg", false, false},
		{"Abholung am Flughafen", false, false},
		{"Đón khách ở sân bay", false, false},
		{"Chuyến bay đi Đà Nẵng", false, false},
		{"Project kickoff", false, true},
		{"Flugzeugbau Kickoff", false, true},
		{"Airport drop-off", true, true},
	}
	want := map[ids.ActivityID]bool{}
	names := map[ids.ActivityID]string{}
	for _, c := range cases {
		var links []ActivityLinkInput
		if c.deal {
			links = append(links, ActivityLinkInput{EntityType: "deal", EntityID: deal})
		}
		id := log(c.subject, links...)
		want[id], names[id] = c.owed, c.subject
	}

	kind := string(crmcontracts.ActivityKindMeeting)
	all, _, err := meetingStore(e).ListActivities(meetingCtx(e), ListActivitiesInput{Kind: &kind})
	if err != nil {
		t.Fatalf("listing the meetings: %v", err)
	}
	now := clocktest.Now(t)
	kept, _, err := meetingStore(e).ListActivities(meetingCtx(e), ListActivitiesInput{Kind: &kind, AwaitingOutcomeAsOf: &now})
	if err != nil {
		t.Fatalf("listing the meetings awaiting an outcome: %v", err)
	}
	filtered := map[ids.ActivityID]bool{}
	for _, row := range kept {
		filtered[ids.From[ids.ActivityKind](ids.UUID(row.Id))] = true
	}
	for _, row := range all {
		id := ids.From[ids.ActivityKind](ids.UUID(row.Id))
		owed, ok := want[id]
		if !ok {
			continue
		}
		if row.AwaitingOutcome == nil || *row.AwaitingOutcome != owed {
			t.Errorf("%q: awaiting_outcome = %v, want %v", names[id], row.AwaitingOutcome, owed)
		}
		if filtered[id] != owed {
			t.Errorf("%q: the outcome filter kept it = %v, want %v", names[id], filtered[id], owed)
		}
		delete(want, id)
	}
	if len(want) != 0 {
		t.Fatalf("%d logged meetings were not listed", len(want))
	}
}

// A meeting with no subject is not travel, so it still owes an outcome.
func TestAMeetingWithNoSubjectStillOwesAnOutcome(t *testing.T) {
	e := setupSend(t)
	helper := seedCustomer(t, e)
	yesterday := clocktest.Now(t).Add(-24 * time.Hour)
	out, _, err := meetingStore(e).LogActivity(meetingCtx(e), LogActivityInput{
		Kind: string(crmcontracts.ActivityKindMeeting), OccurredAt: &yesterday, Source: "manual",
		Links: []ActivityLinkInput{{EntityType: "contact", EntityID: helper}},
	})
	if err != nil {
		t.Fatalf("logging a meeting with no subject: %v", err)
	}
	got, _, err := meetingStore(e).ListActivities(meetingCtx(e), ListActivitiesInput{IDs: []ids.ActivityID{ids.From[ids.ActivityKind](ids.UUID(out.Id))}})
	if err != nil {
		t.Fatalf("reading the meeting: %v", err)
	}
	if len(got) != 1 || got[0].AwaitingOutcome == nil || !*got[0].AwaitingOutcome {
		t.Fatal("a past meeting with no subject no longer owes an outcome")
	}
}
