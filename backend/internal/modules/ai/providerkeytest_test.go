// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// stubBuilder binds adapters to an httptest server's loopback address, which
// the production egress guard refuses by design. Decision adapters get a
// transport that fails the test if it is dialled: a case that reaches one
// belongs in decisionprobe_test.go, behind a scripted transport.
var stubBuilder = keyProbes{
	brain: func(cfg ProviderConfig, keys config.Lookup) (model.Client, error) {
		return selectBrainOn(cfg, keys, http.DefaultClient)
	},
	decider: func(lane DecisionsConfig, keys config.Lookup) (*decisionClient, error) {
		return selectDeciderOn(lane, keys, &http.Client{Transport: refuseDial{}})
	},
}

type refuseDial struct{}

func (refuseDial) RoundTrip(r *http.Request) (*http.Response, error) {
	return nil, fmt.Errorf("unexpected dial to %s", r.URL)
}

// vendorAnswering is an OpenAI-wire vendor that answers every list request
// with status, and a routing document binding openai at it.
func vendorAnswering(t *testing.T, status int) RoutingConfig {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
		if status != http.StatusOK {
			return
		}
		if _, err := w.Write([]byte(`{"data":[{"id":"gpt-a"},{"id":"gpt-b"}]}`)); err != nil {
			t.Errorf("writing the fixture body: %v", err)
		}
	}))
	t.Cleanup(srv.Close)
	return boundAt(providerOpenAI, srv.URL)
}

func boundAt(provider, baseURL string) RoutingConfig {
	return RoutingConfig{
		Profile: ProfileCloudFrontier,
		Tiers:   map[Tier]ProviderConfig{"premium": {Provider: provider, Model: "m", BaseURL: baseURL}},
	}
}

func TestAKeyTheVendorAcceptsReportsHowManyModelsItServes(t *testing.T) {
	cfg := vendorAnswering(t, http.StatusOK)
	got := probeProviderKey(context.Background(), cfg, providerOpenAI, cloudKeyFor(providerOpenAI, "k"), stubBuilder)
	if !got.OK || got.ModelCount != 2 || !got.Counted || got.Reason != "" {
		t.Fatalf("an accepted key should pass with two models and no reason: %+v", got)
	}
}

