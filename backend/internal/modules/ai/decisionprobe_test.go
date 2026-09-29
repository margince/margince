// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/config"
)

// A decision endpoint is tested at the route its HOST answers, never with a
// billed decision. The hosts are named by URL, so a scripted transport stands
// in for each and records what was asked.

type scriptedHost struct {
	t      *testing.T
	answer func(r *http.Request) (int, string)
	asked  []string
}

func (h *scriptedHost) RoundTrip(r *http.Request) (*http.Response, error) {
	h.asked = append(h.asked, r.Method+" "+r.URL.String()+" auth="+r.Header.Get("Authorization"))
	status, body := h.answer(r)
	return &http.Response{
		StatusCode: status, Body: io.NopCloser(strings.NewReader(body)),
		Header: http.Header{}, Request: r,
	}, nil
}

func (h *scriptedHost) probes() keyProbes {
	return keyProbes{decider: func(lane DecisionsConfig, keys config.Lookup) (*decisionClient, error) {
		return selectDeciderOn(lane, keys, &http.Client{Transport: h})
	}}
}

func decisionBound(provider, endpoint string) RoutingConfig {
	return RoutingConfig{
		Profile:   ProfileCloudFrontier,
		Decisions: &DecisionsConfig{Provider: provider, Model: "m", BaseURL: endpoint},
	}
}

// TypeSafe's own API lists its models behind the key, beside the decision
// route — one call is both the list and the proof.
func TestJevIsTestedByListingTypeSafesModels(t *testing.T) {
	host := &scriptedHost{t: t, answer: func(*http.Request) (int, string) {
		return http.StatusOK, `{"models":[{"name":"jev-latest"},{"name":"jev-preview"}]}`
	}}
	got := probeProviderKey(context.Background(), RoutingConfig{Profile: ProfileCloudFrontier}, providerJev,
		cloudKeyFor(providerJev, "tk"), host.probes())

	if !got.OK || !got.Counted || got.ModelCount != 2 {
		t.Fatalf("a listed TypeSafe key should pass with its count: %+v", got)
	}
	want := "GET https://api.typesafe.ai/v1/models auth=Bearer tk"
	if len(host.asked) != 1 || host.asked[0] != want {
		t.Fatalf("asked %v, want [%s]", host.asked, want)
	}
}

// A proxy mounting TypeSafe's API under a prefix keeps it: the list is the
// endpoint's sibling, not the host root.
func TestJevAtAProxyIsListedBesideItsEndpoint(t *testing.T) {
	host := &scriptedHost{t: t, answer: func(*http.Request) (int, string) { return http.StatusUnauthorized, "" }}
	got := probeProviderKey(context.Background(), decisionBound(providerJev, "https://proxy.example/typesafe/v1/systemone"),
		providerJev, cloudKeyFor(providerJev, "tk"), host.probes())

	if got.OK || got.Reason != KeyTestAuthFailed {
		t.Fatalf("a refused TypeSafe key is auth_failed: %+v", got)
	}
	if !strings.HasPrefix(host.asked[0], "GET https://proxy.example/typesafe/v1/models ") {
		t.Fatalf("asked %v", host.asked)
	}
}

// OpenRouter's catalogue is public, so listing would pass any key; its key
// endpoint is the authenticated read, and it passes with no count.
func TestAnOpenRouterDecisionKeyIsTestedAtItsKeyEndpoint(t *testing.T) {
	for status, want := range map[int]KeyTestReason{
		http.StatusOK: "", http.StatusUnauthorized: KeyTestAuthFailed, http.StatusTooManyRequests: KeyTestRateLimited,
	} {
		host := &scriptedHost{t: t, answer: func(*http.Request) (int, string) { return status, `{"data":{}}` }}
		got := probeProviderKey(context.Background(),
			decisionBound(providerJevCompatible, "https://openrouter.ai/api/alpha/decisions"),
			providerJevCompatible, cloudKeyFor(providerJevCompatible, "or"), host.probes())

		if got.Reason != want || got.OK != (want == "") || got.Counted || got.Unconfirmed {
			t.Errorf("http %d: got %+v, want reason %q, no count, and a confirmed key", status, got, want)
		}
		if host.asked[0] != "GET https://openrouter.ai/api/v1/key auth=Bearer or" {
			t.Errorf("http %d: asked %v", status, host.asked)
		}
	}
}

