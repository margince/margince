// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// servesEverything answers every probe verb as served, at every location.
func servesEverything(t *testing.T) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, ":embedContent") {
			writeBody(t, w, `{"embedding":{"values":[0.5]}}`)
			return
		}
		writeBody(t, w, `{"totalTokens":1}`)
	}
}

// A save that introduces a model its location does not serve is refused
// naming the lane, the model and the location; so is one with no usable key.
func TestSavingRefusesAVertexModelItsLocationDoesNotServe(t *testing.T) {
	t.Parallel()
	selector, _ := googleAt(t, servedAt(t))
	for name, tc := range map[string]struct {
		keys  config.Lookup
		cfg   RoutingConfig
		names []string
	}{
		"not served":      {allCloudKeys(t), vertexRouting("eu"), []string{"tier premium", `"gemini-3.5-flash"`, `"eu"`, "does not serve"}},
		"no key held":     {noCloudKeys(), vertexRouting("europe-west4"), []string{"tier premium", "holds no service-account key"}},
		"an unusable key": {cloudKeyFor(providerGeminiVertex, "not json"), vertexRouting("europe-west4"), []string{"tier premium", "not usable", "Provider keys"}},
		"unserved embedder": {allCloudKeys(t), func() RoutingConfig {
			cfg := vertexRouting("europe-west4")
			cfg.Embeddings.Model = "gemini-embedding-000"
			return cfg
		}(), []string{"embeddings", `"gemini-embedding-000"`}},
	} {
		store := &RoutingStore{keys: tc.keys, selectBrain: selector}
		err := store.probeVertexBindings(context.Background(), RoutingConfig{}, tc.cfg)
		if err == nil {
			t.Errorf("%s: admitted", name)
			continue
		}
		for _, want := range tc.names {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("%s: %q does not say %s", name, err, want)
			}
		}
	}
	store := &RoutingStore{keys: allCloudKeys(t), selectBrain: selector}
	if err := store.probeVertexBindings(context.Background(), RoutingConfig{}, vertexRouting("europe-west4")); err != nil {
		t.Errorf("a served model and embedder were refused: %v", err)
	}
}

// A binding stored already was asked when it was saved, so re-pointing an
// unrelated lane asks Google nothing — not even for a token.
func TestAnEditThatLeavesTheVertexBindingsAloneAsksGoogleNothing(t *testing.T) {
	t.Parallel()
	selector, google := googleAt(t, servedAt(t))
	store := &RoutingStore{keys: allCloudKeys(t), selectBrain: selector}
	stored := vertexRouting("europe-west4")
	next := vertexRouting("europe-west4")
	next.Tiers[TierLocalSmall] = ProviderConfig{Provider: providerOllama, Model: "qwen3"}

	if err := store.probeVertexBindings(context.Background(), stored, next); err != nil {
		t.Fatalf("an unrelated edit was refused: %v", err)
	}
	if n := google.requests.Load(); n != 0 {
		t.Errorf("an unrelated edit sent %d request(s) to Google", n)
	}
	next.Tiers[TierPremium] = ProviderConfig{Provider: providerGeminiVertex, Location: "europe-west4", Model: "gemini-3.5-flash-lite"}
	if err := store.probeVertexBindings(context.Background(), stored, next); err == nil {
		t.Error("a changed vertex model the location does not serve was admitted")
	}
	if n := google.probes(); n != 1 {
		t.Errorf("one changed binding was probed %d times, want 1 (the unchanged embedder is not re-asked)", n)
	}
}

// Lanes naming one model at one location raise one question, and every
// location a save names shares one token exchange.
func TestOneSaveAsksEachQuestionOnceOnOneToken(t *testing.T) {
	t.Parallel()
	selector, google := googleAt(t, servesEverything(t))
	store := &RoutingStore{keys: allCloudKeys(t), selectBrain: selector}
	next := vertexRouting("europe-west4")
	next.Tiers[TierFrontier] = next.Tiers[TierPremium]
	next.Tiers[TierCheapCloud] = ProviderConfig{Provider: providerGeminiVertex, Location: "eu", Model: "gemini-3.5-flash"}

	if err := store.probeVertexBindings(context.Background(), RoutingConfig{}, next); err != nil {
		t.Fatalf("a served binding was refused: %v", err)
	}
	if n := google.probes(); n != 3 {
		t.Errorf("probes = %d, want 3: frontier and premium share one question, cheap_cloud and the embedder ask their own", n)
	}
	if n := google.exchanges.Load(); n != 1 {
		t.Errorf("token exchanges = %d, want 1 for the whole save", n)
	}
}

