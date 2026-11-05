// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration_test

// A team's frozen coverage says whether the standing call it reads still
// matches the pipeline, and the team's live exceptions carry the same finding.

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/compose/weekly"
	"github.com/margince/margince/backend/internal/modules/assurance"
	"github.com/margince/margince/backend/internal/modules/forecasting"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// callWeekEnd is a day inside the week teamClock closes.
var callWeekEnd = time.Date(2026, 6, 7, 12, 0, 0, 0, time.UTC)

// standingTeamCall records a call for a team over the month of the closed week
// through the real store.
func standingTeamCall(t *testing.T, e *teamEnv, team ids.UUID) forecasting.Call {
	t.Helper()
	var period forecasting.Period
	if err := database.WithWorkspaceTx(e.leadCtx, e.Pool, func(tx pgx.Tx) error {
		var err error
		period, _, err = compose.ForecastPeriodAt(e.leadCtx, tx, forecasting.PeriodMonth, callWeekEnd)
		return err
	}); err != nil {
		t.Fatalf("resolving the month: %v", err)
	}
	call, err := forecasting.NewStore(compose.InstallationDB(e.Pool)).RecordCall(e.leadCtx, forecasting.NewCall{
		Period: period, Scope: forecasting.Scope{Kind: forecasting.ScopeTeam, ID: &team},
		AmountMinor: 5_000_000, Currency: "EUR",
	})
	if err != nil {
		t.Fatalf("recording the call: %v", err)
	}
	return call
}

// closeHistory gives the team the closed deals a coverage basis needs.
func closeHistory(t *testing.T, e *teamEnv) {
	t.Helper()
	pipeline, open := defaultOpenStages(t, e)
	for range 25 {
		deal := e.SeedDeal(t, "History", pipeline, ids.From[ids.StageKind](open[0]), &e.Rep1)
		e.WsExec(t, `UPDATE deal SET status = 'won', closed_at = '2026-01-15T10:00:00Z' WHERE id = $1`, deal)
	}
}

// raiseCallFinding writes the row the input check raises when the pipeline
// moved under a call.
func raiseCallFinding(t *testing.T, call forecasting.Call, comparison string) {
	t.Helper()
	if _, err := integration.OwnerConn(t).Exec(context.Background(), `INSERT INTO assurance_exception
		(logical_key, type, subject_kind, subject_id, severity, captured_by, observed)
		VALUES ($1, $2, $3, $4, 'medium', 'test', jsonb_build_object('comparison', $5::text, 'deal_ids', '[]'::jsonb))`,
		"forecast_call_moved:"+call.ID.String(), assurance.TypeForecastCallMoved,
		assurance.SubjectKindForecastCall, call.ID, comparison); err != nil {
		t.Fatal(err)
	}
}

func closedMonthCoverage(t *testing.T, e *teamEnv, engine *weekly.Engine) (weekly.TeamReview, weekly.Coverage) {
	t.Helper()
	if _, _, err := engine.AssembleTeamFor(e.leadCtx, e.Team1, "Team One", members(e), teamClock); err != nil {
		t.Fatal(err)
	}
	read, err := engine.LatestTeamReview(e.leadCtx, e.Team1, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range read.Coverage {
		if c.PeriodKind == "month" {
			return read, c
		}
	}
	t.Fatalf("no month coverage in %+v", read.Coverage)
	return read, weekly.Coverage{}
}

func realForecastEngine(e *teamEnv) *weekly.Engine {
	return weekly.NewEngine(e.Pool, identity.NewService(e.Pool)).
		WithForecast(compose.NewWeeklyForecast(forecasting.NewStore(compose.InstallationDB(e.Pool)))).
		WithAttention(compose.WeeklyAttention{})
}

// A call whose pipeline moved is frozen as needing review, and the summary and
// the wire say so; a call that still matches is not.
func TestTheTeamCoverageFreezesThatItsCallNeedsReview(t *testing.T) {
	for _, tc := range []struct {
		name       string
		comparison string
		moved      bool
	}{
		{"the pipeline moved", "moved", true},
		{"the call could not be compared", "limited", false},
		{"the pipeline did not move", "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := setupTeamWeekly(t)
			writeRepWeek(t, e, e.Rep1)
			closeHistory(t, e)
			call := standingTeamCall(t, e, e.Team1)
			if tc.comparison != "" {
				raiseCallFinding(t, call, tc.comparison)
			}

			read, month := closedMonthCoverage(t, e, realForecastEngine(e))

			if month.Basis != "manager_call" {
				t.Fatalf("month coverage %+v, want the manager's call as its basis", month)
			}
			if month.CallNeedsReview == nil || *month.CallNeedsReview != tc.moved {
				t.Errorf("CallNeedsReview = %v, want %v", month.CallNeedsReview, tc.moved)
			}
			if said := strings.Contains(read.Summary, "needs review"); said != tc.moved {
				t.Errorf("summary %q says needs review = %v, want %v", read.Summary, said, tc.moved)
			}
		})
	}
}

