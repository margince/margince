// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// What a budget stop costs the rows it never reached: nothing.
//
// Both verdict engines lease a batch and then judge it one row at a time. When
// the workspace runs out of model budget partway through, the rest of that
// batch is still leased to a pass that will not ask about it — and a lease is
// not free. Left alone, those rows wait out the lease before anyone can look at
// them again, and if the release charged them an attempt, two quiet cycles
// would exhaust the allowance of a row no model ever saw.
//
// So the release is a REFUND, and these two cases hold it for each engine: the
// batch comes back claimable, and every attempt counter is where it started.
// A database is the only thing that can say so — the counter and the claim are
// columns, and the decrement is a CASE inside the UPDATE.

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// brokeBrain is the workspace with no budget left: every ask is deferred before
// it reaches a model. It stands in for the spend guard rather than for a
// provider, which is why it returns the sentinel and not a transport error —
// the two take opposite branches, and only this one refunds.
type brokeBrain struct {
	calls int
	// onAsk runs before the refusal, so a case can change the world underneath
	// the pass at the one moment it is guaranteed to be mid-batch.
	onAsk func()
}

func (b *brokeBrain) Complete(context.Context, model.Request) (model.Response, error) {
	b.calls++
	if b.onAsk != nil {
		b.onAsk()
	}
	return model.Response{}, ai.ErrBudgetDeferred
}

// A pass that stops on the first sender must leave the other seven exactly as
// it found them. The engine stops on sender one, defers it with its attempt
// refunded, and releases the remaining seven the same way.
func TestABudgetStopLeavesTheSendersItNeverAskedAboutUnspent(t *testing.T) {
	e := integration.Setup(t)
	const senders = 8

	dispositions := make([]ids.UUID, 0, senders)
	for range senders {
		address := "unasked-" + ids.NewV7().String() + "@broke.example"
		activityID := seedCapturedMail(t, e, address, "hello")
		dispositions = append(dispositions, seedPendingDisposition(t, e, address, "broke.example", activityID))
	}

	brain := &brokeBrain{}
	engine := NewCounterpartyVerdictEngine(e.Pool, brain, CaptureConfig{}, slog.Default())
	// The pass reports no error: a budget stop is an ordinary end to a pass,
	// not a fault, and the worker must not treat it as one.
	if err := engine.RunWorkspace(principal.WithWorkspaceID(context.Background(), e.WS), senders); err != nil {
		t.Fatalf("a budget stop failed the pass: %v", err)
	}
	if brain.calls != 1 {
		t.Fatalf("the brain was asked %d times; the first refusal is supposed to stop the pass, "+
			"and a pass that kept asking would not reach the release at all", brain.calls)
	}

	for i, id := range dispositions {
		attempts, claimed := dispositionAttemptsAndClaim(t, e, id)
		if claimed {
			t.Errorf("sender %d is still claimed by the pass that stopped: it waits out a lease "+
				"nobody is using before anyone can judge it", i)
		}
		if attempts != 0 {
			t.Errorf("sender %d was charged %d attempts for a question no model was asked; "+
				"two quiet cycles like this retire a genuine sender to unsure", i, attempts)
		}
	}
	if n := countIn(t, e, `SELECT count(*) FROM capture_pending_counterparty WHERE status = 'pending'`); n != senders {
		t.Errorf("%d of %d senders are still pending — a budget stop must end nothing", n, senders)
	}
}

// The thread engine's half of the same rule. Seeded past the claim size so the
// batch genuinely has a tail: with eight or fewer the release would run over an
// empty slice and the case would pass without exercising anything.
func TestABudgetStopLeavesTheThreadsItNeverAskedAboutUnspent(t *testing.T) {
	e := integration.Setup(t)
	const threads = confidentialityClaimSize

	questions := make([]ids.UUID, 0, threads)
	for range threads {
		key := "thread-broke-" + ids.NewV7().String()
		activityID := seedHeldThreadMail(t, e, key, "einkauf@kunde.example", "Anfrage")
		questions = append(questions, seedThreadQuestion(t, e, key, activityID))
	}

	brain := &brokeBrain{}
	engine := NewConfidentialityVerdictEngine(e.Pool, brain, slog.Default())
	if err := engine.RunWorkspace(principal.WithWorkspaceID(context.Background(), e.WS), 0); err != nil {
		t.Fatalf("a budget stop failed the pass: %v", err)
	}
	if brain.calls != 1 {
		t.Fatalf("the brain was asked %d times, want 1", brain.calls)
	}

	for i, id := range questions {
		attempts, claimed := threadAttemptsAndClaim(t, e, id)
		if claimed {
			t.Errorf("thread %d is still leased to the pass that stopped", i)
		}
		if attempts != 0 {
			t.Errorf("thread %d was charged %d attempts for a question no model was asked", i, attempts)
		}
	}
}

