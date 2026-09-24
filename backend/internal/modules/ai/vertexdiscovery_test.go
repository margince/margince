// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/platform/settings"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// countingGoogle counts every request a client sends, the token exchange
// included, before googleFixture answers or reroutes it.
type countingGoogle struct {
	googleFixture
	requests atomic.Int32
}

func (g *countingGoogle) RoundTrip(req *http.Request) (*http.Response, error) {
	g.requests.Add(1)
	return g.googleFixture.RoundTrip(req)
}

// googleAt serves handler as every Google host, and returns the selector a
// store builds its clients through.
func googleAt(t *testing.T, handler http.HandlerFunc) (brainSelector, *countingGoogle) {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	google := &countingGoogle{googleFixture: googleFixture{base: srv.URL}}
	return func(cfg ProviderConfig, keys config.Lookup) (model.Client, error) {
		return selectBrainOn(cfg, keys, &http.Client{Timeout: CallCeiling, Transport: google})
	}, google
}

func routingReader() context.Context { return keySeatCtx(principal.ObjectGrant{Read: true}) }

func TestTheLocationListIsGooglesOptionsStampedWithThisBuildsResidency(t *testing.T) {
	t.Parallel()
	selector, _ := googleAt(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/projects/margince-eu-1/locations" || r.Header.Get("Authorization") != "Bearer ya29.fixture" {
			http.NotFound(w, r)
			return
		}
		if r.URL.Query().Get("pageToken") == "" {
			writeBody(t, w, `{"locations":[{"locationId":"europe-west4","displayName":"Netherlands"},{"locationId":"europe-west2","displayName":"London"},{"locationId":"attacker.example","displayName":"x"}],"nextPageToken":"p2"}`)
			return
		}
		writeBody(t, w, `{"locations":[{"locationId":"europe-west10","displayName":"Berlin"},{"locationId":"us-central1","displayName":"Iowa"}]}`)
	})
	store := &RoutingStore{keys: allCloudKeys(t), selectBrain: selector}

	got, err := store.ListProviderLocations(routingReader(), providerGeminiVertex)
	if err != nil || got.Unavailable != AvailabilityOK {
		t.Fatalf("listing: %+v, %v", got, err)
	}
	byID := map[string]ProviderLocation{}
	for _, l := range got.Locations {
		byID[l.ID] = l
	}
	want := map[string]ProviderLocation{
		"eu":            {ID: "eu", DisplayName: "EU (multi-region)", Jurisdiction: "eu", Resident: true},
		"us":            {ID: "us", DisplayName: "US (multi-region)", Jurisdiction: "us"},
		"global":        {ID: "global", DisplayName: "Global", Jurisdiction: "global"},
		"europe-west4":  {ID: "europe-west4", DisplayName: "Netherlands", Jurisdiction: "eu", Resident: true},
		"europe-west2":  {ID: "europe-west2", DisplayName: "London", Jurisdiction: "other"},
		"europe-west10": {ID: "europe-west10", DisplayName: "Berlin", Jurisdiction: "other"},
		"us-central1":   {ID: "us-central1", DisplayName: "Iowa", Jurisdiction: "us"},
	}
	if len(byID) != len(want) {
		t.Errorf("locations = %+v, want exactly %d (a shape no binding can name is dropped)", got.Locations, len(want))
	}
	for id, w := range want {
		if byID[id] != w {
			t.Errorf("%s = %+v, want %+v", id, byID[id], w)
		}
	}
}

func TestTheLocationListSaysWhyItIsEmpty(t *testing.T) {
	t.Parallel()
	selector, google := googleAt(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})
	for name, tc := range map[string]struct {
		provider string
		keys     config.Lookup
		want     ModelAvailability
	}{
		"another vendor":   {providerGemini, allCloudKeys(t), AvailabilityNotPublished},
		"no key":           {providerGeminiVertex, noCloudKeys(), AvailabilityNoKey},
		"google refuses":   {providerGeminiVertex, allCloudKeys(t), AvailabilityUnreachable},
		"an unknown asker": {"nobody", allCloudKeys(t), AvailabilityNotPublished},
	} {
		store := &RoutingStore{keys: tc.keys, selectBrain: selector}
		got, err := store.ListProviderLocations(routingReader(), tc.provider)
		if err != nil || got.Unavailable != tc.want || len(got.Locations) != 0 {
			t.Errorf("%s: %+v, %v; want %q and no locations", name, got, err, tc.want)
		}
	}
	if google.requests.Load() == 0 {
		t.Error("the refusing case never reached Google, so it proves nothing about unreachable")
	}
}

