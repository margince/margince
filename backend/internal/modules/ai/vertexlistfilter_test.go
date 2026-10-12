// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"context"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"
)

// googleWithCatalog lists catalog as Google's publisher models and serves a
// model at a location only where served names it as "location/model".
func googleWithCatalog(t *testing.T, catalog []string, served map[string]bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/publishers/google/models") {
			var names []string
			for _, id := range catalog {
				names = append(names, `{"name":"publishers/google/models/`+id+`"}`)
			}
			writeBody(t, w, `{"publisherModels":[`+strings.Join(names, ",")+`]}`)
			return
		}
		// /v1/projects/<p>/locations/<loc>/publishers/google/models/<id>:<verb>
		parts := strings.Split(r.URL.Path, "/")
		loc, last := parts[5], parts[len(parts)-1]
		id, _, _ := strings.Cut(last, ":")
		if !served[loc+"/"+id] {
			w.WriteHeader(http.StatusNotFound)
			writeBody(t, w, `{"error":{"code":404,"status":"NOT_FOUND","message":"Publisher Model was not found or your project does not have access to it."}}`)
			return
		}
		if strings.HasSuffix(last, ":predict") {
			writeBody(t, w, `{"predictions":[{"embeddings":{"values":[0.5]}}]}`)
			return
		}
		writeBody(t, w, `{"totalTokens":1}`)
	}
}

func listedIDs(got AvailableModels) []string {
	var ids []string
	for _, m := range got.Models {
		ids = append(ids, m.ID)
	}
	slices.Sort(ids)
	return ids
}

// The publisher catalog is global; what a location serves is asked of the
// location. A model it does not serve is not offered there, and neither is a
// speech, live-audio or image model no binding here can call.
func TestAVertexListOffersOnlyWhatTheLocationServes(t *testing.T) {
	t.Parallel()
	catalog := []string{"gemini-3.5-flash", "gemini-2.5-pro", "gemini-2.5-flash-tts", "gemini-live-2.5-flash-native-audio", "gemini-3.1-flash-image-preview", "gemini-embedding-001"}
	served := map[string]bool{
		"europe-west4/gemini-3.5-flash":     true,
		"europe-west4/gemini-embedding-001": true,
		// Served, and still not offered: no binding here can call a speech model.
		"europe-west4/gemini-2.5-flash-tts": true,
		"us-central1/gemini-2.5-pro":        true,
	}
	selector, _ := googleAt(t, googleWithCatalog(t, catalog, served))
	store := &RoutingStore{keys: allCloudKeys(t), selectBrain: selector}

	got := store.availableModels(context.Background(), RoutingConfig{Profile: ProfileEUHosted},
		AvailableModelsQuery{Provider: providerGeminiVertex, Tier: "premium", Location: "europe-west4"})

	if want := []string{"gemini-3.5-flash", "gemini-embedding-001"}; !slices.Equal(listedIDs(got), want) {
		t.Errorf("europe-west4 offers %v, want %v", listedIDs(got), want)
	}
	if !got.Complete {
		t.Error("a list asked of the location is not marked complete, so a picker adds models it does not serve")
	}
}

// Google not answering a probe is not an answer: the model stays on offer
// rather than vanishing because a call timed out.
func TestAVertexModelGoogleCouldNotBeAskedAboutStaysOnOffer(t *testing.T) {
	t.Parallel()
	selector, _ := googleAt(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/publishers/google/models") {
			writeBody(t, w, `{"publisherModels":[{"name":"publishers/google/models/gemini-3.5-flash"}]}`)
			return
		}
		w.WriteHeader(http.StatusServiceUnavailable)
	})
	store := &RoutingStore{keys: allCloudKeys(t), selectBrain: selector}

	got := store.availableModels(context.Background(), RoutingConfig{Profile: ProfileEUHosted},
		AvailableModelsQuery{Provider: providerGeminiVertex, Tier: "premium", Location: "europe-west4"})

	if !slices.Equal(listedIDs(got), []string{"gemini-3.5-flash"}) {
		t.Errorf("offered %v, want the unanswered model kept", listedIDs(got))
	}
	if got.Complete {
		t.Error("a list Google did not fully answer is marked complete, so a picker drops models it may serve")
	}
}