// dispositionAttemptsAndClaim reads the two columns the refund is spelled in.
func dispositionAttemptsAndClaim(t *testing.T, e *integration.Env, id ids.UUID) (int, bool) {
	t.Helper()
	return attemptsAndClaim(t, e, "capture_pending_counterparty", id)
}

func threadAttemptsAndClaim(t *testing.T, e *integration.Env, id ids.UUID) (int, bool) {
	t.Helper()
	return attemptsAndClaim(t, e, "capture_thread_verdict", id)
}

// attemptsAndClaim is shared because the two queues answer the refund question
// with the same two columns, and a second spelling of it is a second thing to
// keep in step.
func attemptsAndClaim(t *testing.T, e *integration.Env, table string, id ids.UUID) (int, bool) {
	t.Helper()
	var attempts int
	var claimed bool
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			// The table is one of two compile-time literals above, never a
			// value off a request.
			`SELECT attempts, claimed_by IS NOT NULL FROM `+table+` WHERE id = $1`,
			id).Scan(&attempts, &claimed)
	}); err != nil {
		t.Fatalf("reading the refund columns of %s %s: %v", table, id, err)
	}
	return attempts, claimed
}

// A pass whose context dies mid-batch REPORTS it, rather than ending quietly
// with rows still leased.
//
// The release is best effort by nature — a Defer that fails leaves the row to
// wait out its lease, which is the backstop that makes it an optimization
// rather than a correctness requirement. What must not happen is the pass
// treating that as an ordinary budget stop: a budget stop is a clean end and
// the worker records success, and a worker told "success" over a batch it
// could neither judge nor release has lost the information that anything went
// wrong. Cancellation is the honest driver for it — a shutdown lands here —
// and it reaches every arm the refund path has, which no reachable database
// state does.
func TestAPassWhoseContextDiesMidBatchSaysSo(t *testing.T) {
	e := integration.Setup(t)
	const senders = 4
	for range senders {
		address := "cut-short-" + ids.NewV7().String() + "@broke.example"
		activityID := seedCapturedMail(t, e, address, "hello")
		seedPendingDisposition(t, e, address, "broke.example", activityID)
	}

	ctx, stop := context.WithCancel(principal.WithWorkspaceID(context.Background(), e.WS))
	brain := &brokeBrain{onAsk: stop}
	engine := NewCounterpartyVerdictEngine(e.Pool, brain, CaptureConfig{}, slog.Default())

	err := engine.RunWorkspace(ctx, senders)
	if err == nil {
		t.Fatal("a pass that could neither judge nor release its batch reported success: the worker " +
			"records that as a clean run, and nothing anywhere says the batch is still leased")
	}
	if errors.Is(err, ai.ErrBudgetDeferred) {
		t.Errorf("the failure was reported as a budget stop (%v), which RunWorkspace swallows as an "+
			"ordinary end to a pass — the one reading under which this is invisible", err)
	}
}

// The thread engine's half. Both engines carry the same release and the same
// two arms through it, and a rule held on one of them is a rule the other can
// lose quietly — every change to this shape so far has had to be made twice.
func TestAThreadPassWhoseContextDiesMidBatchSaysSo(t *testing.T) {
	e := integration.Setup(t)
	for range confidentialityClaimSize {
		key := "thread-cut-" + ids.NewV7().String()
		activityID := seedHeldThreadMail(t, e, key, "einkauf@kunde.example", "Anfrage")
		seedThreadQuestion(t, e, key, activityID)
	}

	ctx, stop := context.WithCancel(principal.WithWorkspaceID(context.Background(), e.WS))
	brain := &brokeBrain{onAsk: stop}
	engine := NewConfidentialityVerdictEngine(e.Pool, brain, slog.Default())

	err := engine.RunWorkspace(ctx, 0)
	if err == nil {
		t.Fatal("a thread pass that could neither judge nor release its batch reported success")
	}
	if errors.Is(err, ai.ErrBudgetDeferred) {
		t.Errorf("the failure was reported as a budget stop (%v), which RunWorkspace swallows", err)
	}
}
