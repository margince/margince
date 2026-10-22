// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The pass cap is a setting an admin changes while the product runs, and the
// backlog gauge reads the ledger the pass works. These tests drive the real
// engine and store. A cap that is only declared, or a gauge that counts a
// different set than the pass claims, fails here.

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/keyvault"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// answerEveryThreadBrain opens whatever single thread the prompt fences, and
// counts the calls, so a test can say how many threads a pass judged.
type answerEveryThreadBrain struct{ calls atomic.Int64 }

func (b *answerEveryThreadBrain) Complete(_ context.Context, req model.Request) (model.Response, error) {
	askedFor := fencedIDs(req.System, req.Messages[0].Content, "id")
	if len(askedFor) != 1 {
		return model.Response{}, fmt.Errorf("the prompt fenced %d threads, want one", len(askedFor))
	}
	b.calls.Add(1)
	payload, err := json.Marshal(map[string]any{"results": []map[string]any{
		{"id": askedFor[0], "verdict": confidentialityOrdinary, "confidence": 0.95},
	}})
	if err != nil {
		return model.Response{}, err
	}
	return model.Response{Text: string(payload)}, nil
}

// seedHeldThreads opens n threads for the classified seat.
func seedHeldThreads(t *testing.T, e *integration.Env, n int) {
	t.Helper()
	for i := range n {
		key := fmt.Sprintf("thread-cap-%d", i)
		seedThreadQuestion(t, e, key, seedHeldThreadMail(t, e, key, "kunde@example.test", "Anfrage"))
	}
	dueByTheDatabaseClock(t, e)
}

// dueByTheDatabaseClock makes every seeded thread due by the database's clock.
// seedThreadQuestion stamps the due time from this process's clock and the claim
// compares it with the database's, so a database clock a minute behind this
// host's would make every thread not yet due, and a pass would judge none.
func dueByTheDatabaseClock(t *testing.T, e *integration.Env) {
	t.Helper()
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(),
			`UPDATE capture_thread_verdict SET next_attempt_at = now() - interval '1 minute'`)
		return err
	}); err != nil {
		t.Fatalf("making the seeded threads due by the database clock: %v", err)
	}
}

// setPassCap saves the cap the way an admin does, through the settings store.
func setPassCap(t *testing.T, e *integration.Env, passCap int) {
	t.Helper()
	store := capture.NewSettings(NewSettingsStore(e.Pool), StoredConfig(e.Pool))
	if _, err := store.Update(e.Admin(), capture.SettingsPatch{ConfidentialityPassCap: &passCap}); err != nil {
		t.Fatalf("saving a pass cap of %d: %v", passCap, err)
	}
}

// runPassOn runs one pass on engine under the cap in force, the way the worker
// does.
func runPassOn(t *testing.T, e *integration.Env, engine *ConfidentialityVerdictEngine) {
	t.Helper()
	if err := engine.RunWorkspace(principal.WithWorkspaceID(context.Background(), e.WS), 0); err != nil {
		t.Fatalf("confidentiality pass: %v", err)
	}
}

func runPass(t *testing.T, e *integration.Env, brain *answerEveryThreadBrain) {
	t.Helper()
	runPassOn(t, e, NewConfidentialityVerdictEngine(e.Pool, brain, slog.Default()))
}

// The cap is a hard bound, also when it is not a multiple of the claim size.
// Each pass reads the value saved before it started, not the one in force when
// the engine was built.
func TestThePassJudgesNoMoreThanTheSavedCapAndReadsItEveryRun(t *testing.T) {
	e := integration.Setup(t)
	seedHeldThreads(t, e, 12)
	brain := &answerEveryThreadBrain{}
	engine := NewConfidentialityVerdictEngine(e.Pool, brain, slog.Default())

	setPassCap(t, e, 3)
	runPassOn(t, e, engine)
	if got := brain.calls.Load(); got != 3 {
		t.Fatalf("a pass under a cap of 3 judged %d threads, want 3", got)
	}

	// The same engine, a larger cap saved between two passes.
	setPassCap(t, e, 5)
	runPassOn(t, e, engine)
	if got := brain.calls.Load(); got != 8 {
		t.Fatalf("after raising the cap to 5 the second pass brought the total to %d, want 8", got)
	}
}

