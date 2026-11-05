// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// An automatic apply decides to apply a proposal, then records the decision
// and runs its effect, each in a transaction of its own. An owner who leaves,
// loses the deal grant or turns the switch off before either one gets no
// change.

import (
	"context"
	"encoding/json"
	"errors"
	"maps"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// ownerChanges are the ways an owner stops allowing an automatic apply, and
// the control: an owner who changes nothing gets the change.
var ownerChanges = map[string]func(e *closeDateEnv, t *testing.T){
	"nothing changes": func(*closeDateEnv, *testing.T) {},
	"suspended": func(e *closeDateEnv, t *testing.T) {
		e.WsExec(t, `UPDATE app_user SET status = 'suspended' WHERE id = $1`, e.Rep1)
	},
	"role removed": func(e *closeDateEnv, t *testing.T) {
		e.WsExec(t, `DELETE FROM role_assignment WHERE user_id = $1`, e.Rep1)
	},
	"switch turned off": func(e *closeDateEnv, t *testing.T) { e.optOutOfCorrections(t, e.Rep1) },
}

// decisionChanges adds a handover to a colleague, while the old owner can
// still edit the deal. An automatic apply acts for the owner whose switch it
// read, never for whoever holds the deal when it decides. In the effect's
// transaction, the version the approval pinned refuses a handover first.
func decisionChanges() map[string]func(e *closeDateEnv, t *testing.T) {
	changes := maps.Clone(ownerChanges)
	changes["handed to a colleague"] = func(e *closeDateEnv, t *testing.T) {
		e.GrantRole(t, e.Rep1, "admin")
		e.WsExec(t, `UPDATE deal SET owner_id = $1 WHERE owner_id = $2`, e.Rep2, e.Rep1)
	}
	return changes
}

func TestAnOwnerWhoLeavesBeforeTheDecisionGetsNoAutomaticChange(t *testing.T) {
	for name, change := range decisionChanges() {
		t.Run(name, func(t *testing.T) {
			e := setupCloseDate(t)
			deal, approvalID := e.stageServerCard(t)
			applier := autoApplier{pool: e.Pool, svc: e.svc, users: identity.NewService(e.Pool)}
			decision, ownerCtx, ok, err := applier.decide(e.applyCtx(), approvalID)
			if err != nil || !ok {
				t.Fatalf("decide = %v, %v: want the proposal due to apply before the owner changes", ok, err)
			}
			change(e, t)

			applied, err := applier.applyAs(ownerCtx, decision)
			if err != nil {
				t.Fatal(err)
			}
			if name == "nothing changes" {
				e.applied(t, applied, approvalID, deal)
				return
			}
			if applied {
				t.Fatal("applied, want nothing applied")
			}
			if status, _ := e.approvalState(t, approvalID); status != "pending" {
				t.Errorf("the approval is %s, want it still pending for a contact", status)
			}
			e.dateUnchanged(t, deal)
		})
	}
}

func TestAnOwnerWhoLeavesBeforeTheEffectGetsNoAutomaticChange(t *testing.T) {
	for name, change := range ownerChanges {
		t.Run(name, func(t *testing.T) {
			e := setupCloseDate(t)
			deal, approvalID := e.stageServerCard(t)
			// The production effect, with the owner's change committed after the
			// decision commits and before the effect opens its transaction.
			svc := approvals.NewService(e.DB())
			effect := closeDateConfirmEffect(svc, deals.NewStore(e.DB(), DealsInstallation()))
			svc.WithEffect(deals.CloseDateCorrectionKind,
				func(ctx context.Context, id ids.ApprovalID, raw json.RawMessage, hash string) error {
					change(e, t)
					return effect(ctx, id, raw, hash)
				})
			applier := autoApplier{pool: e.Pool, svc: svc, users: identity.NewService(e.Pool)}
			decision, ownerCtx, ok, err := applier.decide(e.applyCtx(), approvalID)
			if err != nil || !ok {
				t.Fatalf("decide = %v, %v: want the proposal due to apply", ok, err)
			}

			applied, err := applier.applyAs(ownerCtx, decision)
			if err != nil {
				t.Fatal(err)
			}
			if name == "nothing changes" {
				e.applied(t, applied, approvalID, deal)
				return
			}
			if applied {
				t.Fatal("applied, want nothing applied")
			}
			if status, consumed := e.approvalState(t, approvalID); status != "approved" || consumed {
				t.Errorf("the approval is %s (consumed %v), want it decided with its effect not run", status, consumed)
			}
			e.dateUnchanged(t, deal)
		})
	}
}

// stageServerCard stages a close-date card on a deal of Rep1's the way a
// server sweep does, so the auto-apply pass may pick it up.
func (e *closeDateEnv) stageServerCard(t *testing.T) (ids.UUID, ids.ApprovalID) {
	t.Helper()
	deal := e.seedSweepDeal(t, "Applied unasked", e.late, nil, new(30), 1)
	proposal, err := json.Marshal(deals.CloseDateCorrection{
		DealID: ids.From[ids.DealKind](deal), ExpectedCloseDate: today().AddDate(0, 0, 45).Format(time.DateOnly),
		Asking: deals.AskingIsThisDateRight,
	})
	if err != nil {
		t.Fatal(err)
	}
	stager := principal.SystemActing(principal.WithWorkspaceID(context.Background(), e.WS), closeDateSweepActor)
	id, err := e.svc.Stage(stager, approvals.StageInput{
		Kind: deals.CloseDateCorrectionKind, ProposedChange: proposal, DiffHash: "h-" + ids.NewV7().String(),
		TargetType: "deal", TargetID: deal, Summary: "the close date has passed",
	})
	if err != nil {
		t.Fatalf("staging the card: %v", err)
	}
	return deal, id
}

// applyCtx is what the auto-apply job binds: the workspace and no principal.
func (e *closeDateEnv) applyCtx() context.Context {
	return principal.WithCorrelationID(principal.WithWorkspaceID(context.Background(), e.WS), ids.NewV7())
}

func (e *closeDateEnv) approvalState(t *testing.T, id ids.ApprovalID) (string, bool) {
	t.Helper()
	var status string
	var consumed bool
	if err := e.owner.QueryRow(context.Background(),
		`SELECT status, consumed_at IS NOT NULL FROM approval WHERE id = $1`, id).Scan(&status, &consumed); err != nil {
		t.Fatal(err)
	}
	return status, consumed
}

// applied asserts the control: the decision, its effect and the new date.
func (e *closeDateEnv) applied(t *testing.T, applied bool, id ids.ApprovalID, deal ids.UUID) {
	t.Helper()
	want := today().AddDate(0, 0, 45)
	status, consumed := e.approvalState(t, id)
	if got := e.readSwept(t, deal); !applied || status != "approved" || !consumed ||
		got.expectedClose == nil || !got.expectedClose.Equal(want) {
		t.Fatalf("applied = %v, approval %s (consumed %v), close date %v: want the card applied and the date %s",
			applied, status, consumed, got.expectedClose, want.Format(time.DateOnly))
	}
}

func (e *closeDateEnv) dateUnchanged(t *testing.T, deal ids.UUID) {
	t.Helper()
	want := today().AddDate(0, 0, 30)
	if got := e.readSwept(t, deal); got.expectedClose == nil || !got.expectedClose.Equal(want) {
		t.Errorf("close date = %v, want %s left alone", got.expectedClose, want.Format(time.DateOnly))
	}
}

// observedPolicy is the production binding, with the test looking in before
// each bind and after each hold on the target.
type observedPolicy struct {
	approvals.PolicyActor
	beforeBind func(ctx context.Context, tx pgx.Tx) error
	afterHold  func(holds int)
	holds      int
}

func (o *observedPolicy) Bind(ctx context.Context, tx pgx.Tx) (context.Context, bool, error) {
	if o.beforeBind != nil {
		if err := o.beforeBind(ctx, tx); err != nil {
			return nil, false, err
		}
	}
	return o.PolicyActor.Bind(ctx, tx)
}

func (o *observedPolicy) HoldTarget(ctx context.Context, tx pgx.Tx) (bool, error) {
	holds, err := o.PolicyActor.HoldTarget(ctx, tx)
	o.holds++
	if o.afterHold != nil {
		o.afterHold(o.holds)
	}
	return holds, err
}

// applyObserved decides one fresh card and applies it through policy, which
// wraps the production binding.
func (e *closeDateEnv) applyObserved(t *testing.T, policy *observedPolicy) (ids.UUID, ids.ApprovalID) {
	t.Helper()
	deal, approvalID := e.stageServerCard(t)
	applier := autoApplier{pool: e.Pool, svc: e.svc, users: identity.NewService(e.Pool)}
	decision, ownerCtx, ok, err := applier.decide(e.applyCtx(), approvalID)
	if err != nil || !ok {
		t.Fatalf("decide = %v, %v: want the proposal due to apply", ok, err)
	}
	policy.PolicyActor = applier.ownerNow(decision)
	if _, err := e.svc.ApplyUnderPolicy(ownerCtx, approvalID, policy); err != nil {
		t.Fatal(err)
	}
	return deal, approvalID
}

// An automatic apply binds the owner before the decision locks the approval,
// and before the effect's redemption locks anything. Neither can then deadlock
// with a member's removal.
func TestAnAutomaticApplyBindsTheOwnerBeforeItLocksARecord(t *testing.T) {
	e := setupCloseDate(t)
	var locksAtBind []int
	policy := &observedPolicy{beforeBind: func(ctx context.Context, tx pgx.Tx) error {
		held, err := rowLocksHeld(ctx, tx)
		locksAtBind = append(locksAtBind, held)
		return err
	}}

	deal, approvalID := e.applyObserved(t, policy)

	e.applied(t, true, approvalID, deal)
	if len(locksAtBind) != 2 || locksAtBind[0] != 0 || locksAtBind[1] != 0 {
		t.Errorf("record locks held at each owner bind = %v, want [0 0]: the decision's and the effect's", locksAtBind)
	}
}

// waitsForTheApply runs change while the effect's transaction is open, and
// reports whether it had to wait for that transaction. The bound only ends a
// wait; a change that does not wait commits well inside it.
func waitsForTheApply(t *testing.T, change func(ctx context.Context) error) bool {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	err := change(ctx)
	if err != nil && !errors.Is(err, context.DeadlineExceeded) && ctx.Err() == nil {
		t.Fatalf("the change failed for another reason: %v", err)
	}
	return err != nil
}

// A switch turned off, or a handover, after the effect read the mode and held
// the deal waits for the write. Neither can commit under a write that already
// decided on the state before it.
func TestAChangeToTheOwnerWaitsForAnAutomaticApplyInFlight(t *testing.T) {
	for name, change := range map[string]func(e *closeDateEnv) func(ctx context.Context) error{
		"switch turned off": switchesOff,
		"handed to a colleague": func(e *closeDateEnv) func(ctx context.Context) error {
			return func(ctx context.Context) error {
				_, err := e.Pool.Exec(ctx, `UPDATE deal SET owner_id = $1 WHERE owner_id = $2`, e.Rep2, e.Rep1)
				return err
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			e := setupCloseDate(t)
			if err := e.setSwitch(context.Background(), true); err != nil {
				t.Fatal(err)
			}
			var waited bool
			policy := &observedPolicy{afterHold: func(holds int) {
				if holds == 2 {
					waited = waitsForTheApply(t, change(e))
				}
			}}

			deal, approvalID := e.applyObserved(t, policy)

			if !waited {
				t.Error("the change committed while the effect's transaction was open, want it to wait")
			}
			e.applied(t, true, approvalID, deal)
		})
	}
}

// switchesOff is Rep1 turning close-date corrections off through the real
// writer, which takes the policy lock exclusively.
func switchesOff(e *closeDateEnv) func(ctx context.Context) error {
	return func(ctx context.Context) error { return e.setSwitch(ctx, false) }
}

// setSwitch stores Rep1's close-date switch. Stored on before a test, the
// later switch-off updates the row and checks no key, so only the policy
// lock can make it wait.
func (e *closeDateEnv) setSwitch(ctx context.Context, on bool) error {
	rep := principal.WithWorkspaceID(ctx, e.WS)
	rep = principal.WithActor(rep, principal.Principal{Type: principal.PrincipalHuman,
		ID: principal.HumanIDPrefix + e.Rep1.String(), UserID: e.Rep1})
	_, err := approvals.NewService(e.DB()).SetAutoApply(rep, deals.CloseDateCorrectionKind, on)
	return err
}