// Google failing to answer is not an answer: the save is admitted and the
// unchecked binding is logged, so an outage there never freezes routing here.
func TestASaveGoogleCannotAnswerForIsAdmittedAndLogged(t *testing.T) {
	t.Parallel()
	for name, handler := range map[string]http.HandlerFunc{
		"unavailable": func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusServiceUnavailable) },
		"a project not found": func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			writeBody(t, w, `{"error":{"code":404,"status":"NOT_FOUND","message":"Project 'margince-eu-1' not found or deleted."}}`)
		},
	} {
		selector, google := googleAt(t, handler)
		var logged bytes.Buffer
		store := &RoutingStore{keys: allCloudKeys(t), selectBrain: selector, log: slog.New(slog.NewTextHandler(&logged, nil))}

		if err := store.probeVertexBindings(context.Background(), RoutingConfig{}, vertexRouting("europe-west4")); err != nil {
			t.Errorf("%s: the save was refused: %v", name, err)
		}
		if google.probes() == 0 {
			t.Errorf("%s: nothing reached Google, so the admission proves nothing", name)
		}
		for _, want := range []string{"level=WARN", "unchecked", "location=europe-west4", "model=gemini-3.5-flash"} {
			if !strings.Contains(logged.String(), want) {
				t.Errorf("%s: the log %q does not say %s", name, logged.String(), want)
			}
		}
		if strings.Contains(logged.String(), "ya29.") {
			t.Errorf("%s: the log carries the access token", name)
		}
	}
}