// The team's live exceptions hold the finding on the team's own call and not
// the one on another team's.
func TestTheTeamExceptionsHoldItsCallFindingAndNotAnotherTeams(t *testing.T) {
	e := setupTeamWeekly(t)
	writeRepWeek(t, e, e.Rep1)
	engine := realForecastEngine(e)
	if _, _, err := engine.AssembleTeamFor(e.leadCtx, e.Team1, "Team One", members(e), teamClock); err != nil {
		t.Fatal(err)
	}
	ours := standingTeamCall(t, e, e.Team1)
	raiseCallFinding(t, ours, "moved")
	// Another team's call, which this lead's seat may not record against.
	var theirs forecasting.Call
	if err := integration.OwnerConn(t).QueryRow(context.Background(), `
		INSERT INTO forecast_call (period_start, period_end, scope_kind, scope_id, amount_minor, currency,
		                           author_id, captured_by)
		SELECT period_start, period_end, 'team', $1, 1000, 'EUR', $2, 'test' FROM forecast_call WHERE id = $3
		RETURNING id`, e.Team2, e.Rep3, ours.ID).Scan(&theirs.ID); err != nil {
		t.Fatal(err)
	}
	raiseCallFinding(t, theirs, "moved")

	read, err := engine.LatestTeamReview(e.leadCtx, e.Team1, nil)
	if err != nil {
		t.Fatal(err)
	}
	var got []ids.UUID
	for _, f := range read.Attention.Exceptions {
		got = append(got, ids.UUID(f.SubjectId))
	}
	if len(got) != 1 || got[0] != ours.ID {
		t.Errorf("the team's exceptions name subjects %v, want only its own call %v (not %v)", got, ours.ID, theirs.ID)
	}
}

// A team whose frozen roster has all left still reads the findings on its call,
// and no deal finding of anybody else's.
func TestTheTeamExceptionsKeepItsCallWhenNoRosterMemberRemains(t *testing.T) {
	e := setupTeamWeekly(t)
	writeRepWeek(t, e, e.Rep1)
	engine := realForecastEngine(e)
	if _, _, err := engine.AssembleTeamFor(e.leadCtx, e.Team1, "Team One", members(e), teamClock); err != nil {
		t.Fatal(err)
	}
	call := standingTeamCall(t, e, e.Team1)
	raiseCallFinding(t, call, "moved")
	pipeline, open := defaultOpenStages(t, e)
	elsewhere := e.SeedDeal(t, "Another team's deal", pipeline, ids.From[ids.StageKind](open[0]), &e.Rep3)
	if _, err := integration.OwnerConn(t).Exec(context.Background(), `INSERT INTO assurance_exception
		(logical_key, type, subject_kind, subject_id, severity, owner_id, captured_by)
		VALUES ($1, 'close_past', 'deal', $2, 'high', $3, 'test')`,
		"close_past:"+elsewhere.String(), elsewhere, e.Rep3); err != nil {
		t.Fatal(err)
	}
	if _, err := integration.OwnerConn(t).Exec(context.Background(), `DELETE FROM team_membership WHERE team_id = $1`, e.Team1); err != nil {
		t.Fatal(err)
	}

	read, err := engine.LatestTeamReview(e.As(e.AdminUser, nil, integration.AdminPerms), e.Team1, nil)
	if err != nil {
		t.Fatal(err)
	}
	var got []ids.UUID
	for _, f := range read.Attention.Exceptions {
		got = append(got, ids.UUID(f.SubjectId))
	}
	if !read.Attention.Read || len(got) != 1 || got[0] != call.ID {
		t.Errorf("a team with nobody left reads %v (read=%v), want only its call %v", got, read.Attention.Read, call.ID)
	}
}
