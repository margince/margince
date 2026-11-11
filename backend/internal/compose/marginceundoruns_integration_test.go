// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"errors"
	"reflect"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func correctionUndoFixture(t *testing.T) (*dealVerdictEnv, marginceChangeHandlers, []crmcontracts.UndoMarginceChangeItem) {
	t.Helper()
	v := setupDealVerdict(t)
	v.run(t)
	h := newMarginceChangeHandlers(v.Pool)
	summary, err := h.deals.PendingChangesOwnedBy(v.owner(t), nil)
	if err != nil || len(summary.Bundles) != 1 {
		t.Fatalf("pending changes = %+v, %v", summary, err)
	}
	bundle := summary.Bundles[0]
	items := []crmcontracts.UndoMarginceChangeItem{{DealId: crmcontracts.Id(bundle.DealID.UUID), Version: bundle.Version, AuditLogIds: wireUUIDs(bundle.AuditLogIDs)}}
	return v, h, items
}

func TestBulkCorrectionUndoReplaysItsCommittedResults(t *testing.T) {
	v, h, items := correctionUndoFixture(t)
	id, ctx := ids.NewV7(), v.owner(t)
	first, err := h.undoRun(ctx, id, items)
	if err != nil || len(first.Results) != 1 || first.Results[0].Outcome != crmcontracts.UndoMarginceChangeResultOutcomeUndone {
		t.Fatalf("first Undo = %+v, %v", first, err)
	}
	audits := v.WsScalar(t, "SELECT count(*) FROM audit_log WHERE batch_id=$1", id)
	replayed, err := h.undoRun(ctx, id, items)
	if err != nil || !reflect.DeepEqual(first, replayed) {
		t.Fatalf("replayed Undo = %+v, %v; first = %+v", replayed, err, first)
	}
	if got := v.WsScalar(t, "SELECT count(*) FROM audit_log WHERE batch_id=$1", id); got != audits || audits == "0" {
		t.Fatalf("audit count = %s, want unchanged nonzero %s", got, audits)
	}
	items[0].Version++
	if _, err := h.undoRun(ctx, id, items); !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("same run with different items = %v, want conflict", err)
	}
	err = database.WithWorkspaceTx(ctx, v.Pool, func(tx pgx.Tx) error {
		_, err := readBulkOperation(ctx, tx, id)
		return err
	})
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("general bulk reader exposed a correction Undo: %v", err)
	}
}

func TestSavedCorrectionUndoChecksItsReaderAndCurrentDealScope(t *testing.T) {
	v, h, items := correctionUndoFixture(t)
	id, ctx := ids.NewV7(), v.owner(t)
	if _, err := h.undoRun(ctx, id, items); err != nil {
		t.Fatal(err)
	}
	if _, err := h.undoRun(v.Admin(), id, items); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("another human resumed the run: %v", err)
	}
	adminRead, err := h.readCorrectionUndoRuns(v.Admin(), []ids.UUID{id})
	if err != nil || len(adminRead) != 1 || len(adminRead[0].Results) != 1 {
		t.Fatalf("administrator read = %+v, %v", adminRead, err)
	}
	owner := ids.From[ids.UserKind](v.Rep2)
	if _, err := v.Deals.UpdateDeal(v.Admin(), ids.From[ids.DealKind](v.deal), deals.UpdateDealInput{OwnerID: &owner}); err != nil {
		t.Fatal(err)
	}
	saved, err := h.readCorrectionUndoRuns(v.owner(t), []ids.UUID{id})
	if err != nil || len(saved) != 1 || !saved[0].Complete || len(saved[0].Results) != 0 || len(saved[0].Items) != 0 {
		t.Fatalf("read after losing access = %+v, %v", saved, err)
	}
	replayed, err := h.undoRun(v.owner(t), id, items)
	if err != nil || len(replayed.Results) != 0 {
		t.Fatalf("replay after losing access = %+v, %v", replayed, err)
	}
}

