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
)

func readerOf(e *integration.Env, scope principal.RowScope) context.Context {
	objects := maps.Clone(integration.AdminPerms.Objects)
	for _, object := range []string{"deal", "lead", "company", "pipeline", "installation_settings", "forecast", "activity", "contact"} {
		objects[object] = principal.ObjectGrant{Read: true}
	}
	perms := principal.Permissions{RoleKeys: integration.AdminPerms.RoleKeys, Objects: objects, RowScope: scope}
	ctx := integration.OnSeat(e.As(e.Rep1, []ids.UUID{e.Team1}, perms), principal.SeatFull)
	return auth.WithIndividualPerformance(ctx, true)
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

// A deal lost, reopened and lost again is two loss rows, each under the reason
// it was lost with. A rep may count their own losses and is refused a colleague's.
func TestADealLostTwiceIsTwoLossEventsAndARepIsRefusedAColleaguesLosses(t *testing.T) {
	e := integration.Setup(t)
	pipeline, open, _ := integration.DealFixture(t, e)
	lost := lossStage(t, e)
	admin := e.Admin()
	lose := func(deal ids.DealID, code string) {
		t.Helper()
		if _, err := e.Deals.AdvanceDeal(admin, deal, deals.AdvanceDealInput{ToStageID: lost, LostReasonCode: &code}); err != nil {
			t.Fatalf("losing as %s: %v", code, err)
		}
	}
	newDeal := func(owner ids.UUID) ids.DealID {
		t.Helper()
		user := ids.From[ids.UserKind](owner)
		made, err := e.Deals.CreateDeal(admin, deals.CreateDealInput{Name: "Lost deal", PipelineID: pipeline, StageID: open, OwnerID: &user, Source: "manual"})
		if err != nil {
			t.Fatal(err)
		}
		return ids.From[ids.DealKind](ids.UUID(made.Id))
	}
	twice := newDeal(e.Rep1)
	lose(twice, "price")
	if _, err := e.Deals.AdvanceDeal(admin, twice, deals.AdvanceDealInput{ToStageID: open}); err != nil {
		t.Fatal(err)
	}
	lose(twice, "competitor")
	lose(newDeal(e.Rep3), "price")

	everything, err := losses(readerOf(e, principal.RowScopeAll), t, e, "loss_reason")
	if err != nil || everything["price"] != int64(2) || everything["competitor"] != int64(1) {
		t.Errorf("loss events by reason = %v, %v, want price 2 (two deals) and competitor 1 (the second loss of one deal)", everything, err)
	}
	rep := readerOf(e, principal.RowScopeOwn)
	mine := analyticsquery.Filter{Field: "owner_id", Op: analyticsquery.OpEq, Value: e.Rep1.String()}
	own, err := losses(rep, t, e, "loss_reason", mine)
	if err != nil || own["price"] != int64(1) || own["competitor"] != int64(1) {
		t.Errorf("a rep's own loss events by reason = %v, %v, want the two losses of their own deal", own, err)
	}
	colleague := analyticsquery.Filter{Field: "owner_id", Op: analyticsquery.OpEq, Value: e.Rep3.String()}
	if _, err := losses(rep, t, e, "loss_reason", colleague); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("a rep asking for a colleague's loss events got %v, want a permission refusal", err)
	}
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

// The entry cohort read as companies counts each company once, under the
// furthest outcome of its leads. Read as sources, each one shows its entrants and
// the leads linked to a deal. Evidence opens the members a count names.
func TestTheEntryCohortReadsAsCompaniesAndAsSources(t *testing.T) {
	e := integration.Setup(t)
	pipeline, open, _ := integration.DealFixture(t, e)
	linkedTo := e.SeedDeal(t, "Linked deal", pipeline, open, &e.Rep1)
	start := clocktest.Now(t).Add(-time.Minute)
	at := start.Add(3 * time.Minute)
	create, store := leadsOf(t, e, start.Add(time.Minute))
	acme, beta := e.SeedCompany(t, "Acme", &e.Rep1), e.SeedCompany(t, "Beta", &e.Rep1)
	qualified := create("website", &acme)
	create("website", &acme)
	disqualified := create("website", &beta)
	create("linkedin", &beta)
	create("linkedin", nil)
	if _, _, err := store.PromoteLead(e.Admin(), qualified, contacts.PromoteLeadInput{Trigger: "human_qualify"}); err != nil {
		t.Fatal(err)
	}
	e.WsExec(t, `UPDATE lead SET qualified_deal_id = $1 WHERE id = $2`, linkedTo, qualified.UUID)
	if _, err := store.DisqualifyLead(e.Admin(), disqualified, contacts.DisqualifyLeadInput{}); err != nil {
		t.Fatal(err)
	}

	service := compose.ReportingServiceFor(e.Pool, func() time.Time { return at })
	interval := crmcontracts.ReportingWindow{StartAt: start, EndAt: at}
	selection := crmcontracts.ReportingSelection{
		Scope: crmcontracts.ReportingScope{Kind: "workspace"}, Period: "custom", Interval: &interval, TargetBasis: "month", CloseWindow: "all_open",
		Metrics: []crmcontracts.ReportingMetricID{"account_progression", "source_quality"},
		Blocks:  []crmcontracts.ReportingBlockKind{"account_cohort", "source_quality"},
	}
	result, err := service.Evaluate(readerOf(e, principal.RowScopeAll), selection)
	if err != nil {
		t.Fatal(err)
	}
	charts := map[crmcontracts.ReportingBlockKind]crmcontracts.ReportingChart{}
	for _, chart := range result.Charts {
		charts[chart.Kind] = chart
	}
	counts := func(point crmcontracts.ReportingPoint) map[string]int64 {
		out := map[string]int64{}
		for _, outcome := range *point.Outcomes {
			out[outcome.Key] = outcome.Count
		}
		return out
	}

	companies := charts["account_cohort"].Points
	if len(companies) != 1 || *companies[0].Observations != 2 {
		t.Fatalf("company cohort points %+v, want one point of the 2 companies", companies)
	}
	if got := counts(companies[0]); got["qualified"] != 1 || got["disqualified"] != 1 || got["unresolved"] != 0 {
		t.Errorf("company outcomes %v, want Acme qualified (its furthest lead) and Beta disqualified, the open lead behind it left out", got)
	}
	sources := charts["source_quality"].Points
	if len(sources) != 2 || sources[0].Key != "source:website" || *sources[0].Observations != 3 || *sources[1].Observations != 2 {
		t.Fatalf("source points %+v, want website with 3 entrants before linkedin with 2", sources)
	}
	if got := counts(sources[0]); got["opportunity_linked"] != 1 || got["not_linked"] != 2 {
		t.Errorf("website outcomes %v, want 1 linked and 2 not linked", got)
	}
	if got := counts(sources[1]); got["opportunity_linked"] != 0 || got["not_linked"] != 2 {
		t.Errorf("linkedin outcomes %v, want none linked", got)
	}

	linked := *(*sources[0].Outcomes)[0].Evidence
	page, err := service.Evidence(readerOf(e, principal.RowScopeAll), selection, linked.Metric, linked.ContextId, *linked.GroupKey, nil, nil, 50,
		reportingExpectation(result))
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Rows) != 1 || page.Rows[0].SourceId == nil || ids.UUID(*page.Rows[0].SourceId) != qualified.UUID {
		t.Errorf("evidence of the linked website leads = %+v, want the one qualified lead", page.Rows)
	}
	entrants := *sources[0].Evidence
	page, err = service.Evidence(readerOf(e, principal.RowScopeAll), selection, entrants.Metric, entrants.ContextId, *entrants.GroupKey, nil, nil, 50,
		reportingExpectation(result))
	if err != nil || len(page.Rows) != 3 {
		t.Errorf("evidence of the website entrants = %d rows, err %v, want its 3 leads", len(page.Rows), err)
	}
}

// reportingExpectation binds evidence to the evaluation a reader just saw.
func reportingExpectation(result crmcontracts.ReportingEvaluation) reporting.EvidenceExpectation {
	return reporting.EvidenceExpectation{Key: result.EvaluationKey, At: result.Context.EvaluatedAt, FrameworkRevision: result.Context.FrameworkRevision}
}
