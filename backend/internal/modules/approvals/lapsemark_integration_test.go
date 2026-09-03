// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package approvals

// An approval a human said yes to, that the agent never came back to spend,
// must stop reading as success.
//
// The sweep executes nothing — an agent-minted staging has no server-side
// executor by design (ADR-0055, and serverProposed in decide.go) — so what is
// under test is the mark, and above all WHICH ROWS DO NOT GET IT. The failure
// this closes was silence, and a sweep that marks too much is the same defect
// pointed the other way: an approver told their work did not happen when it did
// goes and does it twice.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// lapsed stages one agent call, approves it as the lending human, and moves the
// service clock past the redemption window without anyone redeeming.
func (e *stagingEnv) lapsed(t *testing.T) ids.ApprovalID {
	t.Helper()
	passport := e.seedPassport(t)
	org := e.seedOrg(t)
	id, err := e.svc.Stage(e.asPassport(passport), e.agentCall(org))
	if err != nil {
		t.Fatalf("staging the agent call: %v", err)
	}
	e.approve(t, id)
	return id
}

// sweeping binds the actor the tick runs as. The sweep refuses everyone else,
// so a test calling it as nobody would be exercising the guard rather than the
// pass — see TestOnlyTheExpirySweepMayMarkLapsedRedemptions for that half.
func (e *stagingEnv) sweeping() context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), e.ws)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalSystem, ID: ExpiryActor,
	})
}

// pastTheWindow moves the service clock beyond the redemption TTL. The offset
// is derived from redemptionTTL rather than written as a number: a test that
// hardcoded one would keep passing if the window changed, while testing a
// window the product no longer has.
func (e *stagingEnv) pastTheWindow() {
	at := time.Now().Add(redemptionTTL).Add(time.Minute)
	e.svc.now = func() time.Time { return at }
}

func TestAnApprovalTheAgentNeverRedeemedIsMarkedRatherThanLeftSilent(t *testing.T) {
	e := setupStaging(t)
	id := e.lapsed(t)

	// Before the window closes there is nothing to say: the agent may still
	// come back, and marking it now would be a false accusation.
	marked, err := e.svc.MarkLapsedRedemptions(e.sweeping())
	if err != nil {
		t.Fatalf("sweeping inside the window: %v", err)
	}
	if marked != 0 {
		t.Fatalf("marked %d approval(s) while the agent could still redeem", marked)
	}
	if at, _ := e.effectFailureOf(t, id); at != nil {
		t.Fatal("an approval still inside its redemption window carries a failure mark")
	}

	e.pastTheWindow()
	marked, err = e.svc.MarkLapsedRedemptions(e.sweeping())
	if err != nil {
		t.Fatalf("sweeping past the window: %v", err)
	}
	if marked != 1 {
		t.Fatalf("marked %d approval(s), want the one nobody redeemed", marked)
	}

	at, sentence := e.effectFailureOf(t, id)
	if at == nil || sentence == nil {
		t.Fatalf("the lapsed approval carries no mark (at=%v sentence=%v) — it reads as success to its approver", at, sentence)
	}
	// The approver's own sentence. It has to say the work did not happen AND
	// what to do, because a human told only "expired" decides again and the row
	// refuses them as already decided.
	if *sentence != lapsedRedemptionSentence {
		t.Errorf("stored sentence = %q, want %q", *sentence, lapsedRedemptionSentence)
	}
	// The status is untouched. A human DID approve this, and rewriting that to
	// something else would put a decision in the record they never made.
	if status := e.statusOf(t, id); status != approvalStatusApproved {
		t.Errorf("stored status = %s, want approved — the mark annotates the decision, it does not replace it", status)
	}
}

func TestTheLapseSweepIsIdempotent(t *testing.T) {
	e := setupStaging(t)
	id := e.lapsed(t)
	e.pastTheWindow()

	if _, err := e.svc.MarkLapsedRedemptions(e.sweeping()); err != nil {
		t.Fatalf("first sweep: %v", err)
	}
	first, _ := e.effectFailureOf(t, id)

	marked, err := e.svc.MarkLapsedRedemptions(e.sweeping())
	if err != nil {
		t.Fatalf("second sweep: %v", err)
	}
	// Not merely "the row still looks right": a re-mark would move the
	// timestamp, and the surface that reads this orders by it, so an unbounded
	// pass would float every old lapse to the top of the reader's attention on
	// every tick.
	if marked != 0 {
		t.Errorf("the second sweep marked %d row(s); a row already marked is not due again", marked)
	}
	again, _ := e.effectFailureOf(t, id)
	if first == nil || again == nil || !first.Equal(*again) {
		t.Errorf("the mark moved between sweeps: %v then %v", first, again)
	}
}

