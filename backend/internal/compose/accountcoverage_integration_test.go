// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/network"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// coverageFixture is one installation's accounts, each built to land in one
// view or in none.
type coverageFixture struct {
	twoThreaded, oneThreaded, oneNamed ids.UUID
	touched, untouched, wonSilent      ids.UUID
}

// seedCoverageAccounts plants:
//   - Linden: a deal with two engaged seats and one with one, both written to
//     and answered this month, so only the second is single-threaded;
//   - Tern: an open deal with one named seat nobody has touched, so not judged;
//   - Globex: an open deal with no seat and no exchange;
//   - Acme: a deal won ten days ago, no open deal, no exchange since.
func seedCoverageAccounts(t *testing.T, e *forecastEnv, now time.Time) coverageFixture {
	t.Helper()
	company := func(name string) ids.UUID {
		return e.seedID(t, `INSERT INTO company (id, display_name, source, captured_by) VALUES ($1, $2, 'manual', 'human:x')`, name)
	}
	deal := func(name string, company ids.UUID, status string, amount int64) ids.UUID {
		return e.seedID(t, `INSERT INTO deal (id, name, pipeline_id, stage_id, owner_id, company_id, amount_minor, currency, fx_rate_to_base, fx_rate_date, status, closed_at, expected_close_date, source, captured_by)
			VALUES ($1, $2, $3, $4, $5, $6, $7, 'EUR', 1, current_date, $8, CASE WHEN $8 = 'won' THEN $9::timestamptz END, (now() + interval '30 days')::date, 'manual', 'human:x')`,
			name, e.pipeline, e.stages[pipelineTestStage], e.Rep1, company, amount, status, now.AddDate(0, 0, -10))
	}
	f := coverageFixture{touched: company("Linden"), untouched: company("Tern"), wonSilent: company("Acme")}
	globex := company("Globex")
	f.twoThreaded = deal("Linden press", f.touched, "open", 250000)
	f.oneNamed = deal("Tern portal", f.untouched, "open", 120000)
	deal("Globex renewal", globex, "open", 30000)
	deal("Acme rollout", f.wonSilent, "won", 90000)
	// An old win and a deal with no account land in no view.
	old := company("Initech")
	e.seedID(t, `INSERT INTO deal (id, name, pipeline_id, stage_id, owner_id, company_id, amount_minor, currency, fx_rate_to_base, fx_rate_date, status, closed_at, expected_close_date, source, captured_by)
		VALUES ($1, 'Initech launch', $2, $3, $4, $5, 50000, 'EUR', 1, current_date, 'won', $6, (now() + interval '30 days')::date, 'manual', 'human:x')`,
		e.pipeline, e.stages[pipelineTestStage], e.Rep1, old, now.AddDate(0, 0, -200))
	e.seedID(t, `INSERT INTO deal (id, name, pipeline_id, stage_id, owner_id, amount_minor, currency, fx_rate_to_base, fx_rate_date, expected_close_date, source, captured_by)
		VALUES ($1, 'Orphan deal', $2, $3, $4, 70000, 'EUR', 1, current_date, (now() + interval '30 days')::date, 'manual', 'human:x')`,
		e.pipeline, e.stages[pipelineTestStage], e.Rep1)
	f.oneThreaded = deal("Linden spares", f.touched, "open", 60000)
	for _, seat := range []string{"Buyer", "Champion"} {
		contact := e.seedID(t, `INSERT INTO contact (id, full_name, source, captured_by) VALUES ($1, $2, 'manual', 'human:x')`, seat)
		e.seedID(t, `INSERT INTO relationship (id, kind, contact_id, company_id, source, captured_by) VALUES ($1, 'employment', $2, $3, 'manual', 'human:x')`, contact, f.touched)
		for _, seated := range map[string][]ids.UUID{"Buyer": {f.twoThreaded, f.oneThreaded}, "Champion": {f.twoThreaded}}[seat] {
			e.seedID(t, `INSERT INTO relationship (id, kind, contact_id, deal_id, role, source, captured_by) VALUES ($1, 'deal_stakeholder', $2, $3, 'economic_buyer', 'manual', 'human:x')`, contact, seated)
		}
		for _, direction := range []string{"inbound", "outbound"} {
			activity := e.seedID(t, `INSERT INTO activity (id, kind, direction, subject, body, occurred_at, source, captured_by) VALUES ($1, 'email', $2, 'Re: press line', 'text', $3, 'manual', 'human:x')`, direction, now.AddDate(0, 0, -3))
			e.seedID(t, `INSERT INTO activity_link (id, activity_id, entity_type, contact_id) VALUES ($1, $2, 'contact', $3)`, activity, contact)
			for _, onDeal := range []ids.UUID{f.twoThreaded, f.oneThreaded} {
				e.seedID(t, `INSERT INTO activity_link (id, activity_id, entity_type, deal_id) VALUES ($1, $2, 'deal', $3)`, activity, onDeal)
			}
		}
	}
	silent := e.seedID(t, `INSERT INTO contact (id, full_name, source, captured_by) VALUES ($1, 'Quiet buyer', 'manual', 'human:x')`)
	e.seedID(t, `INSERT INTO relationship (id, kind, contact_id, deal_id, role, source, captured_by) VALUES ($1, 'deal_stakeholder', $2, $3, 'economic_buyer', 'manual', 'human:x')`, silent, f.oneNamed)
	return f
}

