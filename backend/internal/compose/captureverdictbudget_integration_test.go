// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The re-ask budget, over a real backlog.
//
// capture_counterparty_verdict's wall was sized for one model call per sender.
// The asymmetric floor made the second call ordinary rather than exceptional —
// a creating answer between verdictConfidenceFloor and verdictCreateFloor is
// re-asked, and that is the common case on a mailbox of first-time senders — so
// a pass sized for one call each could make two.
//
// What this proves is the property the wall rests on: a pass whose answers are
// ALL borderline still makes at most cap + budget calls.

import (
	"context"
	"log/slog"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// borderlineBacklog seeds senders whose answers all land between the two
// floors: confident enough to mean something, not confident enough to create.
// Every one of them asks for a second opinion, which is the worst case the
// budget exists to bound.
func borderlineBacklog(t *testing.T, e *integration.Env, senders int) *scriptedVerdictBrain {
	t.Helper()
	brain := &scriptedVerdictBrain{
		verdicts:   map[string]string{},
		confidence: map[string]float64{},
	}
	for range senders {
		address := "borderline-" + ids.NewV7().String() + "@ambiguous.example"
		activityID := seedCapturedMail(t, e, address, "hello")
		dispositionID := seedPendingDisposition(t, e, address, "ambiguous.example", activityID)
		// A CREATING answer under verdictCreateFloor and over
		// verdictConfidenceFloor: the band the asymmetric floor re-asks, and
		// the reason a pass's cost could double.
		brain.verdicts[dispositionID.String()] = capture.KindPerson
		brain.confidence[dispositionID.String()] = 0.8
	}
	if got := len(brain.verdicts); got != senders {
		t.Fatalf("seeded %d senders, want %d — two ids collided and the backlog is smaller than the test thinks", got, senders)
	}
	return brain
}

// The whole ruling in one assertion: however borderline the backlog, one pass
// costs at most one call per sender plus its re-ask budget. Unbounded, this
// backlog would cost two calls each and run the pass past a wall sized for one.
func TestAnEntirelyBorderlinePassStaysInsideItsCallBudget(t *testing.T) {
	e := integration.Setup(t)
	const senders = 12
	brain := borderlineBacklog(t, e, senders)

	engine := NewCounterpartyVerdictEngine(e.Pool, brain, slog.Default())
	if err := engine.RunWorkspace(principal.WithWorkspaceID(context.Background(), e.WS), senders); err != nil {
		t.Fatalf("verdict pass: %v", err)
	}

	budget := reAskBudgetFor(senders)
	ceiling := senders + budget.left
	if brain.calls > ceiling {
		t.Errorf("%d model calls for %d senders, want at most %d (one each plus %d re-asks): the wall was "+
			"sized for one call per sender, and a pass that can ask twice about every one of them is a "+
			"second pass hidden inside the first",
			brain.calls, senders, ceiling, budget.left)
	}
	// And the budget was actually SPENT — otherwise a pass that stopped
	// re-asking altogether would pass the ceiling assertion above while having
	// quietly dropped the floor's second opinion.
	if brain.calls != ceiling {
		t.Errorf("%d model calls, want exactly %d: every sender is borderline, so the pass should spend its "+
			"whole re-ask budget and no more", brain.calls, ceiling)
	}
}

// A sender the budget could not pay for takes the answer the floor already
// gives: terminally unsure, for a human. Not deferred — the row WAS judged, and
// a deferral would spend one of PendingMaxAttempts on a question this pass
// never asked twice. Not accepted either: an answer this unconfident is exactly
// what the floor exists to refuse.
func TestASenderTheBudgetCouldNotPayForIsLeftForAHuman(t *testing.T) {
	e := integration.Setup(t)
	const senders = 12
	brain := borderlineBacklog(t, e, senders)

	engine := NewCounterpartyVerdictEngine(e.Pool, brain, slog.Default())
	if err := engine.RunWorkspace(principal.WithWorkspaceID(context.Background(), e.WS), senders); err != nil {
		t.Fatalf("verdict pass: %v", err)
	}

	// Every one of them is unsure: the ones that got their re-ask were still
	// borderline on it, and the ones that did not took the same answer.
	if n := countIn(t, e, `SELECT count(*) FROM capture_pending_counterparty WHERE status = $1`,
		capture.PendingStatusUnsure); n != senders {
		t.Errorf("%d of %d senders are unsure: a sender the budget could not re-ask must take the floor's "+
			"own answer rather than being dropped or accepted", n, senders)
	}
	// None created, which is what "not accepted" means for a creating answer.
	if n := countIn(t, e, `SELECT count(*) FROM person p JOIN person_email pe ON pe.person_id = p.id
		 WHERE pe.email LIKE 'borderline-%@ambiguous.example'`); n != 0 {
		t.Errorf("%d people created from answers below the creating floor", n)
	}
	// And none left pending, which is what "not dropped" means: a row still
	// claimable would be re-judged next pass at the cost of another attempt.
	if n := countIn(t, e, `SELECT count(*) FROM capture_pending_counterparty WHERE status = 'pending'`); n != 0 {
		t.Errorf("%d senders were left claimable rather than answered", n)
	}
}
