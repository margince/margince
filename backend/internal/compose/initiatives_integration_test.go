// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"slices"
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/initiatives"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// initiativeManager holds the grants a manager defining an initiative holds:
// the initiative itself, the reporting it narrows, and the records it reads.
func initiativeManager(e *forecastEnv) context.Context {
	ctx := principal.WithCorrelationID(principal.WithWorkspaceID(context.Background(), e.WS), ids.NewV7())
	grants := map[string]principal.ObjectGrant{}
	for _, object := range []string{"sales_initiative", "report_definition", "installation_settings", "deal", "pipeline", "lead", "company", "product"} {
		grants[object] = principal.ObjectGrant{Create: true, Read: true, Update: true}
	}
	return principal.WithActor(ctx, principal.Principal{Type: principal.PrincipalHuman, ID: "human:" + e.Rep1.String(), UserID: e.Rep1, SeatType: principal.SeatFull, TeamIDs: []ids.UUID{e.Team1}, Permissions: principal.Permissions{RowScope: principal.RowScopeAll, Objects: grants}})
}

func seedInitiativeDeals(t *testing.T, e *forecastEnv, start time.Time) (manufacturing, software ids.UUID, open map[string]ids.UUID) {
	t.Helper()
	manufacturing = e.seedID(t, `INSERT INTO company (id, display_name, industry, source, captured_by) VALUES ($1, 'Linden Manufacturing', 'manufacturing', 'manual', 'human:x')`)
	software = e.seedID(t, `INSERT INTO company (id, display_name, industry, source, captured_by) VALUES ($1, 'Tern Digital', 'software', 'manual', 'human:x')`)
	open = map[string]ids.UUID{}
	for name, company := range map[string]ids.UUID{"Linden press": manufacturing, "Linden line": manufacturing, "Tern portal": software} {
		open[name] = e.seedID(t, `INSERT INTO deal (id, name, pipeline_id, stage_id, owner_id, company_id, amount_minor, currency, status, created_at, expected_close_date, source, captured_by)
			VALUES ($1, $2, $3, $4, $5, $6, 10000, 'EUR', 'open', $7, current_date + 30, 'manual', 'human:x')`,
			name, e.pipeline, e.stages[pipelineTestStage], e.Rep1, company, start.Add(-24*time.Hour))
	}
	return manufacturing, software, open
}

