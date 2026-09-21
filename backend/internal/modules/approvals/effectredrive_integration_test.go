// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package approvals

// A decision whose effect failed can be decided again, and the second decision
// runs the work the first one released.
//
// The trap this closes: runDecisionEffect runs AFTER the decision commits, so a
// failing effect leaves the row approved with consumed_at NULL — and deciding
// again used to answer already-decided. Nothing else re-drives an effect, so a
// transient failure at that one moment stranded the approval permanently: the
// human said yes, the work never ran, and nothing they could do made it run.

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestADecisionWhoseEffectFailedIsRedrivenByDecidingAgain(t *testing.T) {
	e := setupStaging(t)
	// The effect fails once and then works — a transient fault, which is the
	// case this exists for. A permanently broken effect is told about twice and
	// that is the honest answer; what must not happen is the FIRST answer
	// becoming the last one available.
	runs := 0
	e.svc.WithEffect(kindSiteLead, func(ctx context.Context, id ids.ApprovalID, _ json.RawMessage, diffHash string) error {
		runs++
		if runs == 1 {
			return errors.New("the capture sink refused this lead: relation lead_intake, host db-3")
		}
		return e.redeems(ctx, id, diffHash)
	})
	ctx := e.asHumanWith(decidesEverything())
	company := e.company(t)
	id := e.stageInto(ctx, t, ids.NewV7(), company, kindSiteLead, "lead-anna")

	if _, err := e.svc.Decide(ctx, id, true, nil); err == nil {
		t.Fatal("the failing effect's error was swallowed — the decider was told it worked")
	}
	if at, _ := e.effectFailureOf(t, id); at == nil {
		t.Fatal("the first decision left no failure mark, so this test is not about a stranded row")
	}

	// Deciding again: the same verdict, on a row that already carries it.
	if _, err := e.svc.Decide(ctx, id, true, nil); err != nil {
		t.Fatalf("deciding an approved row whose effect never ran answered %v — a human who said yes has "+
			"no other way to make the work happen, so this refusal is where the approval dies", err)
	}
	if runs != 2 {
		t.Errorf("the effect ran %d time(s), want 2 — the second decision was accepted without re-driving "+
			"the work it exists to release", runs)
	}
	if status := e.statusOf(t, id); status != approvalStatusApproved {
		t.Errorf("stored status = %s, want approved — a re-drive decides nothing new", status)
	}
}

// A genuinely finished approval still refuses, and this is the assertion that
// makes the one above safe rather than merely convenient.
//
// Re-driving is safe because every effect REDEEMS — it consumes the approval in
// the same transaction as its write — so consumed_at is a fact about whether
// the work ran rather than a guess. An effect that got as far as writing left
// it set, and this must then be a conflict: a second decide that re-ran a
// consumed approval would apply its write twice, and several of these effects
// are not remotely idempotent.
func TestAConsumedApprovalStillRefusesASecondDecision(t *testing.T) {
	e := setupStaging(t)
	runs := 0
	e.svc.WithEffect(kindSiteLead, func(ctx context.Context, id ids.ApprovalID, _ json.RawMessage, diffHash string) error {
		runs++
		return e.redeems(ctx, id, diffHash)
	})
	ctx := e.asHumanWith(decidesEverything())
	company := e.company(t)
	id := e.stageInto(ctx, t, ids.NewV7(), company, kindSiteLead, "lead-anna")

	if _, err := e.svc.Decide(ctx, id, true, nil); err != nil {
		t.Fatalf("the first decision: %v", err)
	}

	_, err := e.svc.Decide(ctx, id, true, nil)

	var decided *AlreadyDecidedError
	if !errors.As(err, &decided) {
		t.Fatalf("deciding a consumed approval answered %v, want already-decided — its work landed, and "+
			"running it again would write it twice", err)
	}
	if runs != 1 {
		t.Errorf("the effect ran %d time(s), want 1 — a consumed approval was re-driven", runs)
	}
}

// A REJECTED row is not re-drivable however its decline went. A decline runs
// inside the decision transaction, so a failed one rolled the decision back and
// left the row pending; a rejected row therefore always means the decline
// succeeded, and an approve arriving on one is a real conflict about the
// verdict rather than a retry of the work.
func TestARejectedApprovalIsNotRedrivenByApprovingIt(t *testing.T) {
	e := setupStaging(t)
	runs := 0
	e.svc.WithEffect(kindSiteLead, func(context.Context, ids.ApprovalID, json.RawMessage, string) error {
		runs++
		return nil
	})
	ctx := e.asHumanWith(decidesEverything())
	company := e.company(t)
	id := e.stageInto(ctx, t, ids.NewV7(), company, kindSiteLead, "lead-anna")

	reason := "not this quarter"
	if _, err := e.svc.Decide(ctx, id, false, &reason); err != nil {
		t.Fatalf("rejecting: %v", err)
	}

	_, err := e.svc.Decide(ctx, id, true, nil)

	var decided *AlreadyDecidedError
	if !errors.As(err, &decided) {
		t.Fatalf("approving a rejected row answered %v, want already-decided", err)
	}
	if runs != 0 {
		t.Errorf("the effect ran %d time(s) for a row nobody approved", runs)
	}
}

// redeems is what a real effect does before it writes: consume the approval,
// once, so the row records that the work ran.
//
// Through the service rather than an UPDATE of consumed_at, because the
// single-use redemption is exactly the mechanism the re-drive rests on — a
// stub that set the column itself would prove the predicate reads a column,
// not that the column means what production makes it mean.
func (e *stagingEnv) redeems(ctx context.Context, id ids.ApprovalID, diffHash string) error {
	_, _, err := e.svc.Redeem(ctx, id, kindSiteLead, diffHash)
	return err
}