// Any other Jev-wire server promises the decision route and nothing else. An
// empty body is refused as malformed only once the caller is let in, so a 400
// is a pass — and with no model named, nothing is billed.
func TestAnyOtherDecisionServerIsProbedWithAnEmptyDecision(t *testing.T) {
	for status, want := range map[int]KeyTestReason{
		http.StatusBadRequest:          "",
		http.StatusUnprocessableEntity: "",
		http.StatusOK:                  KeyTestUnreachable,
		http.StatusFound:               KeyTestUnreachable,
		http.StatusUnauthorized:        KeyTestAuthFailed,
		http.StatusForbidden:           KeyTestAuthFailed,
		http.StatusBadGateway:          KeyTestUnreachable,
	} {
		host := &scriptedHost{t: t, answer: func(r *http.Request) (int, string) {
			body, err := io.ReadAll(r.Body)
			if err != nil || string(body) != "{}" {
				t.Errorf("the probe sent %q, want an empty object", body)
			}
			return status, ""
		}}
		got := probeProviderKey(context.Background(),
			decisionBound(providerJevCompatible, "http://decider.internal:8767/v1/systemone"),
			providerJevCompatible, noCloudKeys(), host.probes())

		if got.Reason != want || got.OK != (want == "") || got.Counted || got.OK != got.Unconfirmed {
			t.Errorf("http %d: got %+v, want reason %q, and a pass marked unconfirmed", status, got, want)
		}
		if host.asked[0] != "POST http://decider.internal:8767/v1/systemone auth=" {
			t.Errorf("http %d: asked %v — a keyless server is sent no header", status, host.asked)
		}
	}
}

// The picker's list: TypeSafe's names, OpenRouter's catalogue narrowed to
// TypeSafe's models, and nothing for a server that publishes no list.
func TestDecisionModelsComeFromWhereEachHostPublishesThem(t *testing.T) {
	catalogue := `{"data":[{"id":"typesafe/jev-1.13","name":"Jev 1.13"},{"id":"~typesafe/jev-latest"},{"id":"openai/gpt-5"}]}`
	host := &scriptedHost{t: t, answer: func(*http.Request) (int, string) { return http.StatusOK, catalogue }}
	client, err := selectDeciderOn(DecisionsConfig{Provider: providerJevCompatible, BaseURL: "https://openrouter.ai/api/alpha/decisions"},
		noCloudKeys(), &http.Client{Transport: host})
	if err != nil {
		t.Fatal(err)
	}
	models, listed, err := client.decisionModels(context.Background(), providerJevCompatible)
	if err != nil || !listed || len(models) != 2 || models[0].ID != "typesafe/jev-1.13" || models[0].Lane != "decisions" {
		t.Fatalf("got %+v listed=%v err=%v", models, listed, err)
	}
	if !strings.HasPrefix(host.asked[0], "GET https://openrouter.ai/api/v1/models?output_modalities=all ") {
		t.Fatalf("asked %v — without every modality the catalogue leaves Jev out", host.asked)
	}

	self, err := selectDeciderOn(DecisionsConfig{Provider: providerJevCompatible, BaseURL: "http://decider.internal/v1/systemone"},
		noCloudKeys(), &http.Client{Transport: refuseDial{}})
	if err != nil {
		t.Fatal(err)
	}
	if _, listed, err := self.decisionModels(context.Background(), providerJevCompatible); listed || err != nil {
		t.Fatalf("a self-hosted server publishes no list: listed=%v err=%v", listed, err)
	}
}

