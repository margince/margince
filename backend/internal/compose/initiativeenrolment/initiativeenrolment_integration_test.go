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

// backdate moves the initiative's creation, rules and launch two days back.
// A match observed now then dates apart from one estimated from the deal's
// first day, a day back.
func (w manufacturingDeals) backdate(t *testing.T, initiative ids.UUID) {
	t.Helper()
	w.e.WsExec(t, `UPDATE sales_initiative SET created_at = now() - interval '2 days', launch_at = now() - interval '2 days' WHERE id = $1`, initiative)
	w.e.WsExec(t, `UPDATE sales_initiative_revision SET created_at = now() - interval '2 days' WHERE initiative_id = $1`, initiative)
}

// joinedAgo is how long ago the open row of record joined.
func (w manufacturingDeals) joinedAgo(t *testing.T, initiative, record ids.UUID) time.Duration {
	t.Helper()
	seconds := w.e.WsCount(t, `SELECT extract(epoch FROM now() - joined_at)::int FROM sales_initiative_enrolment WHERE initiative_id = $1 AND record_id = $2 AND left_at IS NULL`, initiative, record)
	return time.Duration(seconds) * time.Second
}

func (w manufacturingDeals) sweep(t *testing.T) {
	t.Helper()
	sweep := initiativeenrolment.Sweep{Store: w.store, Pool: w.e.Pool}
	if err := sweep.Workspace(context.Background(), w.e.WS); err != nil {
		t.Fatalf("sweeping: %v", err)
	}
}

// The sweep enrols a record that starts matching the rule, dated when the
// sweep saw it, and dates the leaving of one that stops.
func TestTheEnrolmentSweepDatesRecordsThatJoinAndLeave(t *testing.T) {
	w := seedManufacturingDeals(t)
	id := w.launch(t)
	if got := w.open(t, id); len(got) != 2 || got[w.press] != "rule" || got[w.line] != "rule" {
		t.Fatalf("the launch enrolled %v, want the two manufacturing deals by rule", got)
	}
	w.backdate(t, id)

	joiner := w.deal(t, "Linden mill", w.company)
	w.e.WsExec(t, `UPDATE deal SET archived_at = now() WHERE id = $1`, w.line)
	w.sweep(t)

	got := w.open(t, id)
	if len(got) != 2 || got[joiner] != "rule" || got[w.press] != "rule" {
		t.Fatalf("after the sweep the open rows are %v, want the kept deal and the new match", got)
	}
	if ago := w.joinedAgo(t, id, joiner); ago > time.Hour {
		t.Fatalf("the new match joined %v ago, want when the sweep saw it", ago)
	}
	left := w.e.WsCount(t, `SELECT count(*) FROM sales_initiative_enrolment WHERE initiative_id = $1 AND record_id = $2 AND left_at IS NOT NULL`, id, w.line)
	if left != 1 {
		t.Fatalf("the archived deal has %d closed rows, want 1", left)
	}
}

// The sweep reads every rule match, not one bounded page. Past the bound it
// still enrols each match, and closes the row of a record that stopped matching.
func TestTheSweepDatesEveryMatchPastTheReadBound(t *testing.T) {
	w := seedManufacturingDeals(t)
	id := w.launch(t)
	w.e.WsExec(t, `INSERT INTO deal (id, name, pipeline_id, stage_id, owner_id, company_id, amount_minor, currency, status, expected_close_date, source, captured_by)
		SELECT uuidv7(), 'Linden part ' || g, $1, $2, $3, $4, 100, 'EUR', 'open', current_date + 30, 'manual', 'human:x' FROM generate_series(1, $5::int) g`,
		w.pipeline.UUID, w.stage.UUID, w.e.Rep1, w.company, storekit.PredicateRowLimit)
	w.e.WsExec(t, `UPDATE deal SET archived_at = now() WHERE id = $1`, w.line)
	w.sweep(t)

	if got := w.open(t, id); len(got) != storekit.PredicateRowLimit+1 || got[w.line] != "" {
		t.Fatalf("after the sweep %d rows are open, want every %d matches and not the archived deal", len(got), storekit.PredicateRowLimit+1)
	}
}

// A lead and a deal may share an id. When one leaves, the other's row stays open.
func TestALeadAndADealWithOneIDLeaveApart(t *testing.T) {
	w := seedManufacturingDeals(t)
	w.e.WsExec(t, `INSERT INTO lead (id, full_name, status, source, captured_by, owner_id) VALUES ($1, 'Twin', 'engaged', 'manual', 'human:x', $2)`, w.press, w.e.Rep1)
	ctx := w.manager()
	draft, err := w.store.Create(ctx, initiatives.Definition{Name: "Twins", Objective: "qualified_opportunities_created", Deadline: clocktest.Now(t).UTC().AddDate(0, 3, 0)})
	if err != nil {
		t.Fatalf("defining the initiative: %v", err)
	}
	id := ids.UUID(draft.Id)
	if _, err := w.store.ReplaceMembers(ctx, id, append(refs("lead", w.press), refs("deal", w.press)...), nil); err != nil {
		t.Fatalf("picking the twins: %v", err)
	}
	if _, err := w.store.Launch(ctx, id, nil); err != nil {
		t.Fatalf("launching: %v", err)
	}
	if _, err := w.store.ReplaceMembers(ctx, id, refs("deal", w.press), nil); err != nil {
		t.Fatalf("dropping the lead: %v", err)
	}

	openOf := func(recordType string) int {
		return w.e.WsCount(t, `SELECT count(*) FROM sales_initiative_enrolment WHERE initiative_id = $1 AND record_type = $2 AND record_id = $3 AND left_at IS NULL`, id, recordType, w.press)
	}
	if lead, deal := openOf("lead"), openOf("deal"); lead != 0 || deal != 1 {
		t.Fatalf("open rows: lead %d, deal %d; want the lead closed and the deal open", lead, deal)
	}
}

// The upgrade task dates an initiative launched before the dated rows
// existed, estimated from the records' first day, and a replay writes nothing.
func TestTheEnrolmentTaskDatesAnInitiativeLaunchedBeforeTheRows(t *testing.T) {
	w := seedManufacturingDeals(t)
	id := w.launch(t)
	w.e.WsExec(t, `DELETE FROM sales_initiative_enrolment WHERE initiative_id = $1`, id)
	w.backdate(t, id)
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
	if ago := w.joinedAgo(t, id, w.press); ago < 23*time.Hour {
		t.Fatalf("the task dated a match %v ago, want the deal's first day a day ago", ago)
	}
	if out := batch(); out.Changed != 0 {
		t.Fatalf("a replay changed %d initiatives", out.Changed)
	}
}

func refs(recordType string, records ...ids.UUID) []crmcontracts.SalesInitiativeMemberRef {
	out := make([]crmcontracts.SalesInitiativeMemberRef, 0, len(records))
	for _, record := range records {
		out = append(out, crmcontracts.SalesInitiativeMemberRef{RecordType: crmcontracts.SalesInitiativeMemberRefRecordType(recordType), RecordId: openapi_types.UUID(record)})
	}
	return out
}
