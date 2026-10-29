// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"fmt"
	"maps"
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/reporting"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// ownerCostReading is what one reader's evaluation of a team cost and showed.
type ownerCostReading struct {
	statements int64
	targets    map[string]float64
	allocated  *float64
	teamTarget *float64
}

func TestReportingEvaluationCostDoesNotGrowWithTheTeamsOwners(t *testing.T) {
	small := reportingTeamOwnerCost(t, 2)
	large := reportingTeamOwnerCost(t, 12)
	for reader, few := range small {
		many := large[reader]
		t.Logf("%s: statements with 3 owners=%d, with 13 owners=%d", reader, few.statements, many.statements)
		if few.statements == 0 || many.statements != few.statements {
			t.Errorf("%s: an evaluation grew with the owners it shows: 3 owners=%d statements, 13 owners=%d", reader, few.statements, many.statements)
		}
	}
}

// reportingTeamOwnerCost evaluates a team of Rep1 and extra owners under three
// seats. Each owner has a target, and each seat must see the targets its tier
// allows.
func reportingTeamOwnerCost(t *testing.T, extra int) map[string]ownerCostReading {
	t.Helper()
	e := setupForecast(t)
	at := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	service := newReportingService(e.Pool, func() time.Time { return at })
	admin := reportingActor(e)
	owners := []ids.UUID{e.Rep1}
	for i := range extra {
		owner := ids.NewV7()
		if _, err := e.owner.Exec(context.Background(), `INSERT INTO app_user (id, email, display_name) VALUES ($1, $2, $3)`, owner, fmt.Sprintf("owner%02d@forecast.test", i), fmt.Sprintf("Owner %02d", i)); err != nil {
			t.Fatal(err)
		}
		if _, err := e.owner.Exec(context.Background(), `INSERT INTO team_membership (team_id, user_id) VALUES ($1, $2)`, e.Team1, owner); err != nil {
			t.Fatal(err)
		}
		owners = append(owners, owner)
	}
	month := openapi_types.Date{Time: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)}
	want := map[string]float64{}
	for i, owner := range owners {
		value := int64(1000000 * (i + 1))
		if _, err := service.CreateTarget(admin, crmcontracts.ReportingTargetInput{Metric: "bookings_won", Scope: crmcontracts.ReportingScope{Kind: "owner", Id: ptrUUID(owner)}, PeriodKind: "month", PeriodStart: month, Value: value, Reason: "Owner commitment"}); err != nil {
			t.Fatal(err)
		}
		want[owner.String()] = float64(value)
	}
	if _, err := service.CreateTarget(admin, crmcontracts.ReportingTargetInput{Metric: "bookings_won", Scope: crmcontracts.ReportingScope{Kind: "team", Id: ptrUUID(e.Team1)}, PeriodKind: "month", PeriodStart: month, Value: 90000000, Reason: "Team commitment"}); err != nil {
		t.Fatal(err)
	}
	selection := crmcontracts.ReportingSelection{Scope: crmcontracts.ReportingScope{Kind: "team", Id: ptrUUID(e.Team1)}, Period: "this_month", TargetBasis: "month", CloseWindow: "all_open", Metrics: []crmcontracts.ReportingMetricID{"bookings_won", "closed_win_rate", "open_pipeline"}, Blocks: []crmcontracts.ReportingBlockKind{"owner_attainment", "target_progress"}}
	readings := map[string]ownerCostReading{}
	for name, reader := range map[string]context.Context{"admin": admin, "team lead": reportingTieredReader(admin, principal.RowScopeTeam), "rep": reportingTieredReader(admin, principal.RowScopeOwn)} {
		reading := evaluateOwnerCost(reader, t, service, selection)
		visible := want
		if name == "rep" {
			visible = map[string]float64{e.Rep1.String(): want[e.Rep1.String()]}
		}
		if !maps.Equal(reading.targets, visible) {
			t.Errorf("%s: owner targets = %v, want %v", name, reading.targets, visible)
		}
		if reading.teamTarget == nil || *reading.teamTarget != 90000000 {
			t.Errorf("%s: team target = %v, want 90000000", name, reading.teamTarget)
		}
		if name == "admin" && (reading.allocated == nil || *reading.allocated != sumOf(want)) {
			t.Errorf("admin: allocated = %v, want %v", reading.allocated, sumOf(want))
		}
		if name != "admin" && reading.allocated != nil {
			t.Errorf("%s: sees the allocation of targets it does not read", name)
		}
		readings[name] = reading
	}
	return readings
}

