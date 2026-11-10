// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// activities-by-kind's population opt-out, over the saved-analytics door
// (RunAnalyticsQuery / specNarrowings) as well as the run_report one —
// report_projectgrant_integration_test.go already covers this key's project
// grant over this same door, with a RowScopeAll reader that never reaches
// the population resolver at all (an all-scope lens skips it). This is the
// other half: a non-all-scope reader, the shape that used to render the
// engine's hardcoded `owner_id = caller` clause against a table that has no
// such column.

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/compose/analyticsquery"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// teamScopedActivityReader is activityReader's RowScopeTeam twin: the tier
// that actually reaches AnalyticsPopulationClause's owner_id resolution,
// which an all-scope reader's lens never does.
// activityReaderAs is teamScopedActivityReader for a named seat.
func (e *forecastEnv) activityReaderAs(seat ids.UUID) context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), e.WS)
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + seat.String(), UserID: seat,
		TeamIDs: []ids.UUID{e.Team1},
		Permissions: principal.Permissions{
			Objects: map[string]principal.ObjectGrant{
				"activity": {Read: true}, "forecast": {Read: true}, "installation_settings": {Read: true},
			},
			RowScope: principal.RowScopeTeam,
		},
	})
}

func (e *forecastEnv) teamScopedActivityReader() context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), e.WS)
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + e.Rep1.String(), UserID: e.Rep1,
		TeamIDs: []ids.UUID{e.Team1},
		Permissions: principal.Permissions{
			Objects: map[string]principal.ObjectGrant{
				"activity": {Read: true}, "forecast": {Read: true}, "installation_settings": {Read: true},
			},
			RowScope: principal.RowScopeTeam,
		},
	})
}

// A team-scoped reader asking activities-by-kind through the saved-analytics
// door must get a real count, not the 500 the population default's
// hardcoded owner_id clause produced against a table with no such column.
func TestActivitiesByKindAnalyticsQueryDoesNotCrashForATeamScopedReader(t *testing.T) {
	e := setupForecast(t)
	// Above analyticsquery.DefaultFloor (5): a count at or below the floor is
	// WITHHELD by design (a separate mechanism from the population crash this
	// test is about), and a withheld answer would read like this one too.
	for range 6 {
		e.seedID(t, `INSERT INTO activity (id, kind, subject, occurred_at, source, captured_by)
			VALUES ($1, 'meeting', 'checking in', now() - interval '1 hour', 'manual', 'human:x')`)
		e.seedID(t, `INSERT INTO activity (id, kind, subject, occurred_at, source, captured_by)
			VALUES ($1, 'email', 'checking in', now() - interval '1 hour', 'manual', 'human:x')`)
	}

	answer, err := e.askAnalytics(e.teamScopedActivityReader(), t, analyticsquery.Query{
		Entity:   "activities-by-kind",
		Measures: []analyticsquery.Measure{{Fn: analyticsquery.CountAll, As: "activities"}},
	})
	if err != nil {
		t.Fatalf("activities-by-kind for a team-scoped reader errored (want a real answer, not the owner_id crash): %v", err)
	}
	if answer.Withheld {
		t.Fatalf("answer withheld: %+v, want it above the floor", answer)
	}
	if len(answer.Rows) != 1 || answer.Rows[0]["activities"] != int64(6) && answer.Rows[0]["activities"] != float64(6) {
		t.Errorf("activities-by-kind for a team-scoped reader = %+v, want the six meetings counted and no email", answer.Rows)
	}
}

