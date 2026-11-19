// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

// Package entrycohorts tests the reports built on history rows and on the lead
// entry cohort. They are one row per loss, and the cohort's companies and sources.
package entrycohorts

import (
	"context"
	"errors"
	"maps"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/analyticsquery"
	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/reporting"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

func readerOf(e *integration.Env, without ...string) context.Context {
	objects := maps.Clone(integration.AdminPerms.Objects)
	for _, object := range []string{"deal", "lead", "company", "pipeline", "installation_settings", "forecast", "activity", "contact"} {
		objects[object] = principal.ObjectGrant{Read: true}
	}
	for _, object := range []string{"report_definition", "report_edition", "report_schedule", "reporting_framework", "sales_target", "team_oversight"} {
		objects[object] = principal.ObjectGrant{Read: true, Create: true, Update: true, Delete: true}
	}
	for _, object := range without {
		delete(objects, object)
	}
	perms := principal.Permissions{RoleKeys: integration.AdminPerms.RoleKeys, Objects: objects, RowScope: principal.RowScopeAll}
	ctx := integration.OnSeat(e.As(e.Rep1, []ids.UUID{e.Team1}, perms), principal.SeatFull)
	return auth.WithIndividualPerformance(ctx, true)
}

// repReader is Rep1 on the shipped rep permissions, bounded to the team's rows,
// with the reads a loss question needs.
func repReader(e *integration.Env) context.Context {
	perms := integration.RepPerms
	perms.Objects = maps.Clone(perms.Objects)
	for _, object := range []string{"deal", "pipeline", "installation_settings", "company", "contact"} {
		perms.Objects[object] = principal.ObjectGrant{Read: true}
	}
	return auth.WithIndividualPerformance(integration.OnSeat(e.As(e.Rep1, []ids.UUID{e.Team1}, perms), principal.SeatFull), true)
}

func lossStage(t *testing.T, e *integration.Env) ids.StageID {
	t.Helper()
	pipeline, err := e.Deals.DefaultPipeline(e.Admin())
	if err != nil {
		t.Fatal(err)
	}
	for _, stage := range *pipeline.Stages {
		if stage.Semantic == "lost" {
			return ids.From[ids.StageKind](ids.UUID(stage.Id))
		}
	}
	t.Fatal("the default pipeline has no lost stage")
	return ids.StageID{}
}

// losses counts the loss events of the report by the dimension, as one reader asks.
func losses(reader context.Context, t *testing.T, e *integration.Env, by string, filters ...analyticsquery.Filter) (map[string]any, error) {
	t.Helper()
	var answer compose.AnalyticsAnswer
	query := analyticsquery.Query{Entity: "loss-events", GroupBy: []string{by}, Filters: filters, Measures: []analyticsquery.Measure{{Fn: analyticsquery.CountAll, As: "n"}}}
	if err := database.WithWorkspaceTx(reader, e.Pool, func(tx pgx.Tx) error {
		var err error
		answer, err = compose.RunAnalyticsQuery(reader, tx, query, analyticsquery.Floor(1))
		return err
	}); err != nil {
		return nil, err
	}
	out := map[string]any{}
	for _, row := range answer.Rows {
		out[row[by].(string)] = row["n"]
	}
	return out, nil
}

// dealsOf creates deals and moves them through the production deal writer.
type dealsOf struct {
	t               *testing.T
	e               *integration.Env
	pipeline        ids.PipelineID
	open, won, lost ids.StageID
}

func newDeals(t *testing.T, e *integration.Env) dealsOf {
	t.Helper()
	pipeline, open, won := integration.DealFixture(t, e)
	return dealsOf{t: t, e: e, pipeline: pipeline, open: open, won: won, lost: lossStage(t, e)}
}

func (d dealsOf) create(owner ids.UUID) ids.DealID {
	d.t.Helper()
	user := ids.From[ids.UserKind](owner)
	made, err := d.e.Deals.CreateDeal(d.e.Admin(), deals.CreateDealInput{Name: "Closed deal", PipelineID: d.pipeline, StageID: d.open, OwnerID: &user, Source: "manual"})
	if err != nil {
		d.t.Fatal(err)
	}
	return ids.From[ids.DealKind](ids.UUID(made.Id))
}

func (d dealsOf) move(deal ids.DealID, in deals.AdvanceDealInput) {
	d.t.Helper()
	if _, err := d.e.Deals.AdvanceDeal(d.e.Admin(), deal, in); err != nil {
		d.t.Fatalf("moving to %v: %v", in.ToStageID, err)
	}
}

func (d dealsOf) lose(deal ids.DealID, code string) {
	d.t.Helper()
	d.move(deal, deals.AdvanceDealInput{ToStageID: d.lost, LostReasonCode: &code})
}

func (d dealsOf) reopen(deal ids.DealID) {
	d.t.Helper()
	d.move(deal, deals.AdvanceDealInput{ToStageID: d.open})
}

func (d dealsOf) win(deal ids.DealID) {
	d.t.Helper()
	reason := "purchase_order"
	d.move(deal, deals.AdvanceDealInput{ToStageID: d.won, WonWithoutContractReason: &reason})
}

// Every way a deal can leave and re-enter a loss is counted once per loss, under
// its own reason. A loss that was explicitly reversed is no loss at all.
func TestEveryLossIsOneEventWhateverHappenedToTheDealAfterwards(t *testing.T) {
	e := integration.Setup(t)
	d := newDeals(t, e)
	rep := readerOf(e)
	want := map[string]any{}
	check := func(step string, price, competitor int64) {
		t.Helper()
		if price > 0 {
			want["price"] = price
		}
		if competitor > 0 {
			want["competitor"] = competitor
		}
		got, err := losses(rep, t, e, "loss_reason")
		if err != nil || !maps.Equal(got, want) {
			t.Fatalf("after %s, loss events by reason = %v (%v), want %v", step, got, err, want)
		}
	}

	twice := d.create(e.Rep1)
	d.lose(twice, "price")
	d.reopen(twice)
	d.lose(twice, "competitor")
	check("a deal lost twice", 1, 1)

	d.lose(d.create(e.Rep3), "price")
	check("a colleague's loss", 2, 1)

	wonLater := d.create(e.Rep1)
	d.lose(wonLater, "price")
	d.reopen(wonLater)
	d.win(wonLater)
	check("a deal lost and then won", 3, 1)

	lostWonLost := d.create(e.Rep1)
	d.lose(lostWonLost, "price")
	d.reopen(lostWonLost)
	d.win(lostWonLost)
	d.lose(lostWonLost, "competitor")
	check("a deal lost, won and lost again", 4, 2)

	reversed := d.create(e.Rep1)
	d.lose(reversed, "competitor")
	history := e.WsScalar(t, `SELECT id FROM deal_stage_history WHERE deal_id = $1 AND semantic_at_change = 'lost'`, reversed.UUID)
	loss, err := ids.Parse(history)
	if err != nil {
		t.Fatal(err)
	}
	d.move(reversed, deals.AdvanceDealInput{ToStageID: d.open, ReversalOf: &loss})
	check("an explicitly reversed loss", 4, 2)

	mine := analyticsquery.Filter{Field: "owner_id", Op: analyticsquery.OpEq, Value: e.Rep1.String()}
	own, err := losses(repReader(e), t, e, "loss_reason", mine)
	if err != nil || own["price"] != int64(3) || own["competitor"] != int64(2) {
		t.Errorf("a rep's own loss events by reason = %v, %v, want price 3 and competitor 2", own, err)
	}
	colleague := analyticsquery.Filter{Field: "owner_id", Op: analyticsquery.OpEq, Value: e.Rep3.String()}
	if _, err := losses(repReader(e), t, e, "loss_reason", colleague); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("a rep asking for a colleague's loss events got %v, want a permission refusal", err)
	}
}

