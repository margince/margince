// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration_test

// The team week's patterns, roster, quality, coverage, funnel and live
// exceptions over real migrated Postgres.

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/compose/weekly"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/forecasting"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// The roster keeps the member whose week could not be read, the reps carry
// their patterns, and the quality figures are measured rather than absent.
func TestTheTeamWeekFreezesItsRosterPatternsAndQuality(t *testing.T) {
	e := setupTeamWeekly(t)
	writeRepWeek(t, e, e.Rep1)

	written, _, err := e.engine.AssembleTeamFor(e.leadCtx, e.Team1, "Team One", members(e), teamClock)
	if err != nil {
		t.Fatal(err)
	}
	read, err := e.engine.LatestTeamReview(e.leadCtx, e.Team1, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(read.Members) != 2 {
		t.Fatalf("the roster holds %d members, want both", len(read.Members))
	}
	for _, m := range read.Members {
		if want := m.UserID == e.Rep1; m.Measured != want {
			t.Errorf("%s reads measured=%v, want %v", m.DisplayName, m.Measured, want)
		}
	}
	if len(read.Reps) != 1 || len(read.Reps[0].Patterns) == 0 ||
		read.Reps[0].Patterns[0].Kind != read.Reps[0].FocusKind {
		t.Fatalf("the measured rep reads %+v, want patterns headed by its focus", read.Reps)
	}
	q := read.Quality
	if q.ProposalsAccepted == nil || q.ProposalsRejected == nil || q.AutoChangeReversals == nil {
		t.Errorf("a freshly frozen week reads quality %+v, want every figure measured", q)
	}
	if read.Summary == "" || read.Summary != written.Summary {
		t.Errorf("the stored summary %q differs from the written %q", read.Summary, written.Summary)
	}
	audited := e.WsScalar(t, `SELECT after->>'summary' FROM audit_log
		WHERE entity_type = 'team_weekly_review' AND entity_id = $1 AND action = 'create'`, read.ID)
	if audited != read.Summary {
		t.Errorf("the audit image carries %q, want the summary the row holds", audited)
	}
}

// A rep frozen before patterns were kept reads its focus as the one pattern.
func TestALegacyRepReadsItsFocusAsItsPattern(t *testing.T) {
	e := setupTeamWeekly(t)
	writeRepWeek(t, e, e.Rep1)
	if _, _, err := e.engine.AssembleTeamFor(e.leadCtx, e.Team1, "Team One", members(e), teamClock); err != nil {
		t.Fatal(err)
	}
	e.WsExec(t, `DELETE FROM team_weekly_review_rep_pattern`)

	read, err := e.engine.LatestTeamReview(e.leadCtx, e.Team1, nil)
	if err != nil {
		t.Fatal(err)
	}
	rep := read.Reps[0]
	if len(rep.Patterns) != 1 || rep.Patterns[0].Kind != rep.FocusKind || rep.Patterns[0].Label != rep.FocusLabel {
		t.Errorf("a legacy rep reads patterns %+v, want its focus %s", rep.Patterns, rep.FocusKind)
	}
}

// A snapshot nobody could be counted in is replaced once a member's week is
// read, and none of its children survive into the replacement.
func TestTheReplacedSnapshotKeepsNoChildOfTheFirst(t *testing.T) {
	e := setupTeamWeekly(t)
	spy := &teamScopeSpy{outlooks: oneTeamHorizon(), coverage: []weekly.Coverage{
		weekly.CoverageRefused("month", "EUR"),
	}}
	engine := weekly.NewEngine(e.Pool, identity.NewService(e.Pool)).WithForecast(spy)
	first, _, err := engine.AssembleTeamFor(e.leadCtx, e.Team1, "Team One", members(e), teamClock)
	if err != nil {
		t.Fatal(err)
	}
	if first.Counts.RepsCounted != 0 || len(first.Members) != 2 {
		t.Fatalf("the first pass counted %d with %d on the roster, want 0 and 2",
			first.Counts.RepsCounted, len(first.Members))
	}

	writeRepWeek(t, e, e.Rep1)
	second, created, err := engine.AssembleTeamFor(e.leadCtx, e.Team1, "Team One", members(e), teamClock)
	if err != nil {
		t.Fatalf("replacing the unmeasured week: %v", err)
	}
	if !created || second.ID != first.ID {
		t.Fatalf("the replacement wrote %v (created=%v), want the same row %v", second.ID, created, first.ID)
	}
	for table, want := range map[string]int{
		"team_weekly_review_member":   2,
		"team_weekly_review_outlook":  1,
		"team_weekly_review_coverage": 1,
		"team_weekly_review_rep":      1,
	} {
		got := e.WsCount(t, `SELECT count(*) FROM `+table+` WHERE team_weekly_review_id = $1`, first.ID)
		if got != want {
			t.Errorf("%s holds %d rows after the replacement, want %d", table, got, want)
		}
	}
}

// The coverage the forecast close hands back is frozen. A horizon at risk
// against a named basis raises the replan flag and names its basis.
func TestTheTeamWeekFreezesItsCoverage(t *testing.T) {
	e := setupTeamWeekly(t)
	writeRepWeek(t, e, e.Rep1)
	month := weekly.Coverage{
		PeriodKind: "month", Basis: "manager_call", ReferenceLandingMinor: 100_000_00,
		NeededOpenMinor: 200_000_00, CurrentOpenMinor: 120_000_00, RemainingToSupportMinor: 60_000_00,
		CoverageBp: 6000, BaseCurrency: "EUR", ReplanRecommended: true,
	}
	spy := &teamScopeSpy{outlooks: oneTeamHorizon(), coverage: []weekly.Coverage{
		month, weekly.CoverageRefused("quarter", "EUR"),
	}}
	engine := weekly.NewEngine(e.Pool, identity.NewService(e.Pool)).WithForecast(spy)
	if _, _, err := engine.AssembleTeamFor(e.leadCtx, e.Team1, "Team One", members(e), teamClock); err != nil {
		t.Fatal(err)
	}
	read, err := engine.LatestTeamReview(e.leadCtx, e.Team1, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(read.Coverage) != 2 || read.Coverage[0] != month || read.Coverage[1].Absent != weekly.CoverageNotAuthorized {
		t.Fatalf("coverage read back as %+v", read.Coverage)
	}
	if !read.ReplanRecommended() {
		t.Error("a month at 60% against a manager's call recommends no replan")
	}
	if !strings.Contains(read.Summary, "coverage at risk against the manager's call") {
		t.Errorf("the summary %q does not name the basis it is at risk against", read.Summary)
	}
}

// A reader without forecast read gets no coverage figure and no summary
// sentence built from one. The call it was measured against needs that grant.
func TestAReaderWithoutForecastReadGetsNoCoverageFigure(t *testing.T) {
	e := setupTeamWeekly(t)
	writeRepWeek(t, e, e.Rep1)
	month := weekly.Coverage{
		PeriodKind: "month", Basis: "manager_call", ReferenceLandingMinor: 100_000_00,
		NeededOpenMinor: 200_000_00, CurrentOpenMinor: 120_000_00, RemainingToSupportMinor: 60_000_00,
		CoverageBp: 6000, BaseCurrency: "EUR", ReplanRecommended: true,
	}
	spy := &teamScopeSpy{outlooks: oneTeamHorizon(), coverage: []weekly.Coverage{month}}
	engine := weekly.NewEngine(e.Pool, identity.NewService(e.Pool)).WithForecast(spy)
	if _, _, err := engine.AssembleTeamFor(e.leadCtx, e.Team1, "Team One", members(e), teamClock); err != nil {
		t.Fatal(err)
	}
	perms := withoutOversight(integration.AdminPerms)
	perms.RoleKeys = []string{"manager"}
	perms.RowScope = principal.RowScopeTeam
	delete(perms.Objects, "forecast")
	lead := e.As(e.Rep1, []ids.UUID{e.Team1}, perms)

	read, err := engine.LatestTeamReview(lead, e.Team1, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(read.Coverage) != 1 || read.Coverage[0].Absent != weekly.CoverageNotAuthorized ||
		read.Coverage[0].ReferenceLandingMinor != 0 || read.Coverage[0].CoverageBp != 0 || read.ReplanRecommended() {
		t.Errorf("a reader without forecast read got coverage %+v", read.Coverage)
	}
	if read.Summary == "" || strings.Contains(read.Summary, "coverage") {
		t.Errorf("the summary %q must still read, without a coverage sentence", read.Summary)
	}
	full, err := engine.LatestTeamReview(e.leadCtx, e.Team1, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(full.Summary, "coverage at risk against the manager's call") {
		t.Errorf("a reader with forecast read lost the coverage sentence: %q", full.Summary)
	}
}

// The real forecast close answers coverage for the month and the quarter. A
// fresh workspace has neither a call nor history, which is an absence.
func TestTheRealForecastCloseAnswersCoverage(t *testing.T) {
	e := setupTeamWeekly(t)
	writeRepWeek(t, e, e.Rep1)
	engine := weekly.NewEngine(e.Pool, identity.NewService(e.Pool)).
		WithForecast(compose.NewWeeklyForecast(forecasting.NewStore(compose.InstallationDB(e.Pool))))
	written, _, err := engine.AssembleTeamFor(e.leadCtx, e.Team1, "Team One", members(e), teamClock)
	if err != nil {
		t.Fatal(err)
	}
	var kinds []string
	for _, c := range written.Coverage {
		kinds = append(kinds, c.PeriodKind)
		if c.Basis != "" || c.Absent == "" || c.ReplanRecommended {
			t.Errorf("%s coverage %+v, want an absence with no replan", c.PeriodKind, c)
		}
	}
	if !slices.Equal(kinds, []string{"month", "quarter"}) {
		t.Errorf("coverage horizons %v, want month and quarter", kinds)
	}
}

// The database refuses a coverage row whose flag does not follow its figures.
func TestTheDatabaseRefusesAMalformedCoverageRow(t *testing.T) {
	e := setupTeamWeekly(t)
	writeRepWeek(t, e, e.Rep1)
	written, _, err := e.engine.AssembleTeamFor(e.leadCtx, e.Team1, "Team One", members(e), teamClock)
	if err != nil {
		t.Fatal(err)
	}
	insert := `INSERT INTO team_weekly_review_coverage
		(team_weekly_review_id, period_kind, basis, absent, reference_landing_minor, needed_open_minor,
		 current_open_minor, remaining_to_support_minor, coverage_bp, base_currency, replan_recommended)
		VALUES ($1, 'month', $2, $3, $4, 1, 1, 1, $5, 'EUR', $6)`
	for _, tc := range []struct {
		name     string
		basis    any
		absent   any
		figure   any
		coverage any
		replan   bool
	}{
		{"a basis with a figure missing", "manager_call", nil, nil, int64(9000), false},
		{"a replan over full coverage", "manager_call", nil, int64(1), int64(9000), true},
		{"no replan under the floor", "manager_call", nil, int64(1), int64(6000), false},
		{"a replan with no basis", nil, "insufficient_basis", nil, nil, true},
		{"both a basis and an absence", "manager_call", "not_authorized", int64(1), int64(9000), false},
		{"an absence with a figure", nil, "not_authorized", nil, int64(9000), false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := e.WsExecErr(t, insert, written.ID, tc.basis, tc.absent, tc.figure, tc.coverage, tc.replan); err == nil {
				t.Error("the database accepted it")
			}
		})
	}
}

// The live findings follow the deal's owner now, not the owner the finding
// was first raised under.
func TestTheTeamsExceptionsFollowTheDealsLiveOwner(t *testing.T) {
	e := setupTeamWeekly(t)
	writeRepWeek(t, e, e.Rep1)
	engine := weekly.NewEngine(e.Pool, identity.NewService(e.Pool)).WithAttention(compose.WeeklyAttention{})
	if _, _, err := engine.AssembleTeamFor(e.leadCtx, e.Team1, "Team One", members(e), teamClock); err != nil {
		t.Fatal(err)
	}
	pipeline, open := defaultOpenStages(t, e)
	cameToTeam := e.SeedDeal(t, "Handed to the team", pipeline, ids.From[ids.StageKind](open[0]), &e.Rep1)
	leftTeam := e.SeedDeal(t, "Handed away", pipeline, ids.From[ids.StageKind](open[0]), &e.Rep3)
	// Findings come from the nightly check, which this suite does not run;
	// the row is the shape that check writes, owner as first raised.
	owner := integration.OwnerConn(t)
	for deal, raisedUnder := range map[ids.UUID]ids.UUID{cameToTeam: e.Rep3, leftTeam: e.Rep1} {
		if _, err := owner.Exec(context.Background(), `INSERT INTO assurance_exception
			(logical_key, type, subject_kind, subject_id, severity, owner_id, captured_by)
			VALUES ($1, 'close_past', 'deal', $2, 'high', $3, 'test')`,
			"close_past:"+deal.String(), deal, raisedUnder); err != nil {
			t.Fatal(err)
		}
	}

	read, err := engine.LatestTeamReview(e.leadCtx, e.Team1, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !read.Attention.Read || len(read.Attention.Exceptions) != 1 {
		t.Fatalf("the team read carries %+v, want the one finding on a deal the team owns now", read.Attention)
	}
	got := read.Attention.Exceptions[0]
	if ids.UUID(got.SubjectId) != cameToTeam || got.OwnerId == nil || ids.UUID(*got.OwnerId) != e.Rep1 {
		t.Errorf("the finding reads subject %v owner %v, want %v owned by Rep1", got.SubjectId, got.OwnerId, cameToTeam)
	}
}

// A deal quiet past the stalled period appears live, quietest first.
func TestTheTeamsStalledDealsAreReadLive(t *testing.T) {
	e := setupTeamWeekly(t)
	writeRepWeek(t, e, e.Rep1)
	engine := weekly.NewEngine(e.Pool, identity.NewService(e.Pool)).WithAttention(compose.WeeklyAttention{})
	if _, _, err := engine.AssembleTeamFor(e.leadCtx, e.Team1, "Team One", members(e), teamClock); err != nil {
		t.Fatal(err)
	}
	pipeline, open := defaultOpenStages(t, e)
	quiet := e.SeedDeal(t, "Gone quiet", pipeline, ids.From[ids.StageKind](open[0]), &e.Rep2)
	busy := e.SeedDeal(t, "Worked today", pipeline, ids.From[ids.StageKind](open[0]), &e.Rep2)
	e.WsExec(t, `UPDATE deal SET last_activity_at = now() - interval '90 days', created_at = now() - interval '120 days' WHERE id = $1`, quiet)

	read, err := engine.LatestTeamReview(e.leadCtx, e.Team1, nil)
	if err != nil {
		t.Fatal(err)
	}
	stale := read.Attention.StaleDeals
	if len(stale) != 1 || stale[0].DealID != quiet || stale[0].OwnerID != e.Rep2 || stale[0].DaysQuiet < 89 {
		t.Errorf("stalled deals %+v, want only %v (not %v) at about 90 days", stale, quiet, busy)
	}
}

// A member who moved to another team takes their live items with them. The
// old team's lead, opening the old week, sees none of the deals they own now.
func TestALiveListIsNarrowedToWhoIsOnTheTeamNow(t *testing.T) {
	e := setupTeamWeekly(t)
	writeRepWeek(t, e, e.Rep1)
	writeRepWeek(t, e, e.Rep2)
	engine := weekly.NewEngine(e.Pool, identity.NewService(e.Pool)).WithAttention(compose.WeeklyAttention{})
	if _, _, err := engine.AssembleTeamFor(e.leadCtx, e.Team1, "Team One", members(e), teamClock); err != nil {
		t.Fatal(err)
	}
	pipeline, open := defaultOpenStages(t, e)
	stayed := e.SeedDeal(t, "Stayed quiet", pipeline, ids.From[ids.StageKind](open[0]), &e.Rep1)
	moved := e.SeedDeal(t, "Went with the rep", pipeline, ids.From[ids.StageKind](open[0]), &e.Rep2)
	e.WsExec(t, `UPDATE deal SET last_activity_at = now() - interval '90 days',
		created_at = now() - interval '120 days' WHERE id = ANY($1)`, []ids.UUID{stayed, moved})
	owner := integration.OwnerConn(t)
	ctx := context.Background()
	if _, err := owner.Exec(ctx, `INSERT INTO assurance_exception
		(logical_key, type, subject_kind, subject_id, severity, owner_id, captured_by)
		VALUES ($1, 'close_past', 'deal', $2, 'high', $3, 'test')`,
		"close_past:"+moved.String(), moved, e.Rep2); err != nil {
		t.Fatal(err)
	}
	if _, err := owner.Exec(ctx, `DELETE FROM team_membership WHERE team_id = $1 AND user_id = $2`,
		e.Team1, e.Rep2); err != nil {
		t.Fatal(err)
	}
	if _, err := owner.Exec(ctx, `INSERT INTO team_membership (team_id, user_id) VALUES ($1, $2)
		ON CONFLICT DO NOTHING`, e.Team2, e.Rep2); err != nil {
		t.Fatal(err)
	}

	read, err := engine.LatestTeamReview(e.leadCtx, e.Team1, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(read.Attention.Exceptions) != 0 {
		t.Errorf("the old team's lead sees %d findings on the moved rep's deals", len(read.Attention.Exceptions))
	}
	stale := read.Attention.StaleDeals
	if len(stale) != 1 || stale[0].DealID != stayed {
		t.Errorf("stalled deals %+v, want only the one owned by a member still on the team", stale)
	}
}

// The funnel credits each move to its owner at the time. A move whose pipeline
// was not recorded counts on neither side.
func TestTheFunnelAttributesEachMoveToItsOwnerThen(t *testing.T) {
	e := setupTeamWeekly(t)
	pipeline, open := defaultOpenStages(t, e)
	entered, next := open[0], open[1]
	// Rep1 converts one of five, Rep2 four of five: 20% and 80% against a
	// team of 50%. The last of Rep2's deals belongs to Rep1 now.
	seedFunnel(t, e, e.Rep1, pipeline, entered, next, 5, 1, e.Rep1)
	seedFunnel(t, e, e.Rep2, pipeline, entered, next, 4, 3, e.Rep2)
	seedFunnel(t, e, e.Rep2, pipeline, entered, next, 1, 1, e.Rep1)
	// A move with no recorded pipeline counts on neither side.
	legacy := e.SeedDeal(t, "Legacy move", pipeline, ids.From[ids.StageKind](entered), &e.Rep1)
	e.WsExec(t, `INSERT INTO deal_stage_history (deal_id, to_stage_id, changed_by, changed_at,
		owner_id_at_change, semantic_at_change) VALUES ($1, $2, 'human:x', $3, $4, 'open')`,
		legacy, entered, teamClock.AddDate(0, 0, -30), e.Rep1)
	seedImportedFunnelMoves(t, e, pipeline, entered, next)
	writeRepWeek(t, e, e.Rep1)
	writeRepWeek(t, e, e.Rep2)

	if _, _, err := e.engine.AssembleTeamFor(e.leadCtx, e.Team1, "Team One", members(e), teamClock); err != nil {
		t.Fatal(err)
	}
	read, err := e.engine.LatestTeamReview(e.leadCtx, e.Team1, nil)
	if err != nil {
		t.Fatal(err)
	}
	// Rep1's sixth entry stayed put by hand and only an import moved it on.
	want := map[ids.UUID]weekly.FunnelDeviation{
		e.Rep1: {RepEntered: 6, RepConverted: 1, TeamEntered: 11, TeamConverted: 5, DeviationBp: -2879},
		e.Rep2: {RepEntered: 5, RepConverted: 4, TeamEntered: 11, TeamConverted: 5, DeviationBp: 3455},
	}
	for _, rep := range read.Reps {
		if len(rep.Funnel) != 1 {
			t.Fatalf("%s reads %d funnel rows, want 1", rep.DisplayName, len(rep.Funnel))
		}
		got, w := rep.Funnel[0], want[rep.UserID]
		if got.StageID != entered || got.RepEntered != w.RepEntered || got.RepConverted != w.RepConverted ||
			got.TeamEntered != w.TeamEntered || got.TeamConverted != w.TeamConverted || got.DeviationBp != w.DeviationBp {
			t.Errorf("%s funnel %+v, want %+v", rep.DisplayName, got, w)
		}
	}
}

// Below five entries a stage is left out.
func TestAThinStageIsNotAFunnelDeviation(t *testing.T) {
	e := setupTeamWeekly(t)
	pipeline, open := defaultOpenStages(t, e)
	seedFunnel(t, e, e.Rep1, pipeline, open[0], open[1], 4, 0, e.Rep1)
	seedFunnel(t, e, e.Rep2, pipeline, open[0], open[1], 6, 6, e.Rep2)
	writeRepWeek(t, e, e.Rep1)
	written, _, err := e.engine.AssembleTeamFor(e.leadCtx, e.Team1, "Team One", members(e), teamClock)
	if err != nil {
		t.Fatal(err)
	}
	if len(written.Reps) != 1 || len(written.Reps[0].Funnel) != 0 {
		t.Errorf("a stage with four entries produced %+v", written.Reps)
	}
}

// An agent's change a member undid counts; a colleague's does not.
func TestTheTeamCountsTheAutomaticChangesItUndid(t *testing.T) {
	e := setupTeamWeekly(t)
	writeRepWeek(t, e, e.Rep1)
	inWeek := teamClock.AddDate(0, 0, -5)
	owner := integration.OwnerConn(t)
	ctx := context.Background()
	for _, writer := range []struct{ kind, id string }{{"agent", "agent:x"}, {"human", "human:" + e.Rep2.String()}} {
		undone, reversal := ids.NewV7(), ids.NewV7()
		if _, err := owner.Exec(ctx, `INSERT INTO audit_log (id, actor_type, actor_id, action, entity_type, entity_id, occurred_at)
			VALUES ($1, $2, $3, 'create', 'deal', $4, $5)`, undone, writer.kind, writer.id, ids.NewV7(), inWeek); err != nil {
			t.Fatal(err)
		}
		if _, err := owner.Exec(ctx, `INSERT INTO audit_log (id, actor_type, actor_id, action, entity_type, entity_id, occurred_at, evidence)
			VALUES ($1, 'human', $2, 'archive', 'deal', $3, $4, jsonb_build_object('undid_audit_log_id', $5::text))`,
			reversal, "human:"+e.Rep1.String(), ids.NewV7(), inWeek.Add(1), undone.String()); err != nil {
			t.Fatal(err)
		}
	}
	written, _, err := e.engine.AssembleTeamFor(e.leadCtx, e.Team1, "Team One", members(e), teamClock)
	if err != nil {
		t.Fatal(err)
	}
	if got := written.Quality.AutoChangeReversals; got == nil || *got != 1 {
		t.Errorf("reversals = %v, want the one agent change", got)
	}
}

// defaultOpenStages seeds the default pipeline and answers its open stages in
// order.
func defaultOpenStages(t *testing.T, e *teamEnv) (ids.PipelineID, []ids.UUID) {
	t.Helper()
	if err := e.Deals.SeedDefaults(e.Admin()); err != nil && !errors.Is(err, apperrors.ErrConflict) {
		t.Fatal(err)
	}
	p, err := e.Deals.DefaultPipeline(e.Admin())
	if err != nil {
		t.Fatal(err)
	}
	stages := slices.Clone(*p.Stages)
	slices.SortFunc(stages, func(a, b crmcontracts.Stage) int { return a.Position - b.Position })
	var open []ids.UUID
	for _, st := range stages {
		if st.Semantic == "open" {
			open = append(open, ids.UUID(st.Id))
		}
	}
	if len(open) < 2 {
		t.Fatalf("the default pipeline has %d open stages, the funnel needs two", len(open))
	}
	return ids.From[ids.PipelineKind](ids.UUID(p.Id)), open
}

// seedFunnel gives `entries` deals an entry into `stage` under `ownerThen`
// inside the window, `converted` of them a later move to `next`, and leaves
// each deal owned by `ownerNow`. Deals come from the real writer; the moves are
// dated by hand, because no writer takes a past instant.
func seedFunnel(
	t *testing.T, e *teamEnv, ownerThen ids.UUID, pipeline ids.PipelineID,
	stage, next ids.UUID, entries, converted int, ownerNow ids.UUID,
) {
	t.Helper()
	at := teamClock.AddDate(0, 0, -30)
	for i := range entries {
		deal := e.SeedDeal(t, "Funnel deal", pipeline, ids.From[ids.StageKind](stage), &ownerNow)
		e.WsExec(t, `INSERT INTO deal_stage_history (deal_id, to_stage_id, changed_by, changed_at,
			owner_id_at_change, pipeline_id_at_change, semantic_at_change)
			VALUES ($1, $2, 'human:x', $3, $4, $5, 'open')`, deal, stage, at, ownerThen, pipeline)
		if i < converted {
			e.WsExec(t, `INSERT INTO deal_stage_history (deal_id, from_stage_id, to_stage_id, changed_by,
				changed_at, owner_id_at_change, pipeline_id_at_change, semantic_at_change)
				VALUES ($1, $2, $3, 'human:x', $4, $5, $6, 'open')`,
				deal, stage, next, at.AddDate(0, 0, 3), ownerThen, pipeline)
		}
	}
}

// seedImportedFunnelMoves gives Rep1 four deals an import touched:
//   - an entry an import brought in, which is no entry;
//   - an entry an import moved on next, which is left out;
//   - an entry whose next move was made by hand in the same instant as an
//     imported move written before it, so the imported move came next and the
//     entry is left out, whatever the two ids say;
//   - an entry a hand move kept in its stage and an import then moved on, which
//     enters but does not convert.
func seedImportedFunnelMoves(t *testing.T, e *teamEnv, pipeline ids.PipelineID, stage, next ids.UUID) {
	t.Helper()
	run := integration.SeedIDRow(t, integration.OwnerConn(t), `INSERT INTO import_run (id, connector, status, source_ref, source, captured_by)
		VALUES ($1, 'csv', 'running', 'deals.csv', 'upload', 'human:test')`)
	at := teamClock.AddDate(0, 0, -30)
	move := `INSERT INTO deal_stage_history (id, deal_id, from_stage_id, to_stage_id, changed_by, changed_at,
		owner_id_at_change, pipeline_id_at_change, semantic_at_change, provenance, import_run_id)
		VALUES ($1, $2, $3, $4, 'human:x', $5, $6, $7, 'open', $8, $9)`
	deal := func(name string) ids.UUID {
		return e.SeedDeal(t, name, pipeline, ids.From[ids.StageKind](stage), &e.Rep1)
	}
	imported := deal("Imported entry")
	e.WsExec(t, move, ids.NewV7(), imported, nil, stage, at, e.Rep1, pipeline, "imported", run)

	movedOn := deal("Moved on by an import")
	e.WsExec(t, move, ids.NewV7(), movedOn, nil, stage, at, e.Rep1, pipeline, "recorded", nil)
	e.WsExec(t, move, ids.NewV7(), movedOn, stage, next, at.AddDate(0, 0, 3), e.Rep1, pipeline, "imported", run)

	sameInstant := deal("Moved twice in one instant")
	e.WsExec(t, move, ids.NewV7(), sameInstant, nil, stage, at, e.Rep1, pipeline, "recorded", nil)
	smaller, larger := ids.NewV7(), ids.NewV7()
	if smaller.String() > larger.String() {
		smaller, larger = larger, smaller
	}
	e.WsExec(t, move, larger, sameInstant, stage, next, at.AddDate(0, 0, 3), e.Rep1, pipeline, "imported", run)
	e.WsExec(t, move, smaller, sameInstant, stage, next, at.AddDate(0, 0, 3), e.Rep1, pipeline, "recorded", nil)

	keptThenImported := deal("Kept by hand, moved by an import")
	e.WsExec(t, move, ids.NewV7(), keptThenImported, nil, stage, at, e.Rep1, pipeline, "recorded", nil)
	e.WsExec(t, move, ids.NewV7(), keptThenImported, stage, stage, at.AddDate(0, 0, 1), e.Rep1, pipeline, "recorded", nil)
	e.WsExec(t, move, ids.NewV7(), keptThenImported, stage, next, at.AddDate(0, 0, 3), e.Rep1, pipeline, "imported", run)
}