func evaluateOwnerCost(reader context.Context, t *testing.T, service *reporting.Service, selection crmcontracts.ReportingSelection) ownerCostReading {
	t.Helper()
	metered, meter := database.WithStatementMeter(reader)
	result, err := service.Evaluate(metered, selection)
	if err != nil {
		t.Fatal(err)
	}
	reading := ownerCostReading{statements: meter.Cost().Statements, targets: map[string]float64{}}
	for _, metric := range result.Metrics {
		if metric.Id == "bookings_won" {
			reading.teamTarget = metric.Target
		}
	}
	for _, chart := range result.Charts {
		if chart.Kind != "owner_attainment" {
			continue
		}
		reading.allocated = chart.AllocatedTarget
		for _, point := range chart.Points {
			if point.Target != nil {
				reading.targets[point.Key] = *point.Target
			}
		}
	}
	return reading
}

// reportingTieredReader is the admin's seat on a narrower row scope that may
// read targets and set none.
func reportingTieredReader(admin context.Context, scope principal.RowScope) context.Context {
	actor, _ := principal.Actor(admin)
	actor.Permissions.Objects = maps.Clone(actor.Permissions.Objects)
	actor.Permissions.Objects["sales_target"] = principal.ObjectGrant{Read: true}
	actor.Permissions.RowScope = scope
	return principal.WithActor(admin, actor)
}

func sumOf(values map[string]float64) float64 {
	total := float64(0)
	for _, value := range values {
		total += value
	}
	return total
}

// TestReportingTargetCostDoesNotGrowWithTheMetricsHeldAgainstTargets measures
// what reading targets adds: the same seat with and without the target grant.
// It compares two and five metrics, each with a target. The metric readers
// grow with the metrics; the target work must not.
func TestReportingTargetCostDoesNotGrowWithTheMetricsHeldAgainstTargets(t *testing.T) {
	e := setupForecast(t)
	at := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	service := newReportingService(e.Pool, func() time.Time { return at })
	admin := reportingActor(e)
	seedTargetExclusions(t, service, admin, e)
	blind := reportingTieredReader(admin, principal.RowScopeAll)
	actor, _ := principal.Actor(blind)
	actor.Permissions.Objects["sales_target"] = principal.ObjectGrant{}
	blind = principal.WithActor(blind, actor)
	targetCost := func(metrics ...crmcontracts.ReportingMetricID) int64 {
		selection := crmcontracts.ReportingSelection{Scope: crmcontracts.ReportingScope{Kind: "workspace"}, Period: "this_month", TargetBasis: "month", CloseWindow: "all_open", Metrics: metrics, Blocks: []crmcontracts.ReportingBlockKind{"target_progress"}}
		return evaluateOwnerCost(admin, t, service, selection).statements - evaluateOwnerCost(blind, t, service, selection).statements
	}
	few := targetCost("bookings_won", "meetings_held")
	many := targetCost("bookings_won", "meetings_held", "qualified_pipeline_created", "accepted_opportunities", "partner_pipeline")
	t.Logf("statements the targets add: 2 metrics=%d, 5 metrics=%d", few, many)
	if few <= 0 || many != few {
		t.Errorf("target work grew with the metrics: 2 metrics=%d statements, 5 metrics=%d", few, many)
	}
}