// A save that binds no gemini_vertex lane never reads the stored binding: the
// store below has no settings to read, and would panic if it tried.
func TestASaveWithoutVertexReadsAndProbesNothing(t *testing.T) {
	t.Parallel()
	store := &RoutingStore{selectBrain: func(ProviderConfig, config.Lookup) (model.Client, error) {
		t.Error("a binding without gemini_vertex built a client on save")
		return nil, errors.New("unreachable")
	}}
	cfg := RoutingConfig{
		Tiers:      map[Tier]ProviderConfig{TierPremium: {Provider: providerAnthropic, Model: "m"}},
		Embeddings: EmbeddingsConfig{ProviderConfig: ProviderConfig{Provider: ProviderFake, Model: "e"}},
	}
	stored, err := store.storedBeforeProbe(context.Background(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.probeVertexBindings(context.Background(), stored, cfg); err != nil {
		t.Errorf("probing a binding without vertex: %v", err)
	}
}

// A model id is a path segment of every Vertex call, so one that would add a
// segment, a query or a fragment is refused before anything is sent.
func TestAModelIDThatWouldReshapeTheVertexPathIsRefused(t *testing.T) {
	t.Parallel()
	selector, google := googleAt(t, servesEverything(t))
	store := &RoutingStore{keys: allCloudKeys(t), selectBrain: selector}
	for _, id := range []string{"../../locations/us/x", "gemini?alt=sse", "gemini#x", "a/b", "Gemini-3.5"} {
		_, err := store.ListAvailableModels(routingReader(), AvailableModelsQuery{Provider: providerGeminiVertex, Location: "eu", Model: id})
		if !errors.Is(err, apperrors.ErrInvalidArgument) {
			t.Errorf("probe of %q: want an invalid-argument refusal, got %v", id, err)
		}
		binding := ProviderConfig{Provider: providerGeminiVertex, Location: "eu", Model: id}
		if err := ValidateTierBinding(ProfileCloudHosted, TierPremium, binding); err == nil {
			t.Errorf("a binding naming model %q was admitted", id)
		}
	}
	if n := google.requests.Load(); n != 0 {
		t.Errorf("%d request(s) left for a model id that was refused", n)
	}
	got := vertexTransport{host: "https://h", projectID: "p", location: "eu"}.modelURL("a/b?c#d", "countTokens")
	if want := "https://h/v1/projects/p/locations/eu/publishers/google/models/a%2Fb%3Fc%23d:countTokens"; got != want {
		t.Errorf("modelURL = %q, want the id escaped as one segment: %q", got, want)
	}
}

// With no location asked and none stored there is no host to ask, under
// every profile; saying profile_forbids would blame the wrong thing.
func TestAVertexListWithNoLocationHasNoEndpoint(t *testing.T) {
	t.Parallel()
	selector, google := googleAt(t, servesEverything(t))
	store := &RoutingStore{keys: allCloudKeys(t), selectBrain: selector}
	for _, profile := range []Profile{ProfileEUResident, ProfileCloudHosted} {
		got := store.availableModels(context.Background(), RoutingConfig{Profile: profile}, AvailableModelsQuery{Provider: providerGeminiVertex, Tier: "premium"})
		if got.Unavailable != AvailabilityNoEndpoint {
			t.Errorf("%s: unavailable = %q, want no_endpoint", profile, got.Unavailable)
		}
	}
	if n := google.requests.Load(); n != 0 {
		t.Errorf("%d request(s) left with no location to send them to", n)
	}
	stored := RoutingConfig{Profile: ProfileEUResident, Tiers: map[Tier]ProviderConfig{
		TierFrontier: {Provider: providerGeminiVertex, Location: "europe-west4", Model: "gemini-3.5-flash"},
	}}
	if got := store.availableModels(context.Background(), stored, AvailableModelsQuery{Provider: providerGeminiVertex, Tier: "premium"}); got.Unavailable != AvailabilityOK {
		t.Errorf("a stored location on another lane was not used: %q", got.Unavailable)
	}
}

// A stored key file that cannot be used is a key to replace, not an adapter
// this build lacks.
func TestAnUnusableServiceAccountKeyReadsAsNoKey(t *testing.T) {
	t.Parallel()
	store := &RoutingStore{keys: cloudKeyFor(providerGeminiVertex, `{"type":"authorized_user"}`)}
	got, err := store.ListProviderLocations(routingReader(), providerGeminiVertex)
	if err != nil || got.Unavailable != AvailabilityNoKey {
		t.Errorf("locations: %+v, %v; want no_key", got, err)
	}
	bound := ProviderConfig{Provider: providerGeminiVertex, Location: "eu"}
	q := AvailableModelsQuery{Provider: providerGeminiVertex, Location: "eu", Model: "gemini-3.5-flash"}
	if probed := store.probeAvailability(context.Background(), bound, q); probed.Unavailable != AvailabilityNoKey {
		t.Errorf("probe: %+v; want no_key", probed)
	}
}

// Only Vertex naming the publisher model is "not served here"; any other
// NOT_FOUND, AI Studio's included, is a fault.
func TestOnlyAMissingPublisherModelReadsAsNotServed(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		body string
		want bool
	}{
		"vertex, the model":   {`{"error":{"code":404,"status":"NOT_FOUND","message":"Publisher Model ` + "`projects/p/locations/eu/publishers/google/models/x`" + ` was not found or your project does not have access to it."}}`, true},
		"vertex, the project": {`{"error":{"code":404,"status":"NOT_FOUND","message":"Project 'p' not found or deleted."}}`, false},
		"ai studio":           {`{"error":{"code":404,"status":"NOT_FOUND","message":"models/x is not found for API version v1beta, or is not supported for generateContent."}}`, false},
		"no json":             {`<html>Not Found</html>`, false},
	} {
		err := geminiError(t.Context(), &http.Response{StatusCode: http.StatusNotFound, Body: io.NopCloser(strings.NewReader(tc.body))})
		if got := errors.Is(err, errModelNotFound); got != tc.want {
			t.Errorf("%s: not served = %v, want %v (%v)", name, got, tc.want, err)
		}
	}
}