func TestTheLapseSweepLeavesAlone(t *testing.T) {
	// One environment per case: the sweep is installation-wide, so a row seeded
	// for one case would be a candidate in the next.
	t.Run("an approval the agent DID redeem", func(t *testing.T) {
		e := setupStaging(t)
		passport := e.seedPassport(t)
		org := e.seedOrg(t)
		call := e.agentCall(org)
		id, err := e.svc.Stage(e.asPassport(passport), call)
		if err != nil {
			t.Fatalf("staging: %v", err)
		}
		e.approve(t, id)
		if _, _, err := e.svc.Redeem(e.asPassport(passport), id, call.Kind, call.DiffHash); err != nil {
			t.Fatalf("redeeming: %v", err)
		}
		e.pastTheWindow()

		marked, err := e.svc.MarkLapsedRedemptions(e.sweeping())
		if err != nil {
			t.Fatalf("sweeping: %v", err)
		}
		if marked != 0 {
			t.Errorf("marked %d redeemed approval(s) — the agent came back, and what happened after is not this sweep's claim", marked)
		}
		if at, _ := e.effectFailureOf(t, id); at != nil {
			t.Error("a redeemed approval was marked as never carried out")
		}
	})

	t.Run("a SERVER-proposed approval", func(t *testing.T) {
		e := setupStaging(t)
		ctx := e.asHumanWith(decidesEverything())
		org := e.organization(t)
		id := e.stageInto(ctx, t, ids.NewV7(), org, kindSiteLead, "lead-anna")
		if _, err := e.svc.Decide(ctx, id, true, nil); err != nil {
			t.Fatalf("deciding: %v", err)
		}
		e.pastTheWindow()

		marked, err := e.svc.MarkLapsedRedemptions(e.sweeping())
		if err != nil {
			t.Fatalf("sweeping: %v", err)
		}
		// It carries no passport, so its effect ran inside the decision and
		// decide.go owns whether that worked. Marking it here would overwrite a
		// real diagnosis with a guess about an agent that was never involved.
		if marked != 0 {
			t.Errorf("marked %d server-proposed approval(s) — those have an executor, and decide.go already reports on it", marked)
		}
		if at, _ := e.effectFailureOf(t, id); at != nil {
			t.Error("a server-proposed approval was marked as an agent that never came back")
		}
	})

	t.Run("an approval the human DECLINED", func(t *testing.T) {
		e := setupStaging(t)
		passport := e.seedPassport(t)
		org := e.seedOrg(t)
		id, err := e.svc.Stage(e.asPassport(passport), e.agentCall(org))
		if err != nil {
			t.Fatalf("staging: %v", err)
		}
		ctx := e.asHumanWith(decidesEverything())
		if _, err := e.svc.Decide(ctx, id, false, nil); err != nil {
			t.Fatalf("declining: %v", err)
		}
		e.pastTheWindow()

		marked, err := e.svc.MarkLapsedRedemptions(e.sweeping())
		if err != nil {
			t.Fatalf("sweeping: %v", err)
		}
		// A declined row carries decided_at exactly like an approved one, so
		// only the status tells them apart — and nothing lapsed here. The
		// human said no, the work correctly did not happen, and a mark saying
		// it did not happen would read as a malfunction of their own refusal.
		if marked != 0 {
			t.Errorf("marked %d declined approval(s) — nothing was supposed to be carried out", marked)
		}
		if at, _ := e.effectFailureOf(t, id); at != nil {
			t.Error("a declined approval was marked as work that failed to happen")
		}
	})

	t.Run("an approval nobody decided", func(t *testing.T) {
		e := setupStaging(t)
		passport := e.seedPassport(t)
		org := e.seedOrg(t)
		id, err := e.svc.Stage(e.asPassport(passport), e.agentCall(org))
		if err != nil {
			t.Fatalf("staging: %v", err)
		}
		e.pastTheWindow()

		marked, err := e.svc.MarkLapsedRedemptions(e.sweeping())
		if err != nil {
			t.Fatalf("sweeping: %v", err)
		}
		// Undecided is the expiry sweep's subject, and it writes a decision:
		// status, audit row, event. This one writes bookkeeping, and the two
		// must not both answer for the same row.
		if marked != 0 {
			t.Errorf("marked %d undecided approval(s) — an unactioned staging is ExpireDue's to close", marked)
		}
		if at, _ := e.effectFailureOf(t, id); at != nil {
			t.Error("a pending approval was marked as an agent that never came back")
		}
	})
}

// The guard's own half. A bulk write that tells every approver in the workspace
// their decisions were never carried out is not something an authenticated
// caller may trigger — each row would look exactly like a finding the sweep had
// made.
func TestOnlyTheExpirySweepMayMarkLapsedRedemptions(t *testing.T) {
	e := setupStaging(t)
	id := e.lapsed(t)
	e.pastTheWindow()

	if _, err := e.svc.MarkLapsedRedemptions(e.asHumanWith(decidesEverything())); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("a human decider was allowed to run the sweep: %v", err)
	}
	if at, _ := e.effectFailureOf(t, id); at != nil {
		t.Error("the refused call marked a row anyway")
	}
}
