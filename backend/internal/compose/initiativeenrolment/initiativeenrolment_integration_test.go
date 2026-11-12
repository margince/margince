// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package initiativeenrolment_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/initiativeenrolment"
	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/initiatives"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// managerGrants holds what a manager launching an initiative holds: the initiative,
// the reporting it narrows, and the records its rule reads.
var managerGrants = func() principal.Permissions {
	grants := map[string]principal.ObjectGrant{}
	for _, object := range []string{"sales_initiative", "report_definition", "installation_settings", "deal", "pipeline", "lead", "company", "product", "activity"} {
		grants[object] = principal.ObjectGrant{Create: true, Read: true, Update: true}
	}
	return principal.Permissions{RowScope: principal.RowScopeAll, Objects: grants}
}()

// manufacturingDeals is a workspace with two open deals at a manufacturing
// customer and one at a software customer.
type manufacturingDeals struct {
	e           *integration.Env
	store       *initiatives.Store
	company     ids.UUID
	pipeline    ids.PipelineID
	stage       ids.StageID
	press, line ids.UUID
	portal      ids.UUID
}

func seedManufacturingDeals(t *testing.T) manufacturingDeals {
	t.Helper()
	e := integration.Setup(t)
	pipeline, stage, _ := integration.DealFixture(t, e)
	w := manufacturingDeals{e: e, store: compose.NewInitiativeStore(e.Pool), pipeline: pipeline, stage: stage}
	w.company = e.SeedCompany(t, "Linden Manufacturing", nil)
	e.WsExec(t, `UPDATE company SET industry = 'manufacturing' WHERE id = $1`, w.company)
	software := e.SeedCompany(t, "Tern Digital", nil)
	e.WsExec(t, `UPDATE company SET industry = 'software' WHERE id = $1`, software)
	w.press = w.deal(t, "Linden press", w.company)
	w.line = w.deal(t, "Linden line", w.company)
	w.portal = w.deal(t, "Tern portal", software)
	return w
}

func (w manufacturingDeals) deal(t *testing.T, name string, company ids.UUID) ids.UUID {
	t.Helper()
	deal := w.e.SeedDeal(t, name, w.pipeline, w.stage, &w.e.Rep1)
	w.e.WsExec(t, `UPDATE deal SET company_id = $2, amount_minor = 10000, currency = 'EUR', created_at = now() - interval '1 day' WHERE id = $1`, deal, company)
	return deal
}

func (w manufacturingDeals) manager() context.Context {
	return w.e.As(w.e.Rep1, []ids.UUID{w.e.Team1}, managerGrants)
}

// launch launches an initiative whose rule admits the manufacturing deals.
func (w manufacturingDeals) launch(t *testing.T) ids.UUID {
	t.Helper()
	ctx := w.manager()
	rule := storekit.Predicate{Field: "company_industry", Op: "eq", Value: "manufacturing"}
	deadline := clocktest.Now(t).UTC().AddDate(0, 3, 0)
	draft, err := w.store.Create(ctx, initiatives.Definition{Name: "Manufacturing expansion", Objective: "qualified_opportunities_created", Deadline: deadline, DealRule: &rule})
	if err != nil {
		t.Fatalf("defining the initiative: %v", err)
	}
	if _, err := w.store.Launch(ctx, ids.UUID(draft.Id), nil); err != nil {
		t.Fatalf("launching: %v", err)
	}
	return ids.UUID(draft.Id)
}

// open maps each record with an open dated row to its reason.
func (w manufacturingDeals) open(t *testing.T, initiative ids.UUID) map[ids.UUID]string {
	t.Helper()
	ctx := principal.WithWorkspaceID(context.Background(), w.e.WS)
	open := map[ids.UUID]string{}
	if err := database.WithWorkspaceTx(ctx, w.e.Pool, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `SELECT record_id, reason FROM sales_initiative_enrolment WHERE initiative_id = $1 AND left_at IS NULL`, initiative)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var record ids.UUID
			var reason string
			if err := rows.Scan(&record, &reason); err != nil {
				return err
			}
			open[record] = reason
		}
		return rows.Err()
	}); err != nil {
		t.Fatalf("reading the open enrolments: %v", err)
	}
	return open
}

// The sweep enrols a record that starts matching the rule and dates the
// leaving of one that stops.
func TestTheEnrolmentSweepDatesRecordsThatJoinAndLeave(t *testing.T) {
	w := seedManufacturingDeals(t)
	id := w.launch(t)
	if got := w.open(t, id); len(got) != 2 || got[w.press] != "rule" || got[w.line] != "rule" {
		t.Fatalf("the launch enrolled %v, want the two manufacturing deals by rule", got)
	}

	joiner := w.deal(t, "Linden mill", w.company)
	w.e.WsExec(t, `UPDATE deal SET archived_at = now() WHERE id = $1`, w.line)
	sweep := initiativeenrolment.Sweep{Store: w.store, Pool: w.e.Pool}
	if err := sweep.Workspace(context.Background(), w.e.WS); err != nil {
		t.Fatalf("sweeping: %v", err)
	}

	got := w.open(t, id)
	if len(got) != 2 || got[joiner] != "rule" || got[w.press] != "rule" {
		t.Fatalf("after the sweep the open rows are %v, want the kept deal and the new match", got)
	}
	left := w.e.WsCount(t, `SELECT count(*) FROM sales_initiative_enrolment WHERE initiative_id = $1 AND record_id = $2 AND left_at IS NOT NULL`, id, w.line)
	if left != 1 {
		t.Fatalf("the archived deal has %d closed rows, want 1", left)
	}
}

