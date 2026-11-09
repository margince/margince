// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// A pending stage-move card expires through the approvals writer, with an
// audit row. A decided card and a card of another kind stay as they are.
func TestThePendingStageCardsExpire(t *testing.T) {
	e := integration.Setup(t)
	pipeline, open, _ := integration.DealFixture(t, e)
	deal := e.SeedDeal(t, "Warehouse rollout", pipeline, open, &e.AdminUser)
	svc := approvals.NewService(e.DB())
	stage := func(kind, hash string) ids.ApprovalID {
		id, err := svc.Stage(e.Admin(), approvals.StageInput{
			Kind: kind, ProposedChange: []byte(`{}`), DiffHash: hash,
			TargetType: approvalTargetDeal, TargetID: deal, Summary: "Move the deal",
		})
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	pending := []ids.ApprovalID{stage("stage_progression", "a"), stage("stage_progression", "b")}
	decided := stage("stage_progression", "c")
	e.WsExec(t, `UPDATE approval SET status = 'rejected', decided_at = now(), decided_by = $2 WHERE id = $1`, decided, e.AdminUser)
	other := stage("advance_deal", "d")

	if changed := expireRetiredStageCards(t, e); changed != 2 {
		t.Fatalf("the task expired %d cards, want the 2 pending stage-move cards", changed)
	}
	for _, id := range pending {
		if n := e.WsCount(t, `SELECT count(*) FROM approval WHERE id = $1 AND status = 'expired'`, id); n != 1 {
			t.Errorf("pending card %s is not expired", id)
		}
		if n := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type = 'approval' AND entity_id = $1
			 AND after->>'status' = 'expired'`, id); n != 1 {
			t.Errorf("card %s has %d audit rows for the expiry, want one", id, n)
		}
	}
	if n := e.WsCount(t, `SELECT count(*) FROM approval WHERE id = $1 AND status = 'rejected'`, decided); n != 1 {
		t.Error("the task changed a card a human had decided")
	}
	if n := e.WsCount(t, `SELECT count(*) FROM approval WHERE id = $1 AND status = 'pending'`, other); n != 1 {
		t.Error("the task expired a card of another kind")
	}
	if changed := expireRetiredStageCards(t, e); changed != 0 {
		t.Errorf("a second run expired %d cards, want none", changed)
	}
}

// expireRetiredStageCards runs the expiry step to the end, one row a batch.
func expireRetiredStageCards(t *testing.T, e *integration.Env) int {
	t.Helper()
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
	in := datatask.BatchIn{Limit: 1, RunID: ids.NewV7()}
	changed := 0
	for range 100 {
		var out datatask.BatchOut
		if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
			var err error
			out, err = approvals.ExpireRetiredStageCardsTx(ctx, tx, in)
			return err
		}); err != nil {
			t.Fatal(err)
		}
		changed += out.Changed
		if out.Done {
			return changed
		}
		in.Cursor = out.Cursor
	}
	t.Fatal("the expiry did not finish")
	return changed
}
