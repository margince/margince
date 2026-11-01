// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// What an import brings in is the source system's history. The readings of
// how this team sells leave it out: stage conversion and close-date pushes. A
// move saved before provenance was kept still counts.

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// importRunSeedSQL opens a running CSV import, the run every write of an import
// job is marked with.
const importRunSeedSQL = `INSERT INTO import_run (id, connector, status, source_ref, source, captured_by)
	VALUES ($1, 'csv', 'running', 'deals.csv', 'upload', 'human:test')`

// importer is the context the import job writes under: a member allowed to
// import, with every write marked as landed by one run.
func (f *stageCohortFixture) importer(t *testing.T) context.Context {
	t.Helper()
	run := f.env.seedID(t, importRunSeedSQL)
	all := principal.ObjectGrant{Read: true, Create: true, Update: true, Delete: true}
	ctx := principal.WithCorrelationID(principal.WithWorkspaceID(context.Background(), f.env.WS), ids.NewV7())
	ctx = principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + f.env.Rep1.String(), UserID: f.env.Rep1, SeatType: principal.SeatFull,
		Permissions: principal.Permissions{RowScope: principal.RowScopeAll, Objects: map[string]principal.ObjectGrant{
			"deal": all, "pipeline": all, migration.ImportRunObject: all,
		}},
	})
	return storekit.WithImportRun(storekit.WithBatch(ctx, run), run)
}

// importWon brings in a deal its source already won, through the importer's
// own door.
func (f *stageCohortFixture) importWon(importer context.Context, t *testing.T) {
	t.Helper()
	owner := ids.From[ids.UserKind](f.env.Rep1)
	closed := f.at
	err := f.store.Tx(importer, func(tx pgx.Tx) error {
		_, err := f.store.ImportDealTx(importer, tx, deals.CreateDealInput{
			Name: "Imported win", PipelineID: ids.From[ids.PipelineKind](f.env.pipeline), OwnerID: &owner, Source: "import",
		}, &deals.ImportTerminal{StageID: ids.From[ids.StageKind](f.won), ClosedAt: &closed}, deals.CustomColumns{})
		return err
	})
	if err != nil {
		t.Fatalf("importing a won deal: %v", err)
	}
	f.at = f.at.Add(time.Hour)
}

// importMove moves a deal the way a re-import of its row does.
func (f *stageCohortFixture) importMove(importer context.Context, t *testing.T, deal ids.DealID, to ids.UUID) {
	t.Helper()
	err := f.store.Tx(importer, func(tx pgx.Tx) error {
		_, err := f.store.ImportAdvanceDealTx(importer, tx, deal, deals.AdvanceDealInput{ToStageID: ids.From[ids.StageKind](to)}, nil, deals.CustomColumns{})
		return err
	})
	if err != nil {
		t.Fatalf("moving a deal by import: %v", err)
	}
	f.at = f.at.Add(time.Hour)
}

// forgetProvenance makes a deal's moves into one stage look like rows written
// before a move kept where it came from.
func (f *stageCohortFixture) forgetProvenance(t *testing.T, deal ids.DealID, stage *ids.UUID) {
	t.Helper()
	if _, err := f.env.owner.Exec(context.Background(), `UPDATE deal_stage_history SET provenance = NULL
		WHERE deal_id = $1 AND ($2::uuid IS NULL OR to_stage_id = $2)`, deal.UUID, stage); err != nil {
		t.Fatal(err)
	}
}

// An imported move starts no entry, and an entry whose next move was imported
// is history_partial and stays out of the rate. A move saved before provenance
// was kept still counts.
func TestStageConversionLeavesOutImportedMoves(t *testing.T) {
	f := newStageCohortFixture(t)
	s0, s1 := f.stage(0), f.stage(1)
	rep1 := f.env.Rep1
	importer := f.importer(t)
	start := f.at

	// Five hand moves, the least a stage needs to show a rate.
	for range 5 {
		f.deal(t, rep1, s0, s1)
	}
	f.importWon(importer, t)
	movedOnByImport := f.deal(t, rep1, s0)
	f.importMove(importer, t, movedOnByImport, s1)
	legacyExit := f.deal(t, rep1, s0, s1)
	f.forgetProvenance(t, legacyExit, &s1)

	interval := crmcontracts.ReportingWindow{StartAt: start, EndAt: f.at.Add(time.Hour)}
	at := f.at.Add(2 * time.Hour)
	result := f.evaluate(t, newReportingService(f.env.Pool, func() time.Time { return at }), crmcontracts.ReportingScope{Kind: "workspace"}, interval)
	rows := cohortRows(t, result)

	first := rows[s0.String()]
	if first.observations != 7 || first.outcomes[cohortAdvanced] != 6 || first.outcomes[cohortHistoryPartial] != 1 || first.outcomes[cohortWon] != 0 {
		t.Fatalf("first stage: %+v, want six advanced, the legacy exit among them, one exit by import, and no entry for the imported win", first)
	}
	second := rows[s1.String()]
	if second.observations != 6 || second.outcomes[cohortUnresolved] != 6 {
		t.Fatalf("second stage: %+v, want the hand moves' and the legacy move's open entries, not the imported one", second)
	}
	if metric := result.Metrics[0]; metric.Numerator == nil || *metric.Numerator != 6 || *metric.Denominator != 6 || metric.Coverage.Status != reportingPartial {
		t.Fatalf("stage conversion: %+v, want 6 of 6, partial for the imported exit and the legacy move", metric)
	}
}

