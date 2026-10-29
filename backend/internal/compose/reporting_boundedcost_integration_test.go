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
		reading := evaluateOwnerCost(t, service, reader, selection)
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

func evaluateOwnerCost(t *testing.T, service *reporting.Service, reader context.Context, selection crmcontracts.ReportingSelection) ownerCostReading {
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