// An installation nobody tuned runs the declared default.
func TestAnUntunedInstallationRunsTheDeclaredPassCap(t *testing.T) {
	e := integration.Setup(t)
	store := capture.NewThreadVerdictStore(InstallationDB(e.Pool))

	got, err := store.PassCap(principal.WithWorkspaceID(context.Background(), e.WS))
	if err != nil {
		t.Fatalf("reading the pass cap: %v", err)
	}
	if got != capture.DefaultConfidentialityPassCap {
		t.Fatalf("an untuned installation reads a pass cap of %d, want %d", got, capture.DefaultConfidentialityPassCap)
	}
}

// The gauge counts the threads the pass can still claim, and ages the oldest
// from the capture of the message that opened it.
func TestTheBacklogCountsWhatThePassStillOwesAndAgesTheOldest(t *testing.T) {
	e := integration.Setup(t)
	db := InstallationDB(e.Pool)
	ctx := principal.WithWorkspaceID(context.Background(), e.WS)

	empty, err := capture.ReadThreadBacklog(ctx, db)
	if err != nil || empty.Waiting != 0 || empty.OldestWait != 0 {
		t.Fatalf("an empty ledger reads %+v (err %v), want a measured zero", empty, err)
	}

	oldest := seedHeldThreadMail(t, e, "thread-backlog-old", "kunde@example.test", "Alt")
	seedThreadQuestion(t, e, "thread-backlog-old", oldest)
	seedThreadQuestion(t, e, "thread-backlog-new", seedHeldThreadMail(t, e, "thread-backlog-new", "kunde@example.test", "Neu"))
	dueByTheDatabaseClock(t, e)
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(),
			`UPDATE activity SET created_at = now() - interval '30 minutes' WHERE id = $1`, oldest)
		return err
	}); err != nil {
		t.Fatalf("backdating the oldest message: %v", err)
	}

	backlog, err := capture.ReadThreadBacklog(ctx, db)
	if err != nil {
		t.Fatalf("reading the backlog: %v", err)
	}
	if backlog.Waiting != 2 {
		t.Fatalf("the backlog counts %d threads, want 2", backlog.Waiting)
	}
	if backlog.OldestWait < 29*time.Minute || backlog.OldestWait > 31*time.Minute {
		t.Fatalf("the oldest thread has waited %v, want about 30 minutes", backlog.OldestWait)
	}

	// The pass answers them, and the gauge reads that as caught up.
	runPass(t, e, &answerEveryThreadBrain{})
	caughtUp, err := capture.ReadThreadBacklog(ctx, db)
	if err != nil || caughtUp.Waiting != 0 || caughtUp.OldestWait != 0 {
		t.Fatalf("after the pass the backlog reads %+v (err %v), want a measured zero", caughtUp, err)
	}
}

// A thread the pass will not claim is not a thread it owes. A seat that left
// the classified posture is judged by nobody, and counting it would alarm on
// mail no capacity could clear.
func TestTheBacklogLeavesOutThreadsThePassWillNotClaim(t *testing.T) {
	e := integration.Setup(t)
	db := InstallationDB(e.Pool)
	ctx := principal.WithWorkspaceID(context.Background(), e.WS)
	seedThreadQuestion(t, e, "thread-unclaimable", seedHeldThreadMail(t, e, "thread-unclaimable", "kunde@example.test", "Anfrage"))
	// The seat leaves the classified posture through the registry's own writer.
	registry := capture.NewRegistry(e.DB(), capture.NewSink(e.DB()), backfillAuthority{}, keyvault.NewMemory())
	registry.Register(plainSyncConnector{name: "gmail"})
	seat := e.As(e.Rep1, nil, integration.AdminPerms)
	if _, err := registry.SetMailPosture(seat, "gmail", capture.PostureHeld, false); err != nil {
		t.Fatalf("switching the seat's posture: %v", err)
	}

	backlog, err := capture.ReadThreadBacklog(ctx, db)
	if err != nil {
		t.Fatalf("reading the backlog: %v", err)
	}
	claimed, err := capture.NewThreadVerdictStore(db).ClaimDue(ctx, 10)
	if err != nil {
		t.Fatalf("claiming: %v", err)
	}
	if backlog.Waiting != 0 || len(claimed) != 0 {
		t.Fatalf("the backlog counts %d and the pass claims %d for a seat that is no longer classified, want 0 and 0",
			backlog.Waiting, len(claimed))
	}
}