// What a location serves is asked once per location for a while, not on
// every open of the picker.
func TestAVertexLocationsAnswerIsReusedForAWhile(t *testing.T) {
	t.Parallel()
	catalog := []string{"gemini-3.5-flash"}
	selector, google := googleAt(t, googleWithCatalog(t, catalog, map[string]bool{"europe-west4/gemini-3.5-flash": true}))
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	store := &RoutingStore{keys: allCloudKeys(t), selectBrain: selector, now: func() time.Time { return now }, served: &servedAtLocation{}}
	q := AvailableModelsQuery{Provider: providerGeminiVertex, Tier: "premium", Location: "europe-west4"}

	store.availableModels(context.Background(), RoutingConfig{}, q)
	first := google.probes()
	store.availableModels(context.Background(), RoutingConfig{}, q)
	if again := google.probes() - first; again != 1 {
		t.Errorf("a second open within the hour sent %d request(s), want 1 (the list alone)", again)
	}
	now = now.Add(2 * time.Hour)
	store.availableModels(context.Background(), RoutingConfig{}, q)
	if later := google.probes() - first; later != 3 {
		t.Errorf("an open two hours later sent %d request(s) in all since the first, want 3 (the reused list, then the list and the probe)", later)
	}
}

// A probe round Google did not answer is not an answer: nothing is reused,
// so the next open asks again rather than offering the whole catalog for an
// hour.
func TestAnUnansweredProbeRoundIsNotReused(t *testing.T) {
	t.Parallel()
	selector, google := googleAt(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/publishers/google/models") {
			writeBody(t, w, `{"publisherModels":[{"name":"publishers/google/models/gemini-3.5-flash"}]}`)
			return
		}
		w.WriteHeader(http.StatusServiceUnavailable)
	})
	store := &RoutingStore{keys: allCloudKeys(t), selectBrain: selector, served: &servedAtLocation{}}
	q := AvailableModelsQuery{Provider: providerGeminiVertex, Tier: "premium", Location: "europe-west4"}

	store.availableModels(context.Background(), RoutingConfig{}, q)
	first := google.probes()
	store.availableModels(context.Background(), RoutingConfig{}, q)
	if again := google.probes() - first; again != 2 {
		t.Errorf("a second open after an unanswered round sent %d request(s), want 2 (the list and the probe again)", again)
	}
}

// What a location serves differs by Google project, so a key from another
// project is not answered from the first one's lineup.
func TestALocationsAnswerIsKeptPerProject(t *testing.T) {
	cache := &servedAtLocation{}
	at := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	cache.remember("p1/europe-west4", servedAnswer{at: at, served: map[string]bool{"m": true}})
	if got := cache.lookup("p2/europe-west4", at); got != nil {
		t.Errorf("another project's location answered from the first's lineup: %v", got)
	}
	if got := cache.lookup("p1/europe-west4", at); !got["m"] {
		t.Error("the same project's answer was not reused")
	}
}

// A model the catalog gained since a location's answer was kept was never
// asked about there: it stays on offer, and the list is not exact.
func TestAReusedAnswerMissingAModelIsNotComplete(t *testing.T) {
	t.Parallel()
	catalog := []string{"gemini-3.5-flash", "gemini-3.6-flash"}
	selector, _ := googleAt(t, googleWithCatalog(t, catalog, map[string]bool{"europe-west4/gemini-3.5-flash": true}))
	now := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	store := &RoutingStore{keys: allCloudKeys(t), selectBrain: selector, now: func() time.Time { return now }, served: &servedAtLocation{}}
	store.served.remember("margince-eu-1/europe-west4", servedAnswer{at: now, served: map[string]bool{"gemini-3.5-flash": true}})

	got := store.availableModels(context.Background(), RoutingConfig{},
		AvailableModelsQuery{Provider: providerGeminiVertex, Tier: "premium", Location: "europe-west4"})

	if !slices.Equal(listedIDs(got), catalog) {
		t.Errorf("offered %v, want both: the new model was never asked about", listedIDs(got))
	}
	if got.Complete {
		t.Error("a reused answer that never asked about a listed model is marked complete")
	}
}
