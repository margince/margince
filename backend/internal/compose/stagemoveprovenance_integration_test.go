// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// What an import brings in is the source system's history, so the readings of
// how this team sells leave it out: stage conversion and close-date pushes.

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

// importer is the context the import job writes under: a person allowed to
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
func (f *stageCohortFixture) importWon(t *testing.T, importer context.Context) {
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
func (f *stageCohortFixture) importMove(t *testing.T, importer context.Context, deal ids.DealID, to ids.UUID) {
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

// Only a move made in Margince starts an entry. An entry whose next move was
// imported, or predates provenance, is history_partial and stays out of the
// rate.
func TestStageConversionCountsOnlyMovesMadeInMargince(t *testing.T) {
	f := newStageCohortFixture(t)
	s0, s1 := f.stage(0), f.stage(1)
	rep1 := f.env.Rep1
	importer := f.importer(t)
	start := f.at

	// Five hand moves, the least a stage needs to show a rate.
	for range 5 {
		f.deal(t, rep1, s0, s1)
	}
	f.importWon(t, importer)
	movedOnByImport := f.deal(t, rep1, s0)
	f.importMove(t, importer, movedOnByImport, s1)
	legacyExit := f.deal(t, rep1, s0, s1)
	f.forgetProvenance(t, legacyExit, &s1)

	interval := crmcontracts.ReportingWindow{StartAt: start, EndAt: f.at.Add(time.Hour)}
	at := f.at.Add(2 * time.Hour)
	result := f.evaluate(t, newReportingService(f.env.Pool, func() time.Time { return at }), crmcontracts.ReportingScope{Kind: "workspace"}, interval)
	rows := cohortRows(t, result)

	first := rows[s0.String()]
	if first.observations != 7 || first.outcomes[cohortAdvanced] != 5 || first.outcomes[cohortHistoryPartial] != 2 || first.outcomes[cohortWon] != 0 {
		t.Fatalf("first stage: %+v, want the hand moves advanced and two exits not made here; the imported win is no entry", first)
	}
	second := rows[s1.String()]
	if second.observations != 5 || second.outcomes[cohortUnresolved] != 5 {
		t.Fatalf("second stage: %+v, want only the hand moves' open entries", second)
	}
	if metric := result.Metrics[0]; metric.Numerator == nil || *metric.Numerator != 5 || *metric.Denominator != 5 || metric.Coverage.Status != reportingPartial {
		t.Fatalf("stage conversion: %+v, want 5 of 5, partial for the exits not made here", metric)
	}
}

// A deal whose moves all predate provenance starts no entry, and the reading
// says its history is incomplete.
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
	if first.observations != 5 || first.outcomes[cohortAdvanced] != 5 {
		t.Fatalf("first stage: %+v, want the five moves made with provenance", first)
	}
	if metric := result.Metrics[0]; metric.Coverage.Status != reportingPartial {
		t.Fatalf("stage conversion coverage: %+v, want partial for the deal left out", metric.Coverage)
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