// listingServer answers every request with a two-model OpenAI-wire list.
func listingServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if _, err := w.Write([]byte(`{"data":[{"id":"gpt-a"},{"id":"gpt-b"}]}`)); err != nil {
			t.Errorf("writing the fixture body: %v", err)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// A key is tested where its provider is configured, before any lane binds it:
// setting the host and pressing Test comes before picking models.
func TestKeyTest_UsesTheProviderHostWithNoBinding(t *testing.T) {
	srv := listingServer(t)
	cfg := RoutingConfig{
		Profile:   ProfileCloudFrontier,
		Providers: map[string]ProviderSettings{providerOpenAICompatible: {BaseURL: srv.URL}},
	}
	got := probeProviderKey(context.Background(), cfg, providerOpenAICompatible,
		cloudKeyFor(providerOpenAICompatible, "k"), stubBuilder)
	if !got.OK || got.ModelCount != 2 {
		t.Fatalf("the provider's own host was not tried: %+v", got)
	}
}

// A lane naming the provider lends it no host: with no entry the provider has
// none, and the answer is the address to fill in.
func TestKeyTest_NoHostIsNoEndpoint(t *testing.T) {
	cfg := RoutingConfig{
		Profile:   ProfileCloudFrontier,
		Providers: map[string]ProviderSettings{providerOllama: {BaseURL: "http://gpu.internal.test:11434"}},
		Tiers:     map[Tier]ProviderConfig{TierPremium: {Provider: providerOpenAICompatible, Model: "m"}},
	}
	got := probeProviderKey(context.Background(), cfg, providerOpenAICompatible,
		cloudKeyFor(providerOpenAICompatible, "k"), stubBuilder)
	if got.OK || got.Reason != KeyTestNoEndpoint {
		t.Fatalf("got %+v, want no_endpoint", got)
	}
}

// An installation whose only openai_compatible use is a separate embeddings
// server tests the key there rather than answering no_endpoint.
func TestKeyTest_ReachesAnEmbeddingsOnlyServer(t *testing.T) {
	srv := listingServer(t)
	cfg := RoutingConfig{
		Profile:    ProfileCloudFrontier,
		Providers:  map[string]ProviderSettings{providerGemini: {}},
		Tiers:      map[Tier]ProviderConfig{TierPremium: {Provider: providerGemini, Model: "g"}},
		Embeddings: EmbeddingsConfig{ProviderConfig: ProviderConfig{Provider: providerOpenAICompatible, Model: "e", BaseURL: srv.URL}},
	}
	got := probeProviderKey(context.Background(), cfg, providerOpenAICompatible,
		cloudKeyFor(providerOpenAICompatible, "k"), stubBuilder)
	if !got.OK || got.ModelCount != 2 {
		t.Fatalf("the embeddings server was not tried: %+v", got)
	}
}

// The distinction the button exists for: a refused credential, a throttled
// one and a vendor that is down are three different things to do next.
func TestAVendorRefusalIsToldApartByItsStatus(t *testing.T) {
	cases := map[int]KeyTestReason{
		http.StatusUnauthorized:        KeyTestAuthFailed,
		http.StatusForbidden:           KeyTestPermissionDenied,
		http.StatusTooManyRequests:     KeyTestRateLimited,
		http.StatusInternalServerError: KeyTestUnreachable,
		http.StatusNotFound:            KeyTestUnreachable,
	}
	for status, want := range cases {
		cfg := vendorAnswering(t, status)
		got := probeProviderKey(context.Background(), cfg, providerOpenAI, cloudKeyFor(providerOpenAI, "k"), stubBuilder)
		if got.OK || got.Reason != want {
			t.Errorf("http %d: got %+v, want reason %q", status, got, want)
		}
	}
}

func TestAVendorThatDoesNotAnswerIsUnreachable(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	srv.Close()
	got := probeProviderKey(context.Background(), boundAt(providerOpenAI, srv.URL), providerOpenAI,
		cloudKeyFor(providerOpenAI, "k"), stubBuilder)
	if got.OK || got.Reason != KeyTestUnreachable {
		t.Fatalf("a closed port should be unreachable: %+v", got)
	}
}

// Each of these is decided before any vendor is dialled, so a real builder
// would answer the same way; the stub only keeps a mistake from reaching out.
func TestAKeyThatCannotBeTestedSaysWhy(t *testing.T) {
	cases := []struct {
		name     string
		cfg      RoutingConfig
		provider string
		keys     config.Lookup
		want     KeyTestReason
	}{
		{"no key held", boundAt(providerOpenAI, ""), providerOpenAI, noCloudKeys(), KeyTestNoKey},
		{
			"broker with no host",
			RoutingConfig{Profile: ProfileCloudFrontier},
			providerOpenAICompatible,
			cloudKeyFor(providerOpenAICompatible, "k"), KeyTestNoEndpoint,
		},
		{
			"sovereign forbids a cloud vendor",
			RoutingConfig{Profile: ProfileSovereign},
			providerAnthropic,
			cloudKeyFor(providerAnthropic, "k"), KeyTestProfileForbids,
		},
		{
			"a decision broker with no host",
			RoutingConfig{Profile: ProfileCloudFrontier},
			providerJevCompatible,
			noCloudKeys(), KeyTestNoEndpoint,
		},
		{
			"a decision vendor with no key",
			RoutingConfig{Profile: ProfileCloudFrontier},
			providerJev,
			noCloudKeys(), KeyTestNoKey,
		},
		{
			"an adapter this build does not carry",
			RoutingConfig{Profile: ProfileCloudFrontier},
			"not-a-vendor",
			noCloudKeys(), KeyTestNotPublished,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := probeProviderKey(context.Background(), tc.cfg, tc.provider, tc.keys, stubBuilder)
			if got.OK || got.Reason != tc.want || got.Provider != tc.provider {
				t.Fatalf("got %+v, want reason %q", got, tc.want)
			}
		})
	}
}

