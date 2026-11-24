// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"context"
	"maps"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// A meeting whose subject names travel logistics owes no outcome, in each
// language, unless it links a deal, lead or project. A business subject that
// shares a word with travel still owes one. The outcome filter, the list stamp
// and the single read give the same answer for every subject.
func TestATravelSubjectOwesNoOutcomeUnlessItLinksADealLeadOrProject(t *testing.T) {
	e := setupSend(t)
	helper := seedCustomer(t, e)
	pipeline, stage, deal, lead, company, project := ids.NewV7(), ids.NewV7(), ids.NewV7(), ids.NewV7(), ids.NewV7(), ids.NewV7()
	for _, seed := range []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO pipeline (id, name) VALUES ($1, $2)`, []any{pipeline, "Pipeline " + pipeline.String()}},
		{`INSERT INTO stage (id, pipeline_id, name, "position") VALUES ($1, $2, 'Qualified', 1)`, []any{stage, pipeline}},
		{`INSERT INTO deal (id, name, status, owner_id, pipeline_id, stage_id, source, captured_by)
			VALUES ($1, 'Airline deal', 'open', $2, $3, $4, 'manual', 'system')`, []any{deal, e.rep, pipeline, stage}},
		{`INSERT INTO lead (id, full_name, status, source, captured_by, owner_id)
			VALUES ($1, 'Airline lead', 'new', 'manual', 'system', $2)`, []any{lead, e.rep}},
		{`INSERT INTO company (id, display_name, source, captured_by) VALUES ($1, 'Airline', 'seed', 'system')`, []any{company}},
		{`INSERT INTO project (id, name, key, company_id, phase, source, captured_by)
			VALUES ($1, 'Lounge rollout', 'LNG1', $2, 'delivering', 'manual', 'system')`, []any{project, company}},
	} {
		if _, err := e.owner.Exec(t.Context(), seed.sql, seed.args...); err != nil {
			t.Fatalf("seeding the linked records: %v", err)
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
		link    string
		owed    bool
	}{
		{"Airport drop-off", "", false},
		{"Pickup from the station", "", false},
		{"Flug nach Hamburg", "", false},
		{"Abholung am Flughafen", "", false},
		{"Đón khách ở sân bay", "", false},
		{"Chuyến bay đi Đà Nẵng", "", false},
		{"Project kickoff", "", true},
		{"Flugzeugbau Kickoff", "", true},
		{"Taxi to the hotel", "", false},
		{"Zugfahrt nach Berlin", "", false},
		{"Đưa đón khách ra ga tàu", "", false},
		{"Knowledge transfer", "", true},
		{"Train the sales team", "", true},
		{"Hotel prospect review", "", true},
		{"Workshop in Zug", "", true},
		{"Đón đoàn khách hàng", "", true},
		{"Airport drop-off", "deal", true},
		{"Airport drop-off", "lead", true},
		{"Airport drop-off", "project", true},
	}
	want := map[ids.ActivityID]bool{}
	names := map[ids.ActivityID]string{}
	for _, c := range cases {
		var links []ActivityLinkInput
		if target, ok := map[string]ids.UUID{"deal": deal, "lead": lead, "project": project}[c.link]; ok {
			links = append(links, ActivityLinkInput{EntityType: c.link, EntityID: target})
		}
		id := log(c.subject, links...)
		want[id], names[id] = c.owed, c.subject+" "+c.link
	}

	assertOwedAsRead(leadReaderCtx(e), t, e, want, names)

	// A reader with no grant on leads cannot see the lead link, so for them the
	// lead-linked drop-off is plain travel. A link they cannot see changes nothing.
	hidden := map[ids.ActivityID]bool{}
	for id, name := range names {
		if name == "Airport drop-off lead" {
			hidden[id] = false
		}
	}
	assertOwedAsRead(meetingCtx(e), t, e, hidden, names)
}

// leadReaderCtx is meetingCtx with a read grant on leads.
func leadReaderCtx(e *sendEnv) context.Context {
	ctx := principal.WithCorrelationID(principal.WithWorkspaceID(context.Background(), e.ws), ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + e.rep.String(), UserID: e.rep,
		Permissions: principal.Permissions{
			RoleKeys: []string{"rep"},
			Objects: map[string]principal.ObjectGrant{
				"activity": {Read: true, Create: true, Update: true},
				"contact":  {Read: true}, "deal": {Read: true}, "lead": {Read: true},
				"company": {Read: true}, "project": {Read: true},
			},
			RowScope: principal.RowScopeAll,
		},
	})
}

// assertOwedAsRead checks the list stamp, the single read and the outcome
// filter against want, each as the reader in ctx.
func assertOwedAsRead(ctx context.Context, t *testing.T, e *sendEnv, want map[ids.ActivityID]bool, names map[ids.ActivityID]string) {
	t.Helper()
	want = maps.Clone(want)
	kind := string(crmcontracts.ActivityKindMeeting)
	all, _, err := meetingStore(e).ListActivities(ctx, ListActivitiesInput{Kind: &kind})
	if err != nil {
		t.Fatalf("listing the meetings: %v", err)
	}
	now := clocktest.Now(t)
	kept, _, err := meetingStore(e).ListActivities(ctx, ListActivitiesInput{Kind: &kind, AwaitingOutcomeAsOf: &now})
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
		one, err := meetingStore(e).GetActivity(ctx, id, storekit.LiveOnly)
		if err != nil {
			t.Fatalf("reading %q: %v", names[id], err)
		}
		if one.AwaitingOutcome == nil || *one.AwaitingOutcome != owed {
			t.Errorf("%q: the single read's awaiting_outcome = %v, want %v", names[id], one.AwaitingOutcome, owed)
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