// The list sits beside the decision route, whatever shape the stored endpoint
// takes: no path, a trailing slash, or a prefix a proxy mounts it under.
func TestTheModelListIsFoundBesideAnyEndpointShape(t *testing.T) {
	for endpoint, want := range map[string]string{
		"https://api.typesafe.ai/v1/systemone":         "https://api.typesafe.ai/v1/models",
		"https://api.typesafe.ai/v1/systemone/":        "https://api.typesafe.ai/v1/models",
		"https://proxy.example":                        "https://proxy.example/models",
		"https://proxy.example/":                       "https://proxy.example/models",
		"https://proxy.example/typesafe/v1/systemone":  "https://proxy.example/typesafe/v1/models",
		"https://proxy.example/v1/systemone?region=eu": "https://proxy.example/v1/models",
	} {
		got, err := siblingURL(endpoint, "models")
		if err != nil || got != want {
			t.Errorf("siblingURL(%q) = %q, %v; want %q", endpoint, got, err, want)
		}
	}
	if got, err := originURL("https://openrouter.ai/api/alpha/decisions", "/api/v1/key"); err != nil ||
		got != "https://openrouter.ai/api/v1/key" {
		t.Errorf("originURL = %q, %v", got, err)
	}
}

// A `jev` lane pointed at OpenRouter still speaks TypeSafe's API: its key is
// sent to the model list beside its endpoint, never to the broker's key route.
func TestJevOnAnOpenRouterHostIsNotAskedTheBrokersKeyRoute(t *testing.T) {
	host := &scriptedHost{t: t, answer: func(*http.Request) (int, string) { return http.StatusOK, `{"models":[]}` }}
	probeProviderKey(context.Background(), decisionBound(providerJev, "https://openrouter.ai/api/v1/systemone"),
		providerJev, cloudKeyFor(providerJev, "tk"), host.probes())
	if !strings.HasPrefix(host.asked[0], "GET https://openrouter.ai/api/v1/models ") {
		t.Fatalf("asked %v", host.asked)
	}
}

// eu_hosted refuses to bind TypeSafe's own API or OpenRouter's decisions
// endpoint; a test of either says so rather than reporting a key it could
// never use as connected, and dials nothing.
func TestEUHostedRefusesADecisionLaneItWouldNotBind(t *testing.T) {
	for _, tc := range []struct{ provider, endpoint string }{
		{providerJev, ""},
		{providerJevCompatible, "https://openrouter.ai/api/alpha/decisions"},
	} {
		cfg := decisionBound(tc.provider, tc.endpoint)
		cfg.Profile = ProfileEUHosted
		got := probeProviderKey(context.Background(), cfg, tc.provider, cloudKeyFor(tc.provider, "k"), stubBuilder)
		if got.Reason != KeyTestProfileForbids {
			t.Errorf("%s at %q: got %+v, want profile_forbids", tc.provider, tc.endpoint, got)
		}
	}
}

func TestADecisionEndpointThatIsNotAURLIsRefused(t *testing.T) {
	if _, err := siblingURL("not a url", "models"); err == nil {
		t.Fatal("a sibling of a non-URL was built")
	}
	if _, err := originURL("/relative/only", "/api/v1/key"); err == nil {
		t.Fatal("an origin of a relative path was built")
	}
}

// A 200 that is not TypeSafe's list — a proxy answering `{}` — proves no key
// and is not a pass; an empty list that IS the list still is.
func TestAJevAnswerWithNoModelListIsNotAPass(t *testing.T) {
	for body, wantOK := range map[string]bool{`{}`: false, `{"models":[]}`: true} {
		host := &scriptedHost{t: t, answer: func(*http.Request) (int, string) { return http.StatusOK, body }}
		got := probeProviderKey(context.Background(), RoutingConfig{Profile: ProfileCloudFrontier}, providerJev,
			cloudKeyFor(providerJev, "tk"), host.probes())
		if got.OK != wantOK {
			t.Errorf("body %s: got %+v, want ok=%v", body, got, wantOK)
		}
	}
}