func TestTheLocationListNeedsTheRoutingReadGrant(t *testing.T) {
	t.Parallel()
	selector, google := googleAt(t, http.NotFound)
	store := &RoutingStore{keys: allCloudKeys(t), selectBrain: selector}
	_, err := store.ListProviderLocations(keySeatCtx(principal.ObjectGrant{}), providerGeminiVertex)
	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("want a permission denial, got %v", err)
	}
	if n := google.requests.Load(); n != 0 {
		t.Errorf("a refused reader still caused %d request(s)", n)
	}
}

// servedAt answers the probe verbs: only gemini-3.5-flash and the embedding
// model are served, at europe-west4 alone.
func servedAt(t *testing.T) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		served := strings.HasPrefix(r.URL.Path, "/v1/projects/margince-eu-1/locations/europe-west4/publishers/google/models/") &&
			(strings.HasSuffix(r.URL.Path, "/gemini-3.5-flash:countTokens") || strings.HasSuffix(r.URL.Path, "/gemini-embedding-001:embedContent"))
		switch {
		case !served:
			w.WriteHeader(http.StatusNotFound)
			writeBody(t, w, `{"error":{"code":404,"status":"NOT_FOUND","message":"Publisher model not found."}}`)
		case strings.HasSuffix(r.URL.Path, ":embedContent"):
			writeBody(t, w, `{"embedding":{"values":[0.5]}}`)
		default:
			writeBody(t, w, `{"totalTokens":1}`)
		}
	}
}

func TestAProbeSaysWhetherALocationServesOneModel(t *testing.T) {
	t.Parallel()
	selector, _ := googleAt(t, servedAt(t))
	store := &RoutingStore{keys: allCloudKeys(t), selectBrain: selector}
	cfg := RoutingConfig{Profile: ProfileEUResident}
	for name, tc := range map[string]struct {
		q    AvailableModelsQuery
		want ModelAvailability
	}{
		"served chat":             {AvailableModelsQuery{Tier: "premium", Location: "europe-west4", Model: "gemini-3.5-flash"}, AvailabilityOK},
		"served embedder":         {AvailableModelsQuery{Tier: "embeddings", Location: "europe-west4", Model: "gemini-embedding-001"}, AvailabilityOK},
		"not served here":         {AvailableModelsQuery{Tier: "premium", Location: "eu", Model: "gemini-3.5-flash"}, AvailabilityNoEndpoint},
		"an unknown model":        {AvailableModelsQuery{Tier: "premium", Location: "europe-west4", Model: "gemini-0"}, AvailabilityNoEndpoint},
		"embedder on a chat lane": {AvailableModelsQuery{Tier: "premium", Location: "europe-west4", Model: "gemini-embedding-001"}, AvailabilityNoEndpoint},
	} {
		tc.q.Provider = providerGeminiVertex
		got := store.availableModels(context.Background(), cfg, tc.q)
		if got.Unavailable != tc.want {
			t.Errorf("%s: unavailable = %q, want %q", name, got.Unavailable, tc.want)
			continue
		}
		if tc.want == AvailabilityOK && (len(got.Models) != 1 || got.Models[0].ID != tc.q.Model || got.Models[0].Lane != probeLane(tc.q.Tier)) {
			t.Errorf("%s: models = %+v, want just the probed one", name, got.Models)
		}
	}
	other := store.availableModels(context.Background(), cfg, AvailableModelsQuery{Provider: "ollama", Model: "gemma3"})
	if other.Unavailable != AvailabilityNotPublished {
		t.Errorf("a vendor with no per-location availability answered %q, want not_published", other.Unavailable)
	}
}

// The residency refusal comes before any client is built: not one request —
// not even the token exchange — reaches Google for a location eu_resident
// refuses, whether the screen lists models there or probes one.
func TestEUResidentAsksGoogleNothingAboutANonResidentLocation(t *testing.T) {
	t.Parallel()
	selector, google := googleAt(t, servedAt(t))
	store := &RoutingStore{keys: allCloudKeys(t), selectBrain: selector}
	cfg := RoutingConfig{Profile: ProfileEUResident}
	for _, q := range []AvailableModelsQuery{
		{Provider: providerGeminiVertex, Tier: "premium", Location: "europe-west2"},
		{Provider: providerGeminiVertex, Tier: "premium", Location: "us", Model: "gemini-3.5-flash"},
		{Provider: providerGeminiVertex, Tier: "embeddings", Location: "global", Model: "gemini-embedding-001"},
	} {
		if got := store.availableModels(context.Background(), cfg, q); got.Unavailable != AvailabilityProfileForbids {
			t.Errorf("%+v answered %q, want profile_forbids", q, got.Unavailable)
		}
	}
	if n := google.requests.Load(); n != 0 {
		t.Errorf("%d request(s) reached Google for locations eu_resident refuses", n)
	}
	// The control: the same store does reach Google for a resident location.
	store.availableModels(context.Background(), cfg, AvailableModelsQuery{Provider: providerGeminiVertex, Location: "europe-west4", Model: "gemini-3.5-flash"})
	if google.requests.Load() == 0 {
		t.Error("a resident location reached nobody either, so the zero above proves nothing")
	}
}

