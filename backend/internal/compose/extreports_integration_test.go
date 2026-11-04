// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A registered count or breakdown meets the report engine. The kind's read
// right admits it. A field mask excludes rows and says so. The group floor
// withholds the smallest groups and the total. A multi-select custom field
// splits a core population without counting a record twice.

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/margince/margince/backend/internal/compose/analyticsquery"
	"github.com/margince/margince/backend/internal/compose/installseam"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/customfields"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/recordkinds"
	"github.com/margince/margince/backend/pkg/extension"
)

const (
	reportDemoTable  = "ext_reportdemo_visit"
	reportDemoObject = "ext_reportdemo_visit"
)

// registerVisitReports declares a visit kind with its table and the reports
// over it, and puts both registries back when the test ends.
func registerVisitReports(t *testing.T) {
	t.Helper()
	owner := integration.OwnerConn(t)
	ctx := context.Background()
	for _, stmt := range []string{
		`CREATE SCHEMA IF NOT EXISTS ext`,
		`GRANT USAGE ON SCHEMA ext TO margince_app`,
		`DROP TABLE IF EXISTS ext.` + reportDemoTable,
		`CREATE TABLE ext.` + reportDemoTable + ` (id uuid PRIMARY KEY DEFAULT uuidv7(), archived_at timestamptz, code text NOT NULL UNIQUE, site text, tier integer NOT NULL DEFAULT 2)`,
		`GRANT SELECT, INSERT, UPDATE, DELETE ON ext.` + reportDemoTable + ` TO margince_app`,
	} {
		if _, err := owner.Exec(ctx, stmt); err != nil {
			t.Fatalf("preparing the visit table (%s): %v", stmt, err)
		}
	}
	unit := extension.Name("reportdemo")
	kinds := []extension.RecordKindDeclaration{{
		Kind: "visit", DisplayName: "Visit", Table: reportDemoTable,
		Fields:     []extension.RecordKindField{{Name: "code", Label: "Code"}, {Name: "site", Label: "Site"}, {Name: "tier", Label: "Tier"}},
		NaturalKey: []string{"code"},
	}}
	exts := []extension.Extension{{
		Name: unit, RecordKinds: kinds,
		ReportCounts: []extension.ReportCount{{Name: "visits", Label: "Visits", Population: "visit"}},
		ReportBreakdowns: []extension.ReportBreakdown{
			{Name: "by_site", Label: "Visits by site", Population: "visit", By: "site"},
			{Name: "by_tier", Label: "Visits by tier", Population: "visit", By: "tier"},
		},
	}}
	recordkinds.Register(unit, kinds)
	registerReports(exts)
	t.Cleanup(func() {
		recordkinds.Reset()
		registerReports(nil)
		if _, err := owner.Exec(context.Background(), `DROP TABLE IF EXISTS ext.`+reportDemoTable); err != nil {
			t.Errorf("dropping the visit table: %v", err)
		}
	})
}

func seedVisits(t *testing.T, site string, live, archived int) {
	t.Helper()
	owner := integration.OwnerConn(t)
	for i := range live + archived {
		var archivedAt any
		if i >= live {
			archivedAt = "now()"
		}
		if _, err := owner.Exec(context.Background(),
			`INSERT INTO ext.`+reportDemoTable+` (code, site, archived_at) VALUES ($1, $2, CASE WHEN $3::text IS NULL THEN NULL ELSE now() END)`,
			fmt.Sprintf("%s-%d", site, i), site, archivedAt); err != nil {
			t.Fatalf("seeding a visit: %v", err)
		}
	}
}

func visitReader(e *integration.Env, mask ...principal.FieldMask) context.Context {
	return e.As(e.Rep1, nil, principal.Permissions{
		Objects: map[string]principal.ObjectGrant{
			reportDemoObject:        {Read: true},
			"installation_settings": {Read: true},
		},
		// Team scope, because a seat that reads every row is exempt from masks.
		RowScope:   principal.RowScopeTeam,
		FieldMasks: mask,
	})
}

func runRegisteredReport(t *testing.T, e *integration.Env, ctx context.Context, name string) extensionReportAnswer {
	t.Helper()
	engine := newReportEngine(e.Pool)
	for _, r := range registeredReports() {
		if r.name != name {
			continue
		}
		answer, ok, err := engine.runRegistered(ctx, r, analyticsquery.DefaultFloor)
		if err != nil || !ok {
			t.Fatalf("running %s: ok=%v err=%v", name, ok, err)
		}
		return answer
	}
	t.Fatalf("%s is not registered", name)
	return extensionReportAnswer{}
}

func TestARegisteredBreakdownCountsLiveRowsOfTheKind(t *testing.T) {
	e := integration.Setup(t)
	registerVisitReports(t)
	seedVisits(t, "north", 6, 3)
	seedVisits(t, "south", 7, 0)

	answer := runRegisteredReport(t, e, visitReader(e), "ext:reportdemo:by_site")
	if answer.Withheld || answer.Total == nil || *answer.Total != 13 {
		t.Fatalf("answer = %+v, want 13 live visits and nothing withheld (archived rows are not counted)", answer)
	}
	counts := map[string]int{}
	for _, g := range answer.Groups {
		counts[fmt.Sprint(g.Key)] = g.Count
	}
	if counts["north"] != 6 || counts["south"] != 7 {
		t.Errorf("groups = %v, want north=6 south=7", counts)
	}
	count := runRegisteredReport(t, e, visitReader(e), "ext:reportdemo:visits")
	if count.Total == nil || *count.Total != 13 {
		t.Errorf("count = %+v, want 13", count)
	}
}