// coverageReader is the initiative manager with the edge grant a committee
// read takes.
func coverageViewer(e *forecastEnv) context.Context {
	ctx := initiativeManager(e)
	p, _ := principal.Actor(ctx)
	p.Permissions.Objects["relationship"] = principal.ObjectGrant{Read: true}
	p.Permissions.Objects["contact"] = principal.ObjectGrant{Read: true}
	return principal.WithActor(ctx, p)
}

func coverageOf(t *testing.T, e *forecastEnv, ctx context.Context, now time.Time, view crmcontracts.GetAccountCoverageParamsView) crmcontracts.AccountCoverage {
	t.Helper()
	svc := accountCoverage{db: InstallationDB(e.Pool), now: func() time.Time { return now }}
	out, err := svc.Read(ctx, crmcontracts.GetAccountCoverageParams{View: &view})
	if err != nil {
		t.Fatalf("reading the %s view: %v", view, err)
	}
	return out
}

func dealNames(rows []crmcontracts.AccountCoverageRow) []string {
	var names []string
	for _, row := range rows {
		if row.Deal != nil {
			names = append(names, row.Deal.Name)
		} else {
			names = append(names, row.Company.Name)
		}
	}
	return names
}

// Each account lands in the one view its facts put it in, the counts name
// every view, and the thresholds printed are the defaults.
func TestTheAccountCoverageViewsJudgeEachAccountOnce(t *testing.T) {
	t.Parallel()
	e := setupForecast(t)
	now := clocktest.Now(t).UTC().Truncate(time.Second)
	f := seedCoverageAccounts(t, e, now)
	ctx := coverageViewer(e)

	single := coverageOf(t, e, ctx, now, crmcontracts.GetAccountCoverageParamsViewSingleThreaded)
	if got := dealNames(single.Rows); len(got) != 1 || got[0] != "Linden spares" {
		t.Fatalf("single-threaded deals %v, want the touched deal with one engaged seat alone", got)
	}
	if *single.Rows[0].NamedSeats != 1 || *single.Rows[0].EngagedSeats != 1 {
		t.Fatalf("the seat counts are %+v", single.Rows[0])
	}
	assertTheDealPageAgrees(ctx, t, e, now, map[ids.UUID]bool{f.oneThreaded: true, f.twoThreaded: false, f.oneNamed: false})
	if single.Counts.SingleThreaded != 1 || single.Counts.Untouched == nil || *single.Counts.Untouched != 2 || single.Counts.Silent == nil || *single.Counts.Silent != 1 {
		t.Fatalf("the counts are %+v", single.Counts)
	}
	if *single.Thresholds.MaxEngagedSeats != 1 || *single.Thresholds.UntouchedDays != 21 || *single.Thresholds.SilentDays != 30 || *single.Thresholds.WonWithinDays != 180 {
		t.Fatalf("the thresholds are %+v, want the defaults", single.Thresholds)
	}
	if !single.ActivityReadable || single.Scope.Kind != crmcontracts.AnalyticsScopeKindWorkspace && single.Scope.Kind != crmcontracts.AnalyticsScopeKindManagedTeams {
		t.Fatalf("the answer is %+v", single)
	}

	untouched := coverageOf(t, e, ctx, now, crmcontracts.GetAccountCoverageParamsViewUntouched)
	if got := dealNames(untouched.Rows); len(got) != 2 || got[0] != "Tern" || got[1] != "Globex" {
		t.Fatalf("untouched accounts %v, want the two accounts nobody exchanged a word with, largest first", got)
	}
	if untouched.Rows[0].OpenDeals == nil || *untouched.Rows[0].OpenDeals != 1 || untouched.Rows[0].DaysSilent != nil {
		t.Fatalf("the untouched row is %+v, want one open deal and no silence count", untouched.Rows[0])
	}

	silent := coverageOf(t, e, ctx, now, crmcontracts.GetAccountCoverageParamsViewSilent)
	if got := dealNames(silent.Rows); len(got) != 1 || got[0] != "Acme" {
		t.Fatalf("silent accounts %v, want the won account alone", got)
	}
	if silent.Rows[0].WonDeal == nil || silent.Rows[0].WonDeal.Name != "Acme rollout" || silent.Rows[0].DaysSilent == nil || *silent.Rows[0].DaysSilent != 10 {
		t.Fatalf("the silent row is %+v, want the won deal and ten days since its close", silent.Rows[0])
	}
	if single.Truncated != nil && *single.Truncated {
		t.Fatalf("a population under the cap reads as truncated")
	}
	empty := coverageOf(t, e, ctx, now.AddDate(0, 0, 400), crmcontracts.GetAccountCoverageParamsViewSilent)
	if empty.Rows == nil || len(empty.Rows) != 0 {
		t.Fatalf("a view with nothing at risk answers %#v, want an empty list on the wire", empty.Rows)
	}
}

