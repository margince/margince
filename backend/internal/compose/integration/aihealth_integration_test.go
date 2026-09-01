// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// Whether the model lanes are answering, read through the wire.
//
// The failure this surface exists for: under the capture posture a thread stays
// held whether the classifier judged it confidential or never answered at all,
// so an outage and correct cautious behaviour look identical everywhere else.

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestAiHealthReportsARungThatStoppedAnswering(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	seedRungCall(t, e, "local_small", "", 120, time.Now().Add(-5*time.Minute))
	seedRungCall(t, e, "cloud_large", "provider_unavailable", 30, time.Now().Add(-3*time.Minute))

	var got crmcontracts.AiHealth
	if status := e.Call(t, http.MethodGet, "/v1/ai/health", nil, nil, &got); status != http.StatusOK {
		t.Fatalf("GET /v1/ai/health = %d, want 200", status)
	}
	rungs := map[string]crmcontracts.AiRungHealth{}
	for _, r := range got.Rungs {
		rungs[r.Tier] = r
	}
	if !rungs["local_small"].Healthy {
		t.Error("a rung that answered is reported unhealthy")
	}
	dead := rungs["cloud_large"]
	if dead.Healthy {
		t.Error("a rung whose every attempt failed is reported healthy — " +
			"which is the state this endpoint exists to make visible")
	}
	// The sentinel is the operator's first clue: a budget refusal and an
	// unreachable model are both "not answering" and want different fixes.
	if dead.LastSentinel == nil || *dead.LastSentinel != "provider_unavailable" {
		t.Errorf("last_sentinel = %v, want provider_unavailable", dead.LastSentinel)
	}
	if got.WindowHours != 1 {
		t.Errorf("window_hours = %d, want 1", got.WindowHours)
	}
}

// A failure a retry rescued is not a lane that failed. Counting non-terminal
// attempts would report a rung as failing while every caller of it got an
// answer, which is a false alarm an operator learns to ignore.
func TestAiHealthIgnoresAnAttemptARetryRescued(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	logical := ids.NewV7()
	seedAttempt(t, e, "local_small", "provider_unavailable", 40,
		time.Now().Add(-4*time.Minute), logical, 1, false)
	seedAttempt(t, e, "local_small", "", 200,
		time.Now().Add(-3*time.Minute), logical, 2, true)

	var got crmcontracts.AiHealth
	if status := e.Call(t, http.MethodGet, "/v1/ai/health", nil, nil, &got); status != http.StatusOK {
		t.Fatalf("GET /v1/ai/health = %d, want 200", status)
	}
	if len(got.Rungs) != 1 {
		t.Fatalf("%d rungs, want 1", len(got.Rungs))
	}
	if got.Rungs[0].Failures != 0 {
		t.Errorf("failures = %d, want 0 — the retry answered", got.Rungs[0].Failures)
	}
	if !got.Rungs[0].Healthy {
		t.Error("a rung whose retry succeeded is reported unhealthy")
	}
}

// An hour, because the question is whether it is answering NOW. A lane that
// died forty minutes ago must not read as healthy on the strength of a call it
// made this morning.
func TestAiHealthReadsOnlyTheLastHour(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	seedRungCall(t, e, "stale_rung", "", 100, time.Now().Add(-3*time.Hour))

	var got crmcontracts.AiHealth
	if status := e.Call(t, http.MethodGet, "/v1/ai/health", nil, nil, &got); status != http.StatusOK {
		t.Fatalf("GET /v1/ai/health = %d, want 200", status)
	}
	for _, r := range got.Rungs {
		if r.Tier == "stale_rung" {
			t.Error("a call from three hours ago is inside a one-hour window")
		}
	}
}

// Operational configuration, not anybody's own data: the same door /ai/usage
// uses, and a seat without it is refused rather than shown an empty page.
func TestAiHealthIsRefusedWithoutTheAutomationGrant(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	seedRungCall(t, e, "local_small", "", 100, time.Now().Add(-time.Minute))
	demoteToRep(t, e)

	if status := e.Call(t, http.MethodGet, "/v1/ai/health", nil, nil, nil); status != http.StatusForbidden {
		t.Errorf("GET /v1/ai/health as a rep = %d, want 403", status)
	}
}

func seedRungCall(t *testing.T, e *apptest.AppEnv, tier, sentinel string, latency int, at time.Time) {
	t.Helper()
	seedAttempt(t, e, tier, sentinel, latency, at, ids.NewV7(), 1, true)
}

func seedAttempt(t *testing.T, e *apptest.AppEnv, tier, sentinel string,
	latency int, at time.Time, logical ids.UUID, attempt int, terminal bool,
) {
	t.Helper()
	var errSentinel any
	if sentinel != "" {
		errSentinel = sentinel
	}
	if _, err := e.Owner.Exec(context.Background(), `
		INSERT INTO ai_call (id, task, tier, provider, model_id, request_fingerprint,
		                     tokens_in, tokens_out, latency_ms, error_sentinel,
		                     logical_call_id, attempt, is_terminal, occurred_at)
		VALUES ($1, 'capture_confidentiality_verdict', $2, 'test', 'test-model', 'fp',
		        10, 5, $3, $4, $5, $6, $7, $8)`,
		ids.NewV7(), tier, latency, errSentinel, logical, attempt, terminal, at); err != nil {
		t.Fatalf("seeding an ai_call: %v", err)
	}
}