// Each loss falls in the period it happened in, not the period of the deal's
// final close.
func TestLossesFallInTheirOwnPeriodsNotTheFinalCloses(t *testing.T) {
	e := integration.Setup(t)
	d := newDeals(t, e)
	deal := d.create(e.Rep1)
	d.lose(deal, "price")
	d.reopen(deal)
	d.lose(deal, "competitor")
	e.WsExec(t, `UPDATE deal_stage_history SET changed_at = CASE lost_reason_code_at_change
		WHEN 'price' THEN '2026-01-15T12:00:00Z'::timestamptz ELSE '2026-02-15T12:00:00Z'::timestamptz END
		WHERE deal_id = $1 AND semantic_at_change = 'lost'`, deal.UUID)

	got, err := losses(readerOf(e), t, e, "period_month")
	if err != nil || len(got) != 2 {
		t.Fatalf("loss events by month = %v (%v), want two months", got, err)
	}
	closing := closingMonths(t, e)
	for month, n := range got {
		if n != int64(1) || closing[month] != nil {
			t.Errorf("month %s holds %v losses and win-loss closes %v there, want one loss each outside the closing month", month, n, closing[month])
		}
	}
}

// closingMonths counts win-loss by the month of each deal's final close.
func closingMonths(t *testing.T, e *integration.Env) map[string]any {
	t.Helper()
	reader := readerOf(e)
	var answer compose.AnalyticsAnswer
	query := analyticsquery.Query{Entity: "win-loss", GroupBy: []string{"period_month"}, Measures: []analyticsquery.Measure{{Fn: analyticsquery.CountAll, As: "n"}}}
	if err := database.WithWorkspaceTx(reader, e.Pool, func(tx pgx.Tx) error {
		var err error
		answer, err = compose.RunAnalyticsQuery(reader, tx, query, analyticsquery.Floor(1))
		return err
	}); err != nil {
		t.Fatal(err)
	}
	out := map[string]any{}
	for _, row := range answer.Rows {
		out[row["period_month"].(string)] = row["n"]
	}
	return out
}