func TestSavingABulkUndoResultAndItsReversalRollBackTogether(t *testing.T) {
	v, h, items := correctionUndoFixture(t)
	id, ctx := ids.NewV7(), v.owner(t)
	if err := h.startCorrectionUndoRun(ctx, id, items); err != nil {
		t.Fatal(err)
	}
	_, err := integration.SchemaPool(t).Exec(ctx, `CREATE FUNCTION refuse_undo_result() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'result unavailable'; END $$;
		CREATE TRIGGER refuse_undo_result BEFORE UPDATE ON bulk_operation FOR EACH ROW EXECUTE FUNCTION refuse_undo_result()`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.undoRun(ctx, id, items); err == nil {
		t.Fatal("the failed result write was accepted")
	}
	if got := v.WsScalar(t, "SELECT count(*) FROM deal_correction WHERE deal_id=$1 AND reversed_at IS NOT NULL", v.deal); got != "0" {
		t.Fatalf("reversed corrections after failed result = %s", got)
	}
	if got := v.WsScalar(t, "SELECT count(*) FROM audit_log WHERE batch_id=$1", id); got != "0" {
		t.Fatalf("Undo audit rows after failed result = %s", got)
	}
	if _, err := integration.SchemaPool(t).Exec(ctx, "DROP TRIGGER refuse_undo_result ON bulk_operation; DROP FUNCTION refuse_undo_result()"); err != nil {
		t.Fatal(err)
	}
	answer, err := h.undoRun(ctx, id, items)
	if err != nil || len(answer.Results) != 1 || answer.Results[0].Outcome != crmcontracts.UndoMarginceChangeResultOutcomeUndone {
		t.Fatalf("resumed Undo = %+v, %v", answer, err)
	}
}

func TestADurableUndoRefusedForALaterStageKeepsTheWholeDeal(t *testing.T) {
	v := setupBundle(t)
	v.moveOn(t)
	h := newMarginceChangeHandlers(v.Pool)
	summary, err := h.deals.PendingChangesOwnedBy(v.owner(t), nil)
	if err != nil || len(summary.Bundles) != 1 {
		t.Fatalf("pending = %+v, %v", summary, err)
	}
	bundle := summary.Bundles[0]
	before := v.WsScalar(t, "SELECT row_to_json(d)::text FROM deal d WHERE id=$1", v.deal)
	answer, err := h.undoRun(v.owner(t), ids.NewV7(), []crmcontracts.UndoMarginceChangeItem{{
		DealId: crmcontracts.Id(v.deal), Version: bundle.Version, AuditLogIds: wireUUIDs(bundle.AuditLogIDs),
	}})
	if err != nil || len(answer.Results) != 1 || answer.Results[0].Outcome != crmcontracts.UndoMarginceChangeResultOutcomeRefused || len(answer.Results[0].Kept) != 3 {
		t.Fatalf("refused Undo = %+v, %v", answer, err)
	}
	if after := v.WsScalar(t, "SELECT row_to_json(d)::text FROM deal d WHERE id=$1", v.deal); after != before {
		t.Fatalf("refused bundle changed the deal: before %s, after %s", before, after)
	}
	if got := v.WsScalar(t, "SELECT count(*) FROM deal_correction WHERE deal_id=$1 AND reversed_at IS NOT NULL", v.deal); got != "0" {
		t.Fatalf("refused bundle reversed %s corrections", got)
	}
}

func TestConcurrentBulkCorrectionUndoRequestsShareTheirCommittedResult(t *testing.T) {
	v, h, items := correctionUndoFixture(t)
	id, ctx := ids.NewV7(), v.owner(t)
	expected := v.WsScalar(t, "SELECT count(*) FROM deal_correction WHERE deal_id=$1", v.deal)
	if err := h.startCorrectionUndoRun(ctx, id, items); err != nil {
		t.Fatal(err)
	}
	type answer struct {
		result crmcontracts.UndoMarginceChangesResult
		err    error
	}
	answers := make(chan answer, 2)
	start := make(chan struct{})
	for range 2 {
		go func() {
			<-start
			result, err := h.undoRun(ctx, id, items)
			answers <- answer{result, err}
		}()
	}
	close(start)
	first, second := <-answers, <-answers
	if first.err != nil || second.err != nil || !reflect.DeepEqual(first.result, second.result) {
		t.Fatalf("concurrent results = %+v / %+v", first, second)
	}
	if got := v.WsScalar(t, "SELECT count(*) FROM deal_correction WHERE deal_id=$1 AND reversed_at IS NOT NULL", v.deal); got != expected || expected == "0" {
		t.Fatalf("concurrent Undo reversed %s corrections, want %s", got, expected)
	}
}
