// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package ai

// A save probes Google before it takes the routing lock, because a network
// call must not hold it. What it stores is still only what it probed: a
// routing write that lands while Google is being asked is asked about too.

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/margince/margince/backend/internal/platform/settings"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func routingWriter(ws context.Context) context.Context {
	return principal.WithActor(ws, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:test",
		Permissions: principal.Permissions{
			RoleKeys: []string{"fixture"},
			Objects:  map[string]principal.ObjectGrant{routingSettingsObject: {Read: true, Update: true}},
		},
	})
}

// notServedAt serves every model everywhere except one model at one location.
func notServedAt(t *testing.T, location, modelID string, onFirstAsk func(path string)) http.HandlerFunc {
	// Not a sync.Once: the write onFirstAsk makes asks Google too, and those
	// asks must not wait on the first one finishing.
	var asked atomic.Bool
	everything := servesEverything(t)
	return func(w http.ResponseWriter, r *http.Request) {
		if asked.CompareAndSwap(false, true) {
			onFirstAsk(r.URL.Path)
		}
		if strings.Contains(r.URL.Path, "/locations/"+location+"/") && strings.Contains(r.URL.Path, "/models/"+modelID+":") {
			w.WriteHeader(http.StatusNotFound)
			writeBody(t, w, `{"error":{"code":404,"status":"NOT_FOUND","message":"Publisher Model was not found or your project does not have access to it."}}`)
			return
		}
		everything(w, r)
	}
}

func TestALocationMoveIsProbedAgainstABindingWrittenWhileGoogleWasAsked(t *testing.T) {
	e := setupRateStore(t)
	_, wsCtx := e.seedWorkspace(context.Background(), t)
	ctx := routingWriter(wsCtx)
	store := &RoutingStore{settings: settings.New(e.pool, settings.NewRegistry(Routing)), keys: allCloudKeys(t)}
	if err := settings.Set(ctx, store.settings, Routing, vertexRouting("europe-west4").canonical()); err != nil {
		t.Fatalf("seeding the binding: %v", err)
	}
	var concurrent error
	selector, _ := googleAt(t, notServedAt(t, "europe-west1", "gemini-2.5-pro", func(string) {
		// Another administrator binds a model europe-west1 does not serve,
		// while this save is still waiting on Google.
		current, err := store.Get(ctx)
		if err != nil {
			concurrent = err
			return
		}
		current.Tiers[TierFrontier] = ProviderConfig{Provider: providerGeminiVertex, Model: "gemini-2.5-pro"}
		_, concurrent = store.Replace(ctx, current)
	}))
	store.selectBrain = selector

	_, err := store.SetProviderSettings(ctx, providerGeminiVertex, ProviderSettings{Location: "europe-west1"})

	if concurrent != nil {
		t.Fatalf("the concurrent write failed: %v", concurrent)
	}
	var invalid settings.InvalidValue
	if !errors.As(err, &invalid) || !strings.Contains(invalid.Reason, "gemini-2.5-pro") {
		t.Errorf("err = %v, want a refusal naming gemini-2.5-pro, the model the new location does not serve", err)
	}
	stored, err := store.Get(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got := stored.Providers[providerGeminiVertex].Location; got != "europe-west4" {
		t.Errorf("stored location = %q, want europe-west4 kept", got)
	}
}

// A document another write moves during every probe round is never stored
// unprobed: the save gives up as stale and leaves the stored location alone.
func TestALocationMoveOutpacedOnEveryRoundIsRefusedAsStale(t *testing.T) {
	e := setupRateStore(t)
	_, wsCtx := e.seedWorkspace(context.Background(), t)
	ctx := routingWriter(wsCtx)
	store := &RoutingStore{settings: settings.New(e.pool, settings.NewRegistry(Routing)), keys: allCloudKeys(t)}
	if err := settings.Set(ctx, store.settings, Routing, vertexRouting("europe-west4").canonical()); err != nil {
		t.Fatalf("seeding the binding: %v", err)
	}
	everything := servesEverything(t)
	var rounds atomic.Int32
	store.selectBrain, _ = googleAt(t, func(w http.ResponseWriter, r *http.Request) {
		// Only the save's own asks, at the new location, move the document;
		// the competing write's asks at the old one do not, or it would recurse.
		if strings.Contains(r.URL.Path, "/locations/europe-west1/") {
			current, err := store.Get(ctx)
			if err == nil {
				models := []string{"gemini-2.5-pro", "gemini-2.5-flash"}
				current.Tiers[TierFrontier] = ProviderConfig{Provider: providerGeminiVertex, Model: models[rounds.Add(1)%2]}
				_, err = store.Replace(ctx, current)
			}
			if err != nil {
				t.Errorf("the competing write failed: %v", err)
			}
		}
		everything(w, r)
	})

	_, err := store.SetProviderSettings(ctx, providerGeminiVertex, ProviderSettings{Location: "europe-west1"})

	if !errors.Is(err, apperrors.ErrVersionSkew) {
		t.Errorf("err = %v, want ErrVersionSkew after every round went stale", err)
	}
	stored, err := store.Get(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got := stored.Providers[providerGeminiVertex].Location; got != "europe-west4" {
		t.Errorf("stored location = %q, want europe-west4 kept", got)
	}
}