// TestReportingBatchedTargetsKeepEveryExclusion holds each figure against only
// the target of its own metric, scope, pipeline and interval.
func TestReportingBatchedTargetsKeepEveryExclusion(t *testing.T) {
	e := setupForecast(t)
	at := time.Date(2026, 9, 29, 12, 0, 0, 0, time.UTC)
	service := newReportingService(e.Pool, func() time.Time { return at })
	admin := reportingActor(e)
	seedTargetExclusions(t, service, admin, e)
	selection := crmcontracts.ReportingSelection{Scope: crmcontracts.ReportingScope{Kind: "workspace"}, Period: "this_month", TargetBasis: "month", CloseWindow: "all_open", Metrics: []crmcontracts.ReportingMetricID{"bookings_won", "meetings_held", "qualified_pipeline_created", "accepted_opportunities", "partner_pipeline"}, Blocks: []crmcontracts.ReportingBlockKind{"owner_attainment", "target_progress"}}
	result, err := service.Evaluate(admin, selection)
	if err != nil {
		t.Fatal(err)
	}
	// The October and the pipeline target of sales won, the retired partner
	// target and Rep1's own credit target must all stay out.
	want := map[crmcontracts.ReportingMetricID]*float64{"bookings_won": ptrFloat(30000000), "meetings_held": ptrFloat(12), "qualified_pipeline_created": ptrFloat(5000000), "accepted_opportunities": nil, "partner_pipeline": nil}
	for _, metric := range result.Metrics {
		if expected := want[metric.Id]; (expected == nil) != (metric.Target == nil) || (expected != nil && *expected != *metric.Target) {
			t.Errorf("%s: target = %s, want %s", metric.Id, valueOf(metric.Target), valueOf(expected))
		}
	}
	reading := evaluateOwnerCost(admin, t, service, crmcontracts.ReportingSelection{Scope: selection.Scope, Period: selection.Period, TargetBasis: selection.TargetBasis, CloseWindow: selection.CloseWindow, Metrics: []crmcontracts.ReportingMetricID{"bookings_won"}, Blocks: []crmcontracts.ReportingBlockKind{"owner_attainment"}})
	if !maps.Equal(reading.targets, map[string]float64{e.Rep1.String(): 20000000}) {
		t.Errorf("owner targets = %v, want only Rep1's sales target", reading.targets)
	}
}

// seedTargetExclusions writes the September targets an evaluation should
// hold, beside targets that differ in one part of the key each.
func seedTargetExclusions(t *testing.T, service *reporting.Service, admin context.Context, e *forecastEnv) {
	t.Helper()
	september := openapi_types.Date{Time: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)}
	october := openapi_types.Date{Time: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)}
	workspace := crmcontracts.ReportingScope{Kind: "workspace"}
	rep1 := crmcontracts.ReportingScope{Kind: "owner", Id: ptrUUID(e.Rep1)}
	create := func(metric crmcontracts.ReportingMetricID, scope crmcontracts.ReportingScope, pipeline *openapi_types.UUID, start openapi_types.Date, value int64) crmcontracts.ReportingTarget {
		t.Helper()
		target, err := service.CreateTarget(admin, crmcontracts.ReportingTargetInput{Metric: metric, Scope: scope, PipelineId: pipeline, PeriodKind: "month", PeriodStart: start, Value: value, Reason: "Fixture target"})
		if err != nil {
			t.Fatalf("%s target: %v", metric, err)
		}
		return target
	}
	create("bookings_won", workspace, nil, september, 30000000)
	create("bookings_won", workspace, ptrUUID(e.pipeline), september, 31000000)
	create("bookings_won", workspace, nil, october, 32000000)
	create("bookings_won", rep1, nil, september, 20000000)
	create("meetings_held", workspace, nil, september, 12)
	create("qualified_pipeline_created", workspace, nil, september, 5000000)
	create("accepted_opportunities", rep1, nil, september, 7)
	partner := create("partner_pipeline", workspace, nil, september, 9000000)
	retired := true
	input := partner.Definition
	input.Retired = &retired
	input.Reason = "Fixture retirement"
	if _, err := service.UpdateTarget(admin, ids.UUID(partner.Id), partner.Version, input); err != nil {
		t.Fatal(err)
	}
}

func ptrFloat(value float64) *float64 { return &value }

func valueOf(value *float64) string {
	if value == nil {
		return "none"
	}
	return fmt.Sprint(*value)
}
