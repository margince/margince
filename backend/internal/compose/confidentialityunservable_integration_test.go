// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The confidentiality half of the same state: a model is composed and the
// router will not send this task to it.
//
// This lane matters more than its sibling, because what it decides is whether a
// classified mailbox opens a thread at all. Failing the pass leaves every
// thread unjudged AND fills the sweep's log with an alarm about a binding
// somebody chose; holding leaves them private, which is where every other
// unanswered outcome here already lands.

import (
	"context"
	"log/slog"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// refusingConfidentialityBrain is the router's answer for a local-only task
// whose ladder binds no local rung.
type refusingConfidentialityBrain struct{ calls int }

func (b *refusingConfidentialityBrain) Complete(
	context.Context, model.Request,
) (model.Response, error) {
	b.calls++
	return model.Response{}, ai.ErrLocalOnlyUnservable
}

// threadAttempts reads how many tries this thread has spent.
func threadAttempts(t *testing.T, e *integration.Env, id ids.UUID) int {
	t.Helper()
	var attempts int
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT attempts FROM capture_thread_verdict WHERE id = $1`, id).Scan(&attempts)
	}); err != nil {
		t.Fatalf("reading the thread attempts: %v", err)
	}
	return attempts
}

// The thread does not PAY for a binding it has nothing to do with.
//
// This is the whole difference, and the generic path gets it wrong: an error
// out of judgeOne is charged, because "any other fault is a property of THIS
// thread, whose text an outsider writes". An unservable binding is not. Charged
// anyway, every thread burns its allowance on passes that never reached a
// model, and is then held permanently for a reason that is about the routing
// config rather than about the mail.
func TestAnUnservableConfidentialityVerdictDoesNotChargeTheThread(t *testing.T) {
	e := integration.Setup(t)
	activityID := seedHeldThreadMail(t, e, "thread-unservable-3", "buchhaltung@kunde.example", "Rechnung")
	threadID := seedThreadQuestion(t, e, "thread-unservable-3", activityID)
	before := threadAttempts(t, e, threadID)

	brain := &refusingConfidentialityBrain{}
	engine := NewConfidentialityVerdictEngine(e.Pool, brain, slog.Default())
	if err := engine.RunWorkspace(principal.WithWorkspaceID(context.Background(), e.WS), 0); err != nil {
		t.Fatalf("confidentiality pass: %v", err)
	}

	if got := threadAttempts(t, e, threadID); got > before {
		t.Errorf("attempts went %d → %d; a thread must not spend its allowance on a pass that never reached a model", before, got)
	}
}

// The pass succeeds and the thread stays private.
func TestAnUnservableConfidentialityVerdictHoldsRatherThanFailingTheSweep(t *testing.T) {
	e := integration.Setup(t)
	activityID := seedHeldThreadMail(t, e, "thread-unservable", "einkauf@kunde.example", "Nachbestellung")
	threadID := seedThreadQuestion(t, e, "thread-unservable", activityID)

	brain := &refusingConfidentialityBrain{}
	engine := NewConfidentialityVerdictEngine(e.Pool, brain, slog.Default())

	if err := engine.RunWorkspace(principal.WithWorkspaceID(context.Background(), e.WS), 0); err != nil {
		t.Fatalf("the sweep failed on a binding it cannot serve: %v", err)
	}

	// The message stays private. An opening answer is the only one that
	// publishes, and none was given — so a wrong answer, an outage and this all
	// fail the same way, towards privacy.
	if got := activityAudience(t, e, activityID); got == "workspace" {
		t.Errorf("a thread nobody judged was opened to the workspace (%q)", got)
	}
	// And it is not decided: the thread is left for a pass that can answer,
	// rather than recorded as a verdict nothing reached.
	if got := threadStatus(t, e, threadID); got == capture.VerdictCleared {
		t.Errorf("thread status = %q — an unasked question was recorded as cleared", got)
	}
}

// One refusal per pass, not one per thread and not a retry: it is a standing
// property of the binding, so asking again answers the same and the sweep would
// spend its whole budget learning that.
func TestAnUnservableConfidentialityVerdictIsNotReAsked(t *testing.T) {
	e := integration.Setup(t)
	activityID := seedHeldThreadMail(t, e, "thread-unservable-2", "vertrieb@kunde.example", "Angebot")
	seedThreadQuestion(t, e, "thread-unservable-2", activityID)

	brain := &refusingConfidentialityBrain{}
	engine := NewConfidentialityVerdictEngine(e.Pool, brain, slog.Default())

	if err := engine.RunWorkspace(principal.WithWorkspaceID(context.Background(), e.WS), 0); err != nil {
		t.Fatalf("confidentiality pass: %v", err)
	}

	if brain.calls > 1 {
		t.Errorf("asked the refusing router %d times for one thread", brain.calls)
	}
}