func leadsOf(t *testing.T, e *integration.Env, at time.Time) (create func(origin string, company *ids.UUID) ids.LeadID, store *contacts.Store) {
	t.Helper()
	store = contacts.NewStore(compose.InstallationDB(e.Pool)).SetClock(func() time.Time { return at })
	owner := ids.From[ids.UserKind](e.Rep1)
	create = func(origin string, company *ids.UUID) ids.LeadID {
		name := "Cohort lead"
		made, _, err := store.CreateLead(e.Admin(), contacts.CreateLeadInput{FullName: &name, Status: "new", OwnerID: &owner, Source: "manual", Origin: &origin})
		if err != nil {
			t.Fatal(err)
		}
		id := ids.From[ids.LeadKind](ids.UUID(made.Id))
		if company != nil {
			e.WsExec(t, `UPDATE lead SET company_id = $1 WHERE id = $2`, *company, id.UUID)
		}
		return id
	}
	return create, store
}

// cohortWorld is the entry cohort of the tests below. It has website leads, one
// qualified into a deal by the production qualify path. It also has leads from an
// origin whose key holds a colon and so starts like "website".
type cohortWorld struct {
	e         *integration.Env
	service   *reporting.Service
	selection crmcontracts.ReportingSelection
	qualified ids.LeadID
}

func seedCohort(t *testing.T, e *integration.Env) cohortWorld {
	t.Helper()
	integration.DealFixture(t, e)
	start := clocktest.Now(t).Add(-time.Minute)
	at := start.Add(time.Hour)
	create, store := leadsOf(t, e, start.Add(time.Minute))
	e.WsExec(t, `INSERT INTO lead_origin (key, label, intent, sort_order, system) VALUES ('website:campaign', 'Website campaign', 'neutral', 90, false) ON CONFLICT (key) DO NOTHING`)
	acme, beta := e.SeedCompany(t, "Acme", &e.Rep1), e.SeedCompany(t, "Beta", &e.Rep1)
	qualified := create("website", &acme)
	create("website", &acme)
	disqualified := create("website", &beta)
	create("website:campaign", &beta)
	create("website:campaign", nil)
	if _, err := store.DisqualifyLead(e.Admin(), disqualified, contacts.DisqualifyLeadInput{}); err != nil {
		t.Fatal(err)
	}
	// The provider is the production composition: it wires the deal opener.
	choice := datasource.PromotionDealChoice{Deal: &datasource.PromotionNewDeal{Name: "Qualified website lead"}}
	if _, _, err := compose.NewProvider(e.Pool).PromoteLead(e.Admin(), qualified.UUID, "human_qualify", nil, nil, choice); err != nil {
		t.Fatal(err)
	}
	deal := e.WsScalar(t, `SELECT qualified_deal_id::text FROM lead WHERE id = $1`, qualified.UUID)
	if opened := e.WsCount(t, `SELECT count(*) FROM deal WHERE id::text = $1`, deal); deal == "" || opened != 1 {
		t.Fatalf("qualifying opened deal %q and %d deals match it, want the lead to name one deal", deal, opened)
	}
	interval := crmcontracts.ReportingWindow{StartAt: start, EndAt: at}
	return cohortWorld{e: e, qualified: qualified, service: compose.ReportingServiceFor(e.Pool, func() time.Time { return at }), selection: crmcontracts.ReportingSelection{
		Scope: crmcontracts.ReportingScope{Kind: "workspace"}, Period: "custom", Interval: &interval, TargetBasis: "month", CloseWindow: "all_open",
		Metrics: []crmcontracts.ReportingMetricID{"account_progression", "source_quality"},
		Blocks:  []crmcontracts.ReportingBlockKind{"account_cohort", "source_quality"},
	}}
}

