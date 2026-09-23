// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// What a budget stop leaves behind, in both engines that lease a backlog.
//
// The stop lands MID-BATCH in both tests below, which is the only arrangement
// that says anything: a pass stopped on its first row hands back the batch it
// was given, and a release that ignored where the pass got to would look
// identical. What was decided before the stop stays decided; every row still
// claimed is owed one refund of the attempt the claim charged, one explanation,
// and a return date at the window the router named.

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// budgetStopReason is what every row caught by one budget stop says, whichever
// engine caught it. One event, one explanation.
const budgetStopReason = "the workspace was out of model budget"

func TestAThreadBudgetStopRefundsEveryClaimedRowOnceAtTheBudgetWindow(t *testing.T) {
	const schedule = `SELECT status, attempts, coalesce(disposition_reason, ''), next_attempt_at
	                    FROM capture_thread_verdict WHERE id = $1`

	e := integration.Setup(t)
	window := time.Now().Add(72 * time.Hour)
	threads := map[string]ids.UUID{}
	for _, key := range []string{"thread-budget-a", "thread-budget-b", "thread-budget-c"} {
		activityID := seedHeldThreadMail(t, e, key, "einkauf@kunde.example", "Nachbestellung")
		threads[key] = seedThreadQuestion(t, e, key, activityID)
	}
	spendOneThreadAttempt(t, e)

	brain := &threadBudgetBrain{window: window, answers: 1}
	engine := NewConfidentialityVerdictEngine(e.Pool, brain, slog.Default())
	if err := engine.RunWorkspace(principal.WithWorkspaceID(context.Background(), e.WS), 0); err != nil {
		t.Fatalf("a budget stop ends the pass cleanly: %v", err)
	}

	assertOneAnsweredAndTheRestReleased(t, e, schedule, threads, window)
}

func TestASenderBudgetStopRefundsEveryClaimedRowOnceAtTheBudgetWindow(t *testing.T) {
	const schedule = `SELECT status, attempts, coalesce(disposition_reason, ''), next_attempt_at
	                    FROM capture_pending_counterparty WHERE id = $1`

	e := integration.Setup(t)
	window := time.Now().Add(72 * time.Hour)
	senders := map[string]ids.UUID{}
	for range 3 {
		address := "budget-" + ids.NewV7().String() + "@ambiguous.example"
		activityID := seedCapturedMail(t, e, address, "hello")
		senders[address] = seedPendingDisposition(t, e, address, "ambiguous.example", activityID)
	}
	spendOneSenderAttempt(t, e)

	brain := &senderBudgetBrain{window: window, answers: 1}
	engine := NewCounterpartyVerdictEngine(e.Pool, brain, CaptureConfig{}, slog.Default())
	if err := engine.RunWorkspace(principal.WithWorkspaceID(context.Background(), e.WS), 0); err != nil {
		t.Fatalf("a budget stop ends the pass cleanly: %v", err)
	}

	assertOneAnsweredAndTheRestReleased(t, e, schedule, senders, window)
}

// assertOneAnsweredAndTheRestReleased is the whole ruling, read off the backlog
// a stopped pass left.
//
// The reason and the return date are what separate one writer from two: a
// second writer over the stopped row overwrites what the first said, and a row
// spaced by the engine's own failure backoff is not waiting for the window. The
// attempt count is the standing guard behind them — both stores floor the
// counter at zero, so a second refund of one claim shows only on a row that had
// already spent an attempt, and only once somebody relaxes the claim check that
// swallows it today. A row left OUT of the release shows here too, as the claim
// charge it never got back.
func assertOneAnsweredAndTheRestReleased(
	t *testing.T, e *integration.Env, query string, rows map[string]ids.UUID, window time.Time,
) {
	t.Helper()
	answered := 0
	for name, id := range rows {
		status, attempts, reason, nextAttemptAt := deferredSchedule(t, e, query, id)
		if status != "pending" {
			answered++
			continue
		}
		if attempts != 1 {
			t.Errorf("%s has spent %d attempts, want 1: no answer came back for it, so the claim's "+
				"charge comes back exactly once — twice would hand it retries past its own bound",
				name, attempts)
		}
		if reason != budgetStopReason {
			t.Errorf("%s says %q, want %q: one budget stop is one event, and the row it stopped on is "+
				"no better informed than the rows behind it", name, reason, budgetStopReason)
		}
		if nextAttemptAt == nil {
			t.Errorf("%s is still pending with no next attempt, so nothing will ever claim it", name)
			continue
		}
		// A minute absorbs the skew between this process's clock and the
		// database's, which is what the conversion from a deadline to an
		// interval leaves behind.
		if drift := nextAttemptAt.Sub(window); drift > time.Minute || drift < -time.Minute {
			t.Errorf("%s comes back at %s rather than the window the router named, %s: the engine's own "+
				"failure spacing would re-claim and re-release it every cycle until the month turned",
				name, nextAttemptAt, window)
		}
	}
	if answered != 1 {
		t.Fatalf("%d of %d rows were answered, want 1: the stop has to land mid-batch, or the release "+
			"is never asked where the pass got to", answered, len(rows))
	}
}

