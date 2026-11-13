// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A follow-up card whose deal got a next step, or closed, while it waited is
// withdrawn with the reason when its owner approves it. No second next step is
// written.
func TestAFollowUpNoLongerNeededIsWithdrawnWithItsReason(t *testing.T) {
	for name, meanwhile := range map[string]func(e *reconcileEnv, deal ids.UUID){
		"a meeting was booked": func(e *reconcileEnv, deal ids.UUID) {
			e.seedMeeting(t, deal, "Next Thursday", -72, 3600, "booked")
		},
		"the deal closed": func(e *reconcileEnv, deal ids.UUID) {
			e.WsExec(t, `UPDATE deal SET status = 'won', closed_at = now() WHERE id = $1`, deal)
		},
	} {
		t.Run(name, func(t *testing.T) {
			e := setupReconcile(t)
			deal := e.SeedDeal(t, "Moved on", e.pipeline, e.open, &e.Rep1)
			call := e.seedInteraction(t, deal, "call", "Discovery", 1)
			id, _ := e.stageLegacyFollowUp(t, deal, call)
			meanwhile(e, deal)

			_, err := e.svc.Decide(e.As(e.Rep1, []ids.UUID{e.Team1}, reconcilePerms), id, true, nil)
			if _, ok := errors.AsType[*approvals.ObsoleteError](err); !ok {
				t.Fatalf("approving a follow-up no longer needed answered %v, want a no-longer-needed refusal", err)
			}
			assertCardClosedWithNothingWritten(t, e.Env, id.UUID, `SELECT count(*) FROM activity WHERE source = 'overnight-reconcile'`)
		})
	}
}

// The effect asks again as it writes, so a next step logged after the precheck
// still stops the follow-up. Here no precheck is wired, so only the effect can.
// The card is spent with nothing written, and the decider is told why.
func TestTheFollowUpEffectWritesNothingOnceTheDealHasANextStep(t *testing.T) {
	e := setupReconcile(t)
	deal := e.SeedDeal(t, "Booked meanwhile", e.pipeline, e.open, &e.Rep1)
	call := e.seedInteraction(t, deal, "call", "Discovery", 1)
	id, _ := e.stageLegacyFollowUp(t, deal, call)
	e.seedMeeting(t, deal, "Next Thursday", -72, 3600, "booked")

	effectOnly := approvals.NewService(e.DB())
	effectOnly.WithEffect(deals.FollowUpReconcileKind,
		followUpConfirmEffect(effectOnly, e.Activities, func() time.Time { return clocktest.Now(t) }))
	_, err := effectOnly.Decide(e.As(e.Rep1, []ids.UUID{e.Team1}, reconcilePerms), id, true, nil)
	if _, ok := errors.AsType[*approvals.ObsoleteError](err); !ok {
		t.Fatalf("the decider was answered %v, want the no-longer-needed reason", err)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM activity WHERE source = 'overnight-reconcile'`); n != 0 {
		t.Errorf("the effect wrote %d follow-up(s) for a deal that has a next step", n)
	}
	// Spent and settled, so nothing re-drives it and no failed-effect lane
	// tells the decider their approval broke.
	if n := e.WsCount(t, `SELECT count(*) FROM approval WHERE id = $1
		AND consumed_at IS NOT NULL AND effect_ran_at IS NOT NULL`, id); n != 1 {
		t.Error("the card is not marked spent with its effect run")
	}
}

// A promise card is withdrawn with the reason when the promise was dismissed
// since. That holds for a card resting on a claim and for one resting on the
// commitment decision.
func TestAPromiseNoLongerOpenIsWithdrawnWithItsReason(t *testing.T) {
	for name, close := range map[string]func(e *integration.Env, id ids.ApprovalID, change map[string]any){
		"its claim was dismissed": func(e *integration.Env, _ ids.ApprovalID, _ map[string]any) {
			e.WsExec(t, `UPDATE conversation_claim SET status = 'dismissed' WHERE kind = 'commitment_ours'`)
		},
		"its decision is no longer current": func(e *integration.Env, id ids.ApprovalID, change map[string]any) {
			delete(change, "claim_id")
			delete(change, "claim_ids")
			raw, err := json.Marshal(change)
			if err != nil {
				t.Fatal(err)
			}
			e.WsExec(t, `UPDATE approval SET proposed_change = $2 WHERE id = $1`, id, raw)
			e.WsExec(t, `UPDATE activity_commitment_admission SET current_work = false`)
		},
	} {
		t.Run(name, func(t *testing.T) {
			e, id, change := stagedPromiseCard(t)
			close(e, id, change)
			promiser := e.As(e.Rep2, []ids.UUID{e.Team1}, integration.AdminPerms)
			_, err := NewApprovalsServiceWithEffects(e.Pool).Decide(promiser, id, true, nil)
			if _, ok := errors.AsType[*approvals.ObsoleteError](err); !ok {
				t.Fatalf("accepting a promise no longer open answered %v, want a no-longer-needed refusal", err)
			}
			assertCardClosedWithNothingWritten(t, e, id.UUID, `SELECT count(*) FROM activity WHERE kind = 'task'`)
		})
	}
}

// assertCardClosedWithNothingWritten checks the card left the queue as expired,
// not approved, and the work query finds nothing.
func assertCardClosedWithNothingWritten(t *testing.T, e *integration.Env, card ids.UUID, work string) {
	t.Helper()
	if status := e.WsScalar(t, `SELECT status FROM approval WHERE id = $1`, card); status != "expired" {
		t.Errorf("the card is %q, want it withdrawn (expired)", status)
	}
	if n := e.WsCount(t, work); n != 0 {
		t.Errorf("%d task(s) written for work no longer needed", n)
	}
}