// The upgrade task dates an initiative launched before the dated rows
// existed, and a replay writes nothing.
func TestTheEnrolmentTaskDatesAnInitiativeLaunchedBeforeTheRows(t *testing.T) {
	w := seedManufacturingDeals(t)
	id := w.launch(t)
	w.e.WsExec(t, `DELETE FROM sales_initiative_enrolment WHERE initiative_id = $1`, id)
	task := initiativeenrolment.Task(w.store)
	ctx := datatask.Acting(principal.WithWorkspaceID(context.Background(), w.e.WS), task.ID)
	batch := func() datatask.BatchOut {
		t.Helper()
		var out datatask.BatchOut
		if err := database.WithWorkspaceTx(ctx, w.e.Pool, func(tx pgx.Tx) error {
			var err error
			out, err = task.Steps[0].Batch(ctx, tx, datatask.BatchIn{Limit: 10, RunID: ids.NewV7()})
			return err
		}); err != nil {
			t.Fatalf("running the batch: %v", err)
		}
		return out
	}

	if out := batch(); out.Changed != 1 {
		t.Fatalf("the task changed %d initiatives, want 1", out.Changed)
	}
	if got := w.open(t, id); len(got) != 2 {
		t.Fatalf("the task enrolled %v, want the two manufacturing deals", got)
	}
	if out := batch(); out.Changed != 0 {
		t.Fatalf("a replay changed %d initiatives", out.Changed)
	}
}

func leadRefs(leads ...ids.UUID) []crmcontracts.SalesInitiativeMemberRef {
	refs := make([]crmcontracts.SalesInitiativeMemberRef, 0, len(leads))
	for _, lead := range leads {
		refs = append(refs, crmcontracts.SalesInitiativeMemberRef{RecordType: "lead", RecordId: openapi_types.UUID(lead)})
	}
	return refs
}

// A lead picked before its promotion sourced its deal. A lead picked after
// its promotion belongs to the initiative, but its deal was not sourced by it.
func TestALeadPickedAfterItsPromotionSourcedNothing(t *testing.T) {
	w := seedManufacturingDeals(t)
	start := clocktest.Now(t).UTC().Truncate(time.Second)
	ctx := w.manager()
	lead := func(name string) ids.UUID {
		id := ids.NewV7()
		w.e.WsExec(t, `INSERT INTO lead (id, full_name, status, source, captured_by, owner_id, created_at) VALUES ($1, $2, 'engaged', 'manual', 'human:x', $3, $4)`,
			id, name, w.e.Rep1, start.Add(-48*time.Hour))
		return id
	}
	early, late := lead("Ada Early"), lead("Ben Late")
	draft, err := w.store.Create(ctx, initiatives.Definition{Name: "Lead push", Objective: "qualified_opportunities_created", Deadline: start.AddDate(0, 3, 0)})
	if err != nil {
		t.Fatalf("defining the initiative: %v", err)
	}
	id := ids.UUID(draft.Id)
	if _, err := w.store.ReplaceMembers(ctx, id, leadRefs(early), nil); err != nil {
		t.Fatalf("picking the early lead: %v", err)
	}
	if _, err := w.store.Launch(ctx, id, nil); err != nil {
		t.Fatalf("launching: %v", err)
	}
	for lead, deal := range map[ids.UUID]ids.UUID{early: w.press, late: w.portal} {
		w.e.WsExec(t, `UPDATE lead SET qualified_deal_id = $2, promoted_at = now() WHERE id = $1`, lead, deal)
	}
	if _, err := w.store.ReplaceMembers(ctx, id, leadRefs(early, late), nil); err != nil {
		t.Fatalf("picking the late lead: %v", err)
	}

	service := compose.NewReportingService(w.e.Pool, func() time.Time { return start.Add(time.Hour) })
	interval := crmcontracts.ReportingWindow{StartAt: start.AddDate(0, 0, -3), EndAt: start.AddDate(0, 0, 3)}
	wire := openapi_types.UUID(id)
	result, err := service.Evaluate(ctx, crmcontracts.ReportingSelection{
		Scope: crmcontracts.ReportingScope{Kind: "workspace"}, Period: "custom", Interval: &interval, TargetBasis: "month", CloseWindow: "all_open",
		InitiativeId: &wire, Metrics: []crmcontracts.ReportingMetricID{initiativePipeline}, Blocks: []crmcontracts.ReportingBlockKind{"breakdown"},
	})
	if err != nil {
		t.Fatalf("evaluating under the initiative: %v", err)
	}
	if origins := originSplit(t, result); origins["origin:sourced"] != 10_000 || origins["origin:added_after"] != 10_000 {
		t.Fatalf("the origin split is %v, want the early lead's deal sourced and the late lead's added after", origins)
	}
}

const initiativePipeline crmcontracts.ReportingMetricID = "initiative_pipeline"

// originSplit reads the initiative pipeline's breakdown by origin.
func originSplit(t *testing.T, result crmcontracts.ReportingEvaluation) map[string]float64 {
	t.Helper()
	for _, chart := range result.Charts {
		if chart.Kind != "breakdown" || chart.Metric != initiativePipeline {
			continue
		}
		values := map[string]float64{}
		for _, point := range chart.Points {
			if point.Value != nil {
				values[point.Key] = *point.Value
			}
		}
		return values
	}
	t.Fatal("the initiative pipeline has no breakdown chart")
	return nil
}