// A rule on the customer's industry includes the manufacturing deals and no
// other; a pick adds one more; an exclusion stands over the rule; the launch
// freezes those deals as the baseline; and a report narrowed to the
// initiative reads them alone.
func TestAnInitiativeIsDefinedByItsRulesPicksAndExclusions(t *testing.T) {
	e := setupForecast(t)
	start := clocktest.Now(t).UTC().Truncate(time.Second)
	_, _, open := seedInitiativeDeals(t, e, start)
	ctx := initiativeManager(e)
	store := newInitiativeStore(e.Pool)
	target := int64(5)
	draft, err := store.Create(ctx, initiatives.Definition{Name: "Manufacturing expansion", Objective: reportingQualifiedOpportunitiesCreated, Target: &target, Deadline: start.AddDate(0, 3, 0), DealRule: &initiativeRulePredicate})
	if err != nil {
		t.Fatalf("defining the initiative: %v", err)
	}
	id := ids.UUID(draft.Id)
	members, err := store.Members(ctx, id, nil)
	if err != nil {
		t.Fatalf("reading the draft's members: %v", err)
	}
	if members.Deals != 2 || members.Companies != 1 {
		t.Fatalf("the rule matched %d deals of %d companies, want the 2 manufacturing deals of 1 company: %+v", members.Deals, members.Companies, members.Data)
	}
	picked := []crmcontracts.SalesInitiativeMemberRef{{RecordType: "deal", RecordId: openapi_types.UUID(open["Tern portal"])}}
	excluded := []crmcontracts.SalesInitiativeMemberRef{{RecordType: "deal", RecordId: openapi_types.UUID(open["Linden line"])}}
	members, err = store.ReplaceMembers(ctx, id, picked, excluded)
	if err != nil {
		t.Fatalf("setting picks and exclusions: %v", err)
	}
	reasons := map[string]string{}
	for _, member := range members.Data {
		reasons[member.Label] = string(member.Reason)
		if member.Label == "Linden line" && member.Included {
			t.Fatal("an excluded rule match is still included")
		}
	}
	if reasons["Tern portal"] != "picked" || reasons["Linden press"] != "rule" || reasons["Linden line"] != "excluded" {
		t.Fatalf("the reasons are %v", reasons)
	}
	if members.Deals != 2 {
		t.Fatalf("%d deals belong, want the rule match and the pick", members.Deals)
	}

	live, err := store.Launch(ctx, id, nil)
	if err != nil {
		t.Fatalf("launching: %v", err)
	}
	if live.Status != "live" || live.LaunchAt == nil {
		t.Fatalf("the launch left %+v", live)
	}
	var baseline []ids.UUID
	rows, err := e.Pool.Query(context.Background(), `SELECT deal_id FROM sales_initiative_baseline WHERE initiative_id = $1 ORDER BY deal_id`, id)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var deal ids.UUID
		if err := rows.Scan(&deal); err != nil {
			t.Fatal(err)
		}
		baseline = append(baseline, deal)
	}
	rows.Close()
	if len(baseline) != 2 || slices.Contains(baseline, open["Linden line"]) {
		t.Fatalf("the baseline holds %v, want the two members and not the excluded deal", baseline)
	}
	if _, err := store.Launch(ctx, id, nil); err == nil {
		t.Fatal("a second launch was accepted")
	}

	service := newReportingService(e.Pool, func() time.Time { return start.Add(time.Hour) })
	interval := crmcontracts.ReportingWindow{StartAt: start.AddDate(0, 0, -3), EndAt: start.AddDate(0, 0, 3)}
	wire := openapi_types.UUID(id)
	selection := crmcontracts.ReportingSelection{Scope: crmcontracts.ReportingScope{Kind: "workspace"}, Period: "custom", Interval: &interval, TargetBasis: "month", CloseWindow: "all_open", InitiativeId: &wire, Metrics: []crmcontracts.ReportingMetricID{reportingOpenPipeline, reportingInitiativePipeline}, Blocks: []crmcontracts.ReportingBlockKind{"breakdown", "metric_reading"}}
	result, err := service.Evaluate(ctx, selection)
	if err != nil {
		t.Fatalf("evaluating under the initiative: %v", err)
	}
	if got := sectionsValue(t, result, reportingOpenPipeline); got != 20_000 {
		t.Fatalf("open pipeline under the initiative is %v, want the two members' 20000", got)
	}
	if result.Context.InitiativeLabel == nil || *result.Context.InitiativeLabel != "Manufacturing expansion" {
		t.Fatalf("the context names the initiative %v", result.Context.InitiativeLabel)
	}
	origins, _ := sectionsBreakdown(t, result, reportingInitiativePipeline)
	if origins["origin:existing"] != 20_000 {
		t.Fatalf("the origin split is %v, want 20000 existing at launch", origins)
	}
	reader := sectionsReader(e, e.Rep1, []ids.UUID{e.Team1}, principal.RowScopeAll, "sales_initiative")
	if _, err := service.Evaluate(reader, selection); err == nil {
		t.Fatal("a reader without the initiative grant narrowed a report to it")
	}
}

// The deal rule of the test: the customer's industry is manufacturing.
var initiativeRulePredicate = storekitPredicate("company_industry", "eq", "manufacturing")

func storekitPredicate(field, op string, value any) storekit.Predicate {
	return storekit.Predicate{Field: field, Op: op, Value: value}
}