func TestAReaderWithoutTheKindsReadRightCannotRunItsReports(t *testing.T) {
	e := integration.Setup(t)
	registerVisitReports(t)
	seedVisits(t, "north", 6, 0)
	ctx := e.As(e.Rep1, nil, principal.Permissions{RowScope: principal.RowScopeAll})

	_, _, err := newReportEngine(e.Pool).runRegistered(ctx, registeredReports()[0], analyticsquery.DefaultFloor)
	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("err = %v, want permission denied", err)
	}
}

func TestTheFloorWithholdsTheSmallestGroupsAndTheTotal(t *testing.T) {
	e := integration.Setup(t)
	registerVisitReports(t)
	seedVisits(t, "tiny", 1, 0)
	seedVisits(t, "small", 5, 0)
	seedVisits(t, "large", 9, 0)

	answer := runRegisteredReport(t, e, visitReader(e), "ext:reportdemo:by_site")
	if !answer.Withheld || answer.Total != nil {
		t.Fatalf("withheld=%v total=%v, want the total withheld with the groups", answer.Withheld, answer.Total)
	}
	shown := map[string]int{}
	for _, g := range answer.Groups {
		if !g.Withheld {
			shown[fmt.Sprint(g.Key)] = g.Count
		}
	}
	if len(shown) != 1 || shown["large"] != 9 {
		t.Errorf("shown = %v, want only the large group: the tiny one and the smallest remaining are withheld", shown)
	}
}

func TestAFieldMaskExcludesTheKindsRowsAndCountsThem(t *testing.T) {
	e := integration.Setup(t)
	registerVisitReports(t)
	seedVisits(t, "north", 6, 0)
	seedVisits(t, "south", 7, 0)

	masked := visitReader(e, principal.FieldMask{Object: reportDemoObject, Field: "site", Condition: principal.MaskAlways})
	answer := runRegisteredReport(t, e, masked, "ext:reportdemo:by_site")
	if answer.ExcludedByPermission == nil || *answer.ExcludedByPermission != 13 {
		t.Fatalf("excluded_by_permission = %v, want 13", answer.ExcludedByPermission)
	}
	if answer.Total == nil || *answer.Total != 0 || len(answer.Groups) != 0 {
		t.Errorf("answer = %+v, want the masked rows out of every group and the total", answer)
	}
	open := runRegisteredReport(t, e, visitReader(e), "ext:reportdemo:by_site")
	if open.ExcludedByPermission != nil {
		t.Errorf("unmasked excluded_by_permission = %v, want none", *open.ExcludedByPermission)
	}
}

func TestABreakdownByANonTextColumnKeepsItsKeys(t *testing.T) {
	e := integration.Setup(t)
	registerVisitReports(t)
	seedVisits(t, "north", 6, 0)

	answer := extensionReportWire(runRegisteredReport(t, e, visitReader(e), "ext:reportdemo:by_tier"))
	if len(answer.Groups) != 1 || answer.Groups[0].Key == nil || *answer.Groups[0].Key != "2" {
		t.Fatalf("groups = %+v, want one group keyed \"2\": an integer column is text on the wire", answer.Groups)
	}
}

func TestAMultiselectCustomFieldSplitsDealsByCombinationWithoutDoubleCounting(t *testing.T) {
	e := integration.Setup(t)
	catalog := customfields.NewService(e.Pool, integration.SchemaPool(t))
	field, err := catalog.Create(e.As(e.Rep1, nil, integration.CustomFieldAdminPerms), customfields.FieldSpec{
		Object: "deal", Label: "Regions", Type: customfields.TypeMultiselect,
		Options: []string{"emea", "apac", "amer"}, Source: "manual",
	})
	if err != nil {
		t.Fatalf("create the multiselect field: %v", err)
	}
	column := *field.ColumnName
	pipeline, open, _ := integration.DealFixture(t, e)
	store := deals.NewStore(e.DB(), installseam.Deals()).WithFieldCatalog(catalog)
	for i, regions := range [][]string{
		{"emea", "apac"}, {"apac", "emea"}, {"emea", "apac"}, {"emea", "apac"}, {"emea", "apac"},
		{"amer"}, {"amer"}, {"amer"}, {"amer"}, {"amer"},
	} {
		if _, err := store.CreateDeal(e.Admin(), deals.CreateDealInput{
			Name: fmt.Sprintf("Regional deal %d", i), PipelineID: pipeline, StageID: open,
			CustomFields: map[string]any{column: regions},
		}); err != nil {
			t.Fatalf("seed deal %d: %v", i, err)
		}
	}
	registerReports([]extension.Extension{{
		Name: "reportdemo",
		ReportBreakdowns: []extension.ReportBreakdown{
			{Name: "by_region", Label: "Deals by region", Population: "pipeline-current", By: column},
		},
	}})
	t.Cleanup(func() { registerReports(nil) })

	answer := runRegisteredReport(t, e, e.Admin(), "ext:reportdemo:by_region")
	counts := map[string]int{}
	for _, g := range answer.Groups {
		counts[fmt.Sprint(g.Key)] = g.Count
	}
	if counts["apac,emea"] != 5 || counts["amer"] != 5 || answer.Total == nil || *answer.Total != 10 {
		t.Errorf("groups = %v total = %v, want {emea,apac} in one group of 5, amer 5, total 10", counts, answer.Total)
	}
}
