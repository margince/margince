// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The verdict, label and owed-reply runs read their per-run cap from the stored
// settings an admin saves. Each test saves a cap through the settings store and
// runs the real engine the way its worker does, with no explicit limit. A cap
// that is only declared, or an engine that still holds a compiled constant,
// judges the whole backlog here and fails.

import (
	"context"
	"fmt"
	"log/slog"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// savePassCaps saves caps the way an admin does, through the settings store.
func savePassCaps(t *testing.T, e *integration.Env, patch capture.SettingsPatch) {
	t.Helper()
	store := capture.NewSettings(NewSettingsStore(e.Pool), StoredConfig(e.Pool))
	if _, err := store.Update(e.Admin(), patch); err != nil {
		t.Fatalf("saving the pass cap: %v", err)
	}
}

func TestTheVerdictRunJudgesNoMoreSendersThanTheSavedCap(t *testing.T) {
	e := integration.Setup(t)
	const passCap = 3
	for range 12 {
		address := "capped-" + ids.NewV7().String() + "@failing.example"
		seedPendingDisposition(t, e, address, "failing.example", seedCapturedMail(t, e, address, "hello"))
	}
	verdictCap := passCap
	savePassCaps(t, e, capture.SettingsPatch{VerdictPassCap: &verdictCap})
	brain := &unreadableVerdictBrain{}

	engine := NewCounterpartyVerdictEngine(e.Pool, brain, CaptureConfig{}, slog.Default())
	if err := engine.RunWorkspace(principal.WithWorkspaceID(context.Background(), e.WS), 0); err != nil {
		t.Fatalf("verdict run: %v", err)
	}

	if got := brain.calls.Load(); got != passCap {
		t.Fatalf("a run under a saved cap of %d asked the model about %d senders, want %d", passCap, got, passCap)
	}
}

func TestTheLabelRunSettlesNoMoreMessagesThanTheSavedCap(t *testing.T) {
	e := integration.Setup(t)
	for i := range 3 * classifyBatchSize {
		seedUnlabeledEmail(t, e, fmt.Sprintf("Anfrage %d", i))
	}
	classifyCap := classifyBatchSize
	savePassCaps(t, e, capture.SettingsPatch{ClassifyPassCap: &classifyCap})
	brain := &scriptedClassifyBrain{}

	if err := NewCaptureClassifier(e.Pool, brain, slog.New(slog.DiscardHandler)).RunWorkspace(asClassifyPass(e), 0); err != nil {
		t.Fatalf("label run: %v", err)
	}

	if brain.calls != 1 {
		t.Fatalf("a run under a saved cap of one batch made %d model calls, want 1", brain.calls)
	}
}

func TestTheOwedReplyRunJudgesNoMoreNewMailThanTheSavedCap(t *testing.T) {
	e := integration.Setup(t)
	for i := range 3 * owedBatchSize {
		seedWaitingMail(t, e, fmt.Sprintf("Thread %d", i))
	}
	owedCap := owedBatchSize
	savePassCaps(t, e, capture.SettingsPatch{OwedClassifyPassCap: &owedCap})
	brain := &owedBrainStub{verdict: activities.OwedVerdictInformsUs, confidence: 0.95}

	runOwedWorker(t, e, brain)

	if len(brain.prompts) != 1 {
		t.Fatalf("a run under a saved cap of one batch made %d model calls, want 1", len(brain.prompts))
	}
}