// spendOneThreadAttempt puts every due thread one attempt into its allowance,
// through the claim and the deferral production spends it with.
func spendOneThreadAttempt(t *testing.T, e *integration.Env) {
	t.Helper()
	store := capture.NewThreadVerdictStore(InstallationDB(e.Pool))
	claimed, err := store.ClaimDue(e.Admin(), 10)
	if err != nil {
		t.Fatalf("claiming the threads a first pass would have tried: %v", err)
	}
	for _, row := range claimed {
		if err := store.Defer(e.Admin(), row, 0, "a first pass could not answer", false); err != nil {
			t.Fatalf("charging a thread its first attempt: %v", err)
		}
	}
}

// spendOneSenderAttempt is spendOneThreadAttempt over the disposition ledger.
func spendOneSenderAttempt(t *testing.T, e *integration.Env) {
	t.Helper()
	store := capture.NewPendingStore(InstallationDB(e.Pool))
	claimed, err := store.ClaimDue(e.Admin(), 10)
	if err != nil {
		t.Fatalf("claiming the senders a first pass would have tried: %v", err)
	}
	for _, row := range claimed {
		if err := store.Defer(e.Admin(), row, 0, "a first pass could not answer", false); err != nil {
			t.Fatalf("charging a sender its first attempt: %v", err)
		}
	}
}

// deferredSchedule reads where a row stands, when it comes back and what it was
// told. The query is the caller's compile-time literal because the two engines
// keep their backlogs in two tables.
//
// A row that reached an answer carries no next attempt at all, so the moment is
// nullable and only a released row is asked for it.
func deferredSchedule(
	t *testing.T, e *integration.Env, query string, id ids.UUID,
) (string, int, string, *time.Time) {
	t.Helper()
	var status, reason string
	var attempts int
	var nextAttemptAt *time.Time
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), query, id).
			Scan(&status, &attempts, &reason, &nextAttemptAt)
	}); err != nil {
		t.Fatalf("reading the row's schedule: %v", err)
	}
	return status, attempts, reason, nextAttemptAt
}

// threadBudgetBrain answers a few threads and then reports the workspace out of
// budget, the way the router does: before a provider is reached, naming the
// window the task may be asked again in.
type threadBudgetBrain struct {
	window  time.Time
	answers int
}

func (b *threadBudgetBrain) Complete(_ context.Context, req model.Request) (model.Response, error) {
	if b.answers <= 0 {
		return model.Response{}, &ai.BudgetDeferralError{
			Task:          ai.TaskCaptureConfidentialityVerdict,
			NextAttemptAt: b.window,
		}
	}
	b.answers--
	askedFor := fencedIDs(req.System, req.Messages[0].Content, "id")
	if len(askedFor) != 1 {
		return model.Response{}, fmt.Errorf(
			"confidentiality prompt fenced %d threads rather than one", len(askedFor))
	}
	payload, err := json.Marshal(map[string]any{"results": []map[string]any{
		{"id": askedFor[0], "verdict": confidentialityOrdinary, "confidence": 0.95},
	}})
	if err != nil {
		return model.Response{}, err
	}
	return model.Response{Text: string(payload)}, nil
}

// senderBudgetBrain is threadBudgetBrain over the disposition ledger, answering
// through the scripted brain the rest of this package's verdict tests use so
// the answers it gives are the answers they give.
type senderBudgetBrain struct {
	window   time.Time
	answers  int
	scripted scriptedVerdictBrain
}

func (b *senderBudgetBrain) Complete(ctx context.Context, req model.Request) (model.Response, error) {
	if b.answers <= 0 {
		return model.Response{}, &ai.BudgetDeferralError{
			Task:          ai.TaskCaptureCounterpartyVerdict,
			NextAttemptAt: b.window,
		}
	}
	b.answers--
	return b.scripted.Complete(ctx, req)
}