// A client with no list endpoint cannot be tested by listing, and saying
// "unreachable" would send a reader after a network fault that is not there.
func TestAClientWithNoListIsNotPublished(t *testing.T) {
	unlisted := keyProbes{brain: func(ProviderConfig, config.Lookup) (model.Client, error) { return unlistedClient{}, nil }}
	got := probeProviderKey(context.Background(), RoutingConfig{Profile: ProfileCloudFrontier}, providerOpenAI,
		noCloudKeys(), unlisted)
	if got.OK || got.Reason != KeyTestNotPublished {
		t.Fatalf("got %+v, want not_published", got)
	}
}

type unlistedClient struct{ model.Client }

func (unlistedClient) Health() model.ProviderHealthStatus {
	return model.ProviderHealthStatus{Health: model.HealthOK}
}

func TestEveryPickerStateHasAKeyTestReading(t *testing.T) {
	cases := map[ModelAvailability]KeyTestReason{
		AvailabilityNoKey:          KeyTestNoKey,
		AvailabilityProfileForbids: KeyTestProfileForbids,
		AvailabilityNotPublished:   KeyTestNotPublished,
		AvailabilityUnreachable:    KeyTestUnreachable,
		AvailabilityNoEndpoint:     KeyTestNoEndpoint,
	}
	for state, want := range cases {
		if got := keyTestRefusal(state); got != want {
			t.Errorf("%q reads as %q, want %q", state, got, want)
		}
	}
}

// Gemini refuses a bad key with 400 and names it API_KEY_INVALID in the
// error's details; read as unreachable, the button would send a reader after
// the network for a key that is simply wrong. The code decides, not the
// status: a bare 400 — a proxy in front of it — is not a refused key.
func TestGeminiReadsItsInvalidKeyCodeAndNothingElse(t *testing.T) {
	cases := map[string]KeyTestReason{
		`{"error":{"code":400,"status":"INVALID_ARGUMENT","details":[{"reason":"API_KEY_INVALID"}]}}`: KeyTestAuthFailed,
		`{"error":{"code":400,"details":[{"reason":"MODEL_NOT_FOUND"}]}}`:                             KeyTestUnreachable,
		`<html>bad request sk-live-abc</html>`:                                                        KeyTestUnreachable,
		``:                                                                                            KeyTestUnreachable,
	}
	for body, want := range cases {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusBadRequest)
			if _, err := w.Write([]byte(body)); err != nil {
				t.Errorf("writing the fixture body: %v", err)
			}
		}))
		got := probeProviderKey(context.Background(), boundAt(providerGemini, srv.URL), providerGemini,
			cloudKeyFor(providerGemini, "k"), stubBuilder)
		srv.Close()
		if got.Reason != want {
			t.Errorf("gemini 400 %q: got %+v, want %q", body, got, want)
		}
	}
}

// The code reads as a refused key only on Gemini's 400: a throttle that names
// it is still a throttle.
func TestGeminisKeyCodeOnAnotherStatusKeepsThatStatusReading(t *testing.T) {
	err := &listStatusError{vendor: geminiListVendor, status: http.StatusTooManyRequests, reason: geminiKeyInvalid}
	if got := keyTestFailure(err); got != KeyTestRateLimited {
		t.Fatalf("a 429 naming API_KEY_INVALID reads %q, want rate_limited", got)
	}
}

// A code only ever arrives as a code: prose in its place is dropped.
func TestAnErrorCodeIsAdmittedOnlyAsACode(t *testing.T) {
	if got := errorInfoReason(strings.NewReader(`{"error":{"details":[{"reason":"key sk-live-abc is bad"}]}}`)); got != "" {
		t.Fatalf("prose was read as a code: %q", got)
	}
}

// A 403 from an endpoint an operator runs says nothing certain about the key, so
// it stays a refusal of the key; only a named vendor's 403 means "permissions".
func TestAnOperatorEndpointsForbiddenStaysARefusedKey(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	}))
	t.Cleanup(srv.Close)
	got := probeProviderKey(context.Background(), boundAt(providerOpenAICompatible, srv.URL), providerOpenAICompatible,
		cloudKeyFor(providerOpenAICompatible, "k"), stubBuilder)
	if got.OK || got.Reason != KeyTestAuthFailed {
		t.Errorf("got %+v, want auth_failed", got)
	}
}