// A week already read does not move when somebody answers later. The report
// credits the host the held transition recorded, not the one today's RSVPs
// would name.
func TestAHeldMeetingsCreditDoesNotFollowALaterResponse(t *testing.T) {
	e := setupForecast(t)
	// Above analyticsquery.DefaultFloor, or a withheld answer would read like a
	// wrong one.
	for range 6 {
		meeting := e.seedID(t, `INSERT INTO activity
			(id, kind, subject, occurred_at, source, captured_by, host_user_id, meeting_status)
			VALUES ($1, 'meeting', 'held last week', now() - interval '7 days', 'manual', 'human:x', $2, 'held')`,
			e.Rep3)
		// What the transition recorded. Rep3 holds the row's host column, which
		// is what a reader of today's record would answer.
		e.seedID(t, `INSERT INTO activity_meeting_history
			(id, activity_id, status, scheduled_start, scheduled_precision, actor,
			 source_system, source_id, host_id_at_change, effective_at)
			VALUES ($1, $2, 'held', now() - interval '7 days', 'instant', 'human:x',
			 'manual', $4, $3, now() - interval '7 days')`,
			meeting, e.Rep1, meeting.String())
	}

	// host_user_id narrows rather than groups on this report, so the question is
	// asked once per seat.
	// Six is above the floor, so it is readable. A zero is not: the floor
	// withholds it, which creditedCount reports as withheld rather than as a
	// count of none.
	if recorded := e.creditedCount(t, e.Rep1); recorded != 6 {
		t.Errorf("the host the transition recorded is credited with %d meetings, want 6", recorded)
	}
	if current := e.creditedCount(t, e.Rep3); current == 6 {
		t.Error("the row's current host is credited with all six, so a later response moved a past week")
	}
}

// creditedCount is how many meetings the report credits to one seat, asked by
// that seat. Another colleague's figure is an individual-performance question,
// which the switch withholds, so asking as anybody else would measure the
// switch instead of the credit rule.
func (e *forecastEnv) creditedCount(t *testing.T, seat ids.UUID) int {
	t.Helper()
	answer, err := e.askAnalytics(e.activityReaderAs(seat), t, analyticsquery.Query{
		Entity:   "activities-by-kind",
		Measures: []analyticsquery.Measure{{Fn: analyticsquery.CountAll, As: "activities"}},
		Filters: []analyticsquery.Filter{
			{Field: "host_user_id", Op: analyticsquery.OpEq, Value: seat.String()},
		},
	})
	if err != nil {
		t.Fatalf("asking what %s is credited: %v", seat, err)
	}
	if answer.Withheld {
		// The floor, not an answer. Reporting it as zero would let a caller
		// read "none credited" out of "too few to say".
		return -1
	}
	if len(answer.Rows) == 0 {
		return 0
	}
	count, ok := answer.Rows[0]["activities"].(int64)
	if !ok {
		t.Fatalf("the count for %s is %T, not a number: %+v", seat, answer.Rows[0]["activities"], answer.Rows[0])
	}
	return int(count)
}

// A meeting credits everyone the attendance evidence names, so a report
// filtered to any one of them finds it. Comparing the single seat a listing
// shows answered no for every colleague but that one.
func TestAMeetingIsFoundByEveryColleagueItCredits(t *testing.T) {
	e := setupForecast(t)
	// Above the floor for both seats, or a withheld answer reads like a miss.
	for range 6 {
		meeting := e.seedID(t, `INSERT INTO activity
			(id, kind, subject, occurred_at, source, captured_by, host_user_id, meeting_status)
			VALUES ($1, 'meeting', 'both attended', now() - interval '2 days', 'manual', 'human:x', $2, 'held')`,
			e.Rep1)
		// Rep3 accepted, so the evidence credits them too. Rep1 holds the host
		// column, which is the one seat a listing would show.
		e.seedID(t, `INSERT INTO activity_participant
			(id, activity_id, role, user_id, response)
			VALUES ($1, $2, 'attendee', $3, 'accepted')`, meeting, e.Rep3)
	}

	if host := e.creditedCount(t, e.Rep1); host != 6 {
		t.Errorf("the host is credited with %d meetings, want 6", host)
	}
	if guest := e.creditedCount(t, e.Rep3); guest != 6 {
		t.Errorf("the colleague who accepted is credited with %d, want 6: the filter found only the listing seat",
			guest)
	}
}