// Without the activity grant nothing is judged: every verdict reads who
// wrote, so the answer says so rather than counting zero.
func TestTheAccountCoverageReadWithoutActivityJudgesNothing(t *testing.T) {
	t.Parallel()
	e := setupForecast(t)
	now := clocktest.Now(t).UTC().Truncate(time.Second)
	seedCoverageAccounts(t, e, now)
	ctx := principal.WithCorrelationID(principal.WithWorkspaceID(context.Background(), e.WS), ids.NewV7())
	grants := map[string]principal.ObjectGrant{}
	for _, object := range []string{"deal", "company", "contact", "relationship", "installation_settings"} {
		grants[object] = principal.ObjectGrant{Read: true}
	}
	ctx = principal.WithActor(ctx, principal.Principal{Type: principal.PrincipalHuman, ID: "human:" + e.Rep1.String(), UserID: e.Rep1, SeatType: principal.SeatFull, TeamIDs: []ids.UUID{e.Team1}, Permissions: principal.Permissions{RowScope: principal.RowScopeAll, Objects: grants}})

	out := coverageOf(t, e, ctx, now, crmcontracts.GetAccountCoverageParamsViewSingleThreaded)
	if out.ActivityReadable || out.Counts.Untouched != nil || out.Counts.Silent != nil || len(out.Rows) != 0 || out.Counts.SingleThreaded != 0 || out.Withheld != nil {
		t.Fatalf("without activity the answer is %+v, want nothing judged", out)
	}
}

// A published threshold moves the verdict on both surfaces: with two engaged
// seats allowed, the two-threaded deal is single-threaded too.
func TestThePublishedThresholdsMoveTheAccountCoverageVerdict(t *testing.T) {
	t.Parallel()
	e := setupForecast(t)
	now := clocktest.Now(t).UTC().Truncate(time.Second)
	f := seedCoverageAccounts(t, e, now)
	ctx := coverageViewer(e)
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `INSERT INTO reporting_framework_revision (revision, definition, effective_at, created_by) VALUES (1, '{"template":"sales","qualification":[],"capture_contexts":[],"reason":"test","account_coverage":{"max_engaged_seats":2}}', now(), $1)`, e.Rep1)
		if err != nil {
			return err
		}
		_, err = tx.Exec(context.Background(), `INSERT INTO reporting_framework (revision, version) VALUES (1, 1)`)
		return err
	}); err != nil {
		t.Fatalf("publishing a framework: %v", err)
	}
	out := coverageOf(t, e, ctx, now, crmcontracts.GetAccountCoverageParamsViewSingleThreaded)
	if out.Counts.SingleThreaded != 2 || *out.Thresholds.MaxEngagedSeats != 2 || out.FrameworkRevision != 1 {
		t.Fatalf("under a threshold of two the answer is counts %+v, thresholds %+v, revision %d", out.Counts, out.Thresholds, out.FrameworkRevision)
	}
	assertTheDealPageAgrees(ctx, t, e, now, map[ids.UUID]bool{f.oneThreaded: true, f.twoThreaded: true, f.oneNamed: false})
}

// assertTheDealPageAgrees wants each deal page's single-threaded finding where Account coverage put the deal.
func assertTheDealPageAgrees(ctx context.Context, t *testing.T, e *forecastEnv, now time.Time, want map[ids.UUID]bool) {
	t.Helper()
	for dealID, single := range want {
		var coverage network.DealCoverage
		if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) (err error) {
			coverage, err = network.CoverageFor(ctx, tx, ids.From[ids.DealKind](dealID), now)
			return err
		}); err != nil {
			t.Fatalf("reading deal %s's coverage: %v", dealID, err)
		}
		if flagged := slices.ContainsFunc(coverage.Risks, func(r network.Risk) bool { return r.Kind == network.RiskSingleThreadedTheirs }); flagged != single {
			t.Errorf("the deal page flags deal %s single-threaded %v, Account coverage says %v", dealID, flagged, single)
		}
	}
}
