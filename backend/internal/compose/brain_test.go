// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// A nil pool is safe here: FakeRoutingConfig's embeddings binding has no
// model, so seedEmbedBinding's identity check is empty and it never
// touches the store construction wraps. The router itself does no I/O
// until a call is served.
func TestNewModelPathExposesCacheInvalidation(t *testing.T) {
	path, err := NewModelPath(context.Background(), ai.FakeRoutingConfig(), nil, false, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("NewModelPath: %v", err)
	}
	if path.InvalidateCache == nil {
		t.Fatal("InvalidateCache is nil; a data reset cannot drop cached completions and the reseeded install would serve pre-reset answers")
	}
	path.InvalidateCache(ids.WorkspaceID{}) // must not panic
}

// The hook must actually reach the router's result cache, not just exist:
// a second identical request under the same workspace is served from
// cache (one model call recorded), and only a request AFTER
// InvalidateCache reaches the fake client again. Runs over
// NewLocalModelPath (no Postgres) because the router refuses to serve a
// completion outside a workspace context, and metering a real NewModelPath
// call needs the real pool the compose/integration suite already covers
// (see ai_fake_modelpath_integration_test.go).
func TestModelPathInvalidateCacheForcesAFreshCompletion(t *testing.T) {
	fake := ai.NewFakeClient()
	path, err := NewLocalModelPath(ai.FakeRoutingConfig(), ai.WithFakeClient(fake))
	if err != nil {
		t.Fatalf("NewLocalModelPath: %v", err)
	}

	workspace := ids.New[ids.WorkspaceKind]()
	ctx := principal.WithWorkspaceID(context.Background(), workspace.UUID)
	req := model.Request{
		System:   "test",
		Messages: []model.Message{{Role: "user", Content: "hello"}},
	}

	if _, err := path.ColdStart.Complete(ctx, req); err != nil {
		t.Fatalf("first Complete: %v", err)
	}
	if _, err := path.ColdStart.Complete(ctx, req); err != nil {
		t.Fatalf("second Complete: %v", err)
	}
	if calls := len(fake.Calls()); calls != 1 {
		t.Fatalf("calls after two identical requests = %d, want 1 (the second must be a cache hit)", calls)
	}

	path.InvalidateCache(workspace)

	if _, err := path.ColdStart.Complete(ctx, req); err != nil {
		t.Fatalf("third Complete: %v", err)
	}
	if calls := len(fake.Calls()); calls != 2 {
		t.Fatalf("calls after invalidation = %d, want 2 (the cache must have been dropped)", calls)
	}
}

// Both boot loops offer a fallback candidate, and both must offer it for ONE
// reason only: the binding names a provider this process cannot call. A
// database fault from the embed marker is not that, and falling back on it
// would launch a process whose marker is unestablished while telling the
// operator a credential is missing — sending them to look at the wrong thing
// while a storage outage stands.
//
// A binding whose provider needs a key it does not have is the reachable case
// here; a nil pool never gets as far as the store, so what this pins is the
// classification the loops read, not the store failure itself.
func TestAnUnservableBindingIsTheOnlyFailureAFallbackMayAnswer(t *testing.T) {
	unkeyed := ai.RoutingConfig{
		Profile: ai.ProfileEUHosted,
		Tiers: map[ai.Tier]ai.ProviderConfig{
			ai.TierPremium: {Provider: "gemini", Model: "gemini-2.5-flash"},
		},
	}

	_, err := NewModelPath(context.Background(), unkeyed, nil, false,
		slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err == nil {
		t.Fatal("a binding with no credential built a model path, so the fail-closed rule is gone")
	}
	var unservable *UnservableBindingError
	if !errors.As(err, &unservable) {
		t.Fatalf("a provider that cannot be called is not marked unservable (%v), so a boot loop "+
			"cannot tell it from a database fault and will either refuse to fall back or fall "+
			"back on both", err)
	}
	if !errors.Is(err, unservable.Cause) {
		t.Error("the wrapper hides its cause, so the operator loses the sentence naming what is wrong")
	}
}