func chartsOf(result crmcontracts.ReportingEvaluation) map[crmcontracts.ReportingBlockKind]crmcontracts.ReportingChart {
	out := map[crmcontracts.ReportingBlockKind]crmcontracts.ReportingChart{}
	for _, chart := range result.Charts {
		out[chart.Kind] = chart
	}
	return out
}

func countsOf(point crmcontracts.ReportingPoint) map[string]int64 {
	out := map[string]int64{}
	for _, outcome := range *point.Outcomes {
		out[outcome.Key] = outcome.Count
	}
	return out
}

// The entry cohort read as companies counts each company once, under the
// furthest outcome of its leads. Read as sources, each one shows its entrants and
// the leads linked to a deal. Evidence opens the members a count names, and an
// origin whose key extends another's is a source of its own.
func TestTheEntryCohortReadsAsCompaniesAndAsSources(t *testing.T) {
	e := integration.Setup(t)
	w := seedCohort(t, e)
	reader := readerOf(e)
	result, err := w.service.Evaluate(reader, w.selection)
	if err != nil {
		t.Fatal(err)
	}
	charts := chartsOf(result)

	companies := charts["account_cohort"].Points
	if len(companies) != 1 || *companies[0].Observations != 2 {
		t.Fatalf("company cohort points %+v, want one point of the 2 companies", companies)
	}
	if got := countsOf(companies[0]); got["qualified"] != 1 || got["disqualified"] != 1 || got["unresolved"] != 0 {
		t.Errorf("company outcomes %v, want Acme qualified (its furthest lead) and Beta disqualified, the open lead behind it left out", got)
	}
	sources := charts["source_quality"].Points
	if len(sources) != 2 || sources[0].Label != "Website" || *sources[0].Observations != 3 || *sources[1].Observations != 2 {
		t.Fatalf("source points %+v, want Website with 3 entrants before Website campaign with 2", sources)
	}
	if got := countsOf(sources[0]); got["opportunity_linked"] != 1 || got["not_linked"] != 2 {
		t.Errorf("website outcomes %v, want 1 linked and 2 not linked", got)
	}
	if got := countsOf(sources[1]); got["opportunity_linked"] != 0 || got["not_linked"] != 2 {
		t.Errorf("website campaign outcomes %v, want none linked", got)
	}

	rows := func(ref crmcontracts.ReportingEvidenceRef) []crmcontracts.ReportingEvidenceRow {
		t.Helper()
		page, err := w.service.Evidence(reader, w.selection, ref.Metric, ref.ContextId, *ref.GroupKey, nil, nil, 50, reportingExpectation(result))
		if err != nil {
			t.Fatal(err)
		}
		return page.Rows
	}
	linked := rows(*(*sources[0].Outcomes)[0].Evidence)
	if len(linked) != 1 || linked[0].SourceId == nil || ids.UUID(*linked[0].SourceId) != w.qualified.UUID {
		t.Errorf("evidence of the linked website leads = %+v, want the one qualified lead", linked)
	}
	if got := len(rows(*sources[0].Evidence)); got != 3 {
		t.Errorf("evidence of the website entrants = %d rows, want its 3 leads and none of the campaign's", got)
	}
	if got := len(rows(*sources[1].Evidence)); got != 2 {
		t.Errorf("evidence of the website campaign entrants = %d rows, want its 2 leads", got)
	}
}

