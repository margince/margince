// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A provider outage is a deferral exactly like a budget stop: no model read the
// message, so the row keeps its attempt, waits for the provider's own probe
// time, and the pass stops. A plain transport error reads the same words but is
// not a deferral, and still charges.

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// outageFor is a blocked provider whose next probe is an hour away, far from
// both the lanes' fixed backoff and any budget window.
func outageFor() *ai.ProviderDownError {
	return &ai.ProviderDownError{
		Provider: "acme", Health: model.HealthOutOfCredit, RetryAfter: time.Now().Add(time.Hour),
	}
}

func TestAProviderOutageRefundsTheSendersAndSchedulesAtTheProbe(t *testing.T) {
	e := integration.Setup(t)
	const senders = 4
	down := outageFor()
	dispositions := make([]ids.UUID, 0, senders)
	for range senders {
		address := "down-" + ids.NewV7().String() + "@outage.example"
		activityID := seedCapturedMail(t, e, address, "hello")
		dispositions = append(dispositions, seedPendingDisposition(t, e, address, "outage.example", activityID))
	}

	brain := &brokeBrain{err: down}
	engine := NewCounterpartyVerdictEngine(e.Pool, brain, CaptureConfig{}, slog.Default())
	if err := engine.RunWorkspace(principal.WithWorkspaceID(context.Background(), e.WS), senders); err != nil {
		t.Fatalf("an outage failed the pass: %v", err)
	}
	if brain.calls != 1 {
		t.Fatalf("the brain was asked %d times; an outage must stop the pass at the first refusal", brain.calls)
	}
	for i, id := range dispositions {
		if attempts, claimed := dispositionAttemptsAndClaim(t, e, id); attempts != 0 || claimed {
			t.Errorf("sender %d: attempts=%d claimed=%v, want a refunded, released row", i, attempts, claimed)
		}
		assertScheduledNear(t, e, "capture_pending_counterparty", id, down.RetryAfter)
	}
}

func TestAPlainProviderErrorStillChargesTheSender(t *testing.T) {
	e := integration.Setup(t)
	address := "plain-" + ids.NewV7().String() + "@outage.example"
	activityID := seedCapturedMail(t, e, address, "hello")
	id := seedPendingDisposition(t, e, address, "outage.example", activityID)

	brain := &brokeBrain{err: errors.New("provider said: internal error")}
	engine := NewCounterpartyVerdictEngine(e.Pool, brain, CaptureConfig{}, slog.Default())
	if err := engine.RunWorkspace(principal.WithWorkspaceID(context.Background(), e.WS), 1); err != nil {
		t.Fatalf("a message-level failure failed the pass: %v", err)
	}
	if attempts, _ := dispositionAttemptsAndClaim(t, e, id); attempts != 1 {
		t.Errorf("attempts = %d, want 1: a failure that is not a deferral is the message's to pay for", attempts)
	}
}

func TestAProviderOutageRefundsTheThreadsAndSchedulesAtTheProbe(t *testing.T) {
	e := integration.Setup(t)
	down := outageFor()
	questions := make([]ids.UUID, 0, confidentialityClaimSize)
	for range confidentialityClaimSize {
		key := "thread-down-" + ids.NewV7().String()
		activityID := seedHeldThreadMail(t, e, key, "einkauf@kunde.example", "Anfrage")
		questions = append(questions, seedThreadQuestion(t, e, key, activityID))
	}

	brain := &brokeBrain{err: down}
	engine := NewConfidentialityVerdictEngine(e.Pool, brain, slog.Default())
	if err := engine.RunWorkspace(principal.WithWorkspaceID(context.Background(), e.WS), 0); err != nil {
		t.Fatalf("an outage failed the pass: %v", err)
	}
	if brain.calls != 1 {
		t.Fatalf("the brain was asked %d times, want 1", brain.calls)
	}
	for i, id := range questions {
		if attempts, claimed := threadAttemptsAndClaim(t, e, id); attempts != 0 || claimed {
			t.Errorf("thread %d: attempts=%d claimed=%v, want a refunded, released row", i, attempts, claimed)
		}
		assertScheduledNear(t, e, "capture_thread_verdict", id, down.RetryAfter)
	}
}

func TestAPlainProviderErrorStillChargesTheThread(t *testing.T) {
	e := integration.Setup(t)
	key := "thread-plain-" + ids.NewV7().String()
	activityID := seedHeldThreadMail(t, e, key, "einkauf@kunde.example", "Anfrage")
	id := seedThreadQuestion(t, e, key, activityID)

	brain := &brokeBrain{err: errors.New("provider said: internal error")}
	engine := NewConfidentialityVerdictEngine(e.Pool, brain, slog.Default())
	if err := engine.RunWorkspace(principal.WithWorkspaceID(context.Background(), e.WS), 1); err != nil {
		t.Fatalf("a message-level failure failed the pass: %v", err)
	}
	if attempts, _ := threadAttemptsAndClaim(t, e, id); attempts != 1 {
		t.Errorf("attempts = %d, want 1", attempts)
	}
}

// assertScheduledNear holds that the row waits for the provider's probe time
// rather than the lane's fixed backoff or a budget boundary. The tolerance
// covers the gap between the test's clock and the database's.
func assertScheduledNear(t *testing.T, e *integration.Env, table string, id ids.UUID, want time.Time) {
	t.Helper()
	var next time.Time
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		// The table is a compile-time literal at each call site.
		return tx.QueryRow(context.Background(),
			`SELECT next_attempt_at FROM `+table+` WHERE id = $1`, id).Scan(&next)
	}); err != nil {
		t.Fatalf("reading next_attempt_at of %s %s: %v", table, id, err)
	}
	if delta := next.Sub(want).Abs(); delta > time.Minute {
		t.Errorf("next_attempt_at = %s, want within a minute of the probe %s", next, want)
	}
}