// A deal whose moves all predate provenance still counts, and the reading says
// some of its moves may have been imports.
func TestStageConversionIsPartialWhenAnEntryPredatesProvenance(t *testing.T) {
	f := newStageCohortFixture(t)
	s0, s1 := f.stage(0), f.stage(1)
	start := f.at
	for range 5 {
		f.deal(t, f.env.Rep1, s0, s1)
	}
	legacy := f.deal(t, f.env.Rep1, s0, s1)
	f.forgetProvenance(t, legacy, nil)

	interval := crmcontracts.ReportingWindow{StartAt: start, EndAt: f.at.Add(time.Hour)}
	at := f.at.Add(2 * time.Hour)
	result := f.evaluate(t, newReportingService(f.env.Pool, func() time.Time { return at }), crmcontracts.ReportingScope{Kind: "workspace"}, interval)
	first := cohortRows(t, result)[s0.String()]
	if first.observations != 6 || first.outcomes[cohortAdvanced] != 6 {
		t.Fatalf("first stage: %+v, want the five moves with provenance and the legacy one", first)
	}
	if metric := result.Metrics[0]; metric.Coverage.Status != reportingPartial {
		t.Fatalf("stage conversion coverage: %+v, want partial for the moves that may be imports", metric.Coverage)
	}
}

// An entry's outcome is its next move before the evaluation instant, so a
// legacy exit after the window's end still marks the reading partial.
func TestStageConversionIsPartialWhenALegacyExitFallsAfterTheWindow(t *testing.T) {
	f := newStageCohortFixture(t)
	s0, s1 := f.stage(0), f.stage(1)
	start := f.at
	for range 5 {
		f.deal(t, f.env.Rep1, s0, s1)
	}
	late := f.deal(t, f.env.Rep1, s0)
	interval := crmcontracts.ReportingWindow{StartAt: start, EndAt: f.at}
	f.at = f.at.Add(2 * time.Hour)
	if _, err := f.store.AdvanceDeal(f.writer, late, deals.AdvanceDealInput{ToStageID: ids.From[ids.StageKind](s1)}); err != nil {
		t.Fatal(err)
	}
	f.forgetProvenance(t, late, &s1)

	at := f.at.Add(time.Hour)
	result := f.evaluate(t, newReportingService(f.env.Pool, func() time.Time { return at }), crmcontracts.ReportingScope{Kind: "workspace"}, interval)
	if first := cohortRows(t, result)[s0.String()]; first.observations != 6 || first.outcomes[cohortAdvanced] != 6 {
		t.Fatalf("first stage: %+v, want the late deal advanced by its legacy exit", first)
	}
	if metric := result.Metrics[0]; metric.Coverage.Status != reportingPartial {
		t.Fatalf("stage conversion coverage: %+v, want partial for the legacy exit after the window", metric.Coverage)
	}
}

// A close date an import moved is the source system's history. Only the push a
// user made counts toward the closePushed rule.
func TestACloseDateAnImportMovedIsNotAPush(t *testing.T) {
	e := integration.Setup(t)
	pipeline, open, _ := integration.DealFixture(t, e)
	deal := ids.From[ids.DealKind](e.SeedDeal(t, "Pushed by an import", pipeline, open, nil))
	run := integration.SeedIDRow(t, integration.OwnerConn(t), importRunSeedSQL)
	imported := storekit.WithImportRun(storekit.WithBatch(e.Admin(), run), run)
	today := clocktest.Now(t).UTC().Truncate(24 * time.Hour)
	for i, writer := range []context.Context{e.Admin(), imported, imported, e.Admin()} {
		date := today.AddDate(0, 0, 30+10*i)
		if _, err := e.Deals.UpdateDeal(writer, deal, deals.UpdateDealInput{ExpectedClose: &date}); err != nil {
			t.Fatalf("moving the close date to %s: %v", date.Format(time.DateOnly), err)
		}
	}

	for _, subject := range subjectsFor(t, e) {
		if subject.DealID == deal.String() {
			if subject.CloseDatePushes != 1 {
				t.Fatalf("close-date pushes = %d, want 1: the two the import made do not count", subject.CloseDatePushes)
			}
			return
		}
	}
	t.Fatal("the pushed deal is not among the subjects")
}