// A scheduled edition freezes both new cards. An admin reads them back whole.
// A rep without company read gets the edition with the company card withheld.
func TestAnEditionWithTheEntryCohortCardsReadsBack(t *testing.T) {
	e := integration.Setup(t)
	w := seedCohort(t, e)
	admin := readerOf(e)
	edition := w.publish(admin, t)

	whole, err := w.service.GetEdition(admin, edition)
	if err != nil {
		t.Fatalf("an admin reading the edition: %v", err)
	}
	charts := chartsOf(whole.Evaluation)
	if whole.Withheld || len(charts["account_cohort"].Points) != 1 || len(charts["source_quality"].Points) != 2 {
		t.Errorf("admin edition: withheld %v, charts %+v, want both cards whole", whole.Withheld, charts)
	}
	for _, kind := range []crmcontracts.ReportingBlockKind{"account_cohort", "source_quality"} {
		metric := charts[kind].Metric
		group := charts[kind].Points[0].Evidence.GroupKey
		page, err := w.service.EditionEvidence(admin, edition, metric, "interval", deref(group), nil, nil, 50)
		if err != nil || len(page.Rows) == 0 {
			t.Errorf("edition evidence of %s = %d rows, err %v, want its members", kind, len(page.Rows), err)
		}
	}

	rep := readerOf(e, "company")
	narrowed, err := w.service.GetEdition(rep, edition)
	if err != nil {
		t.Fatalf("a rep without company read reading the edition: %v", err)
	}
	if !narrowed.Withheld {
		t.Error("the edition a rep without company read was handed is not marked withheld")
	}
	narrowedCharts := chartsOf(narrowed.Evaluation)
	if points := narrowedCharts["account_cohort"].Points; len(points) != 0 && *points[0].Observations != 0 {
		t.Errorf("the company card shows %+v to a rep who may not read companies", points)
	}
	if len(narrowedCharts["source_quality"].Points) != 2 {
		t.Errorf("the source card lost its points for a rep who may read leads: %+v", narrowedCharts["source_quality"].Points)
	}
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// reportingExpectation binds evidence to the evaluation a reader just saw.
func reportingExpectation(result crmcontracts.ReportingEvaluation) reporting.EvidenceExpectation {
	return reporting.EvidenceExpectation{Key: result.EvaluationKey, At: result.Context.EvaluatedAt, FrameworkRevision: result.Context.FrameworkRevision}
}

// publish freezes the selection as a workspace edition and answers its id.
func (w cohortWorld) publish(admin context.Context, t *testing.T) ids.UUID {
	t.Helper()
	w.e.GrantRole(t, w.e.Rep1, "admin")
	report, err := w.service.CreateReport(admin, crmcontracts.ReportingReportInput{Name: "Entry cohort", Audience: "workspace", Selection: w.selection})
	if err != nil {
		t.Fatal(err)
	}
	run, err := w.service.Freeze(admin, ids.UUID(report.Id), 0, "entry-cohort")
	if err != nil {
		t.Fatal(err)
	}
	if err := w.service.Sweep(principal.SystemActing(admin, "system:entry-cohort")); err != nil {
		t.Fatal(err)
	}
	done, err := w.service.GetExecution(admin, ids.UUID(run.Id))
	if err != nil || done.EditionId == nil {
		t.Fatalf("publication: %+v %v", done, err)
	}
	return ids.UUID(*done.EditionId)
}

// The company card is derived from leads. A reader who may read companies but
// not leads gets it withheld, on a frozen edition and on a live evaluation.
func TestTheCompanyCardIsWithheldFromAReaderWhoMayNotReadItsLeads(t *testing.T) {
	e := integration.Setup(t)
	w := seedCohort(t, e)
	admin := readerOf(e)
	edition := w.publish(admin, t)
	noLeads := readerOf(e, "lead")

	frozen, err := w.service.GetEdition(noLeads, edition)
	if err != nil {
		t.Fatalf("frozen edition for a reader without lead read: %v", err)
	}
	if points := chartsOf(frozen.Evaluation)["account_cohort"].Points; !frozen.Withheld || (len(points) != 0 && *points[0].Observations != 0) {
		t.Errorf("frozen: withheld %v, company points %+v, want the card withheld", frozen.Withheld, points)
	}

	selection := w.selection
	selection.Metrics = []crmcontracts.ReportingMetricID{"account_progression"}
	selection.Blocks = []crmcontracts.ReportingBlockKind{"account_cohort"}
	live, err := w.service.Evaluate(noLeads, selection)
	if err == nil {
		if points := chartsOf(live)["account_cohort"].Points; len(points) != 0 && *points[0].Observations != 0 {
			t.Errorf("live: company points %+v, want none for a reader without lead read", points)
		}
	}
}
