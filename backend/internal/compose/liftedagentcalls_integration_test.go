// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"encoding/json"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// An approval a colleague's agent staged for a tool that no longer waits is
// withdrawn, because the agent's retry already ran the call. One for a tool that
// still waits, one staged with no passport, and one a scheduled run is paused on
// all stay.
func TestAPendingAgentCallThatNoLongerWaitsIsWithdrawn(t *testing.T) {
	a := newApprovalNotifyEnv(t)
	passport := a.e.SeedPassport(t, integration.OwnerConn(t), "lifted call probe")
	stage := func(kind string, onPassport bool) ids.ApprovalID {
		t.Helper()
		id, _ := a.stage(a.proposerCtx(a.e.AdminUser), t, approvals.StageInput{
			Kind: kind, ProposedChange: json.RawMessage(`{"n":"` + kind + `"}`), DiffHash: "h-" + kind, Summary: kind,
		})
		if onPassport {
			a.e.WsExec(t, `UPDATE approval SET passport_id = $2 WHERE id = $1`, id, passport)
		}
		return id
	}
	lifted := stage("merge_tags", true)
	waits := stage("enrich", true)
	automatic := stage("invite_meeting", false)
	paused := stage("progress_deal", true)
	a.e.WsExec(t, `INSERT INTO agent_run (agent_spec, goal, trigger_ref, status, approval_id, pending)
		VALUES ('probe', 'move a deal', 'lifted-call-probe', 'awaiting_approval', $1, '{}')`, paused)

	if changed := withdrawLiftedAgentCalls(t, a.e); changed != 1 {
		t.Fatalf("the data task withdrew %d approvals, want the one that no longer waits", changed)
	}
	for id, want := range map[ids.ApprovalID]string{lifted: "expired", waits: "pending", automatic: "pending", paused: "pending"} {
		if got := a.e.WsScalar(t, `SELECT status FROM approval WHERE id = $1`, id); got != want {
			t.Errorf("approval %s is %s, want %s", id, got, want)
		}
	}
	if changed := withdrawLiftedAgentCalls(t, a.e); changed != 0 {
		t.Errorf("a second run withdrew %d more", changed)
	}
}

// withdrawLiftedAgentCalls runs the withdrawal step to the end, one row a batch.
func withdrawLiftedAgentCalls(t *testing.T, e *integration.Env) int {
	t.Helper()
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
	batch := approvals.WithdrawLiftedAgentCallsTx(liftedAgentCallKinds())
	in := datatask.BatchIn{Limit: 1, RunID: ids.NewV7()}
	changed := 0
	for range 100 {
		var out datatask.BatchOut
		if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
			var err error
			out, err = batch(ctx, tx, in)
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
	t.Fatal("the withdrawal did not finish in 100 batches")
	return changed
}