func TestAMalformedLocationIsRefusedAsTheCallersFault(t *testing.T) {
	t.Parallel()
	store := &RoutingStore{keys: allCloudKeys(t)}
	_, err := store.ListAvailableModels(routingReader(), AvailableModelsQuery{Provider: providerGeminiVertex, Location: "eu.attacker.example"})
	if !errors.Is(err, apperrors.ErrInvalidArgument) {
		t.Fatalf("want an invalid-argument refusal, got %v", err)
	}
}

func vertexRouting(location string) RoutingConfig {
	return RoutingConfig{
		Profile: ProfileEUResident,
		Tiers: map[Tier]ProviderConfig{
			TierPremium:    {Provider: providerGeminiVertex, Location: location, Model: "gemini-3.5-flash"},
			TierLocalSmall: {Provider: providerOllama, Model: "gemma3"},
		},
		Embeddings: EmbeddingsConfig{ProviderConfig: ProviderConfig{Provider: providerGeminiVertex, Location: location, Model: "gemini-embedding-001"}},
	}
}

// Saving re-asks each gemini_vertex binding's location, and a model it does
// not serve is the admin's 422 naming the tier, the model and the location —
// refused before the write, so nothing unservable is stored.
func TestSavingRefusesAVertexModelItsLocationDoesNotServe(t *testing.T) {
	t.Parallel()
	selector, _ := googleAt(t, servedAt(t))
	admin := keySeatCtx(principal.ObjectGrant{Read: true, Update: true})
	for name, tc := range map[string]struct {
		keys  config.Lookup
		cfg   RoutingConfig
		names []string
	}{
		"not served":  {allCloudKeys(t), vertexRouting("eu"), []string{"tier premium", `"gemini-3.5-flash"`, `"eu"`, "does not serve"}},
		"no key held": {noCloudKeys(), vertexRouting("europe-west4"), []string{"tier premium", "holds no service-account key"}},
	} {
		store := &RoutingStore{keys: tc.keys, selectBrain: selector}
		_, err := store.ReplaceIfVersion(admin, tc.cfg, "")
		var invalid settings.InvalidValue
		if !errors.As(err, &invalid) {
			t.Fatalf("%s: want a 422-shaped refusal, got %v", name, err)
		}
		for _, want := range tc.names {
			if !strings.Contains(invalid.Reason, want) {
				t.Errorf("%s: %q does not say %s", name, invalid.Reason, want)
			}
		}
	}
	store := &RoutingStore{keys: allCloudKeys(t), selectBrain: selector}
	if err := store.probeVertexBindings(context.Background(), vertexRouting("europe-west4")); err != nil {
		t.Errorf("a served model and embedder were refused: %v", err)
	}
	embeddingsOnly := vertexRouting("europe-west4")
	embeddingsOnly.Embeddings.Model = "gemini-embedding-000"
	if err := store.probeVertexBindings(context.Background(), embeddingsOnly); err == nil || !strings.Contains(err.Error(), "embeddings") {
		t.Errorf("an unserved embedder was admitted: %v", err)
	}
}

// A binding with no gemini_vertex lane is never probed, so every save that
// was valid before this check is valid now and makes no call.
func TestSavingABindingWithoutVertexProbesNothing(t *testing.T) {
	t.Parallel()
	store := &RoutingStore{selectBrain: func(ProviderConfig, config.Lookup) (model.Client, error) {
		t.Error("a binding without gemini_vertex built a client on save")
		return nil, errors.New("unreachable")
	}}
	cfg := RoutingConfig{
		Tiers:      map[Tier]ProviderConfig{TierPremium: {Provider: providerAnthropic, Model: "m"}},
		Embeddings: EmbeddingsConfig{ProviderConfig: ProviderConfig{Provider: ProviderFake, Model: "e"}},
	}
	if err := store.probeVertexBindings(context.Background(), cfg); err != nil {
		t.Errorf("probing a binding without vertex: %v", err)
	}
}
