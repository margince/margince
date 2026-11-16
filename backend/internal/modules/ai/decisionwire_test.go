// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/ports/decision"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// The fixtures: decision_jev_response.json is one real answer from
// OpenRouter's decisions endpoint, captured 2026-09-25 with only its id
// replaced. decision_selfhosted_response.json is CONSTRUCTED, shaped from the
// 2026-09-24 observation of a self-hosted server's (Laya's) /v1/systemone: Jev's fields plus
// answer_confidence, action and routing, output_tokens 0 and no cost.
// decision_gemini_response.json is CONSTRUCTED from Google's documented shape.

var triageQuestion = decision.Request{
	Model: "typesafe/jev-1.13",
	State: json.RawMessage(`{"page":{"url":"https://example.org","text":"Example Domain."}}`),
	Questions: map[string]decision.Question{"kind": {
		Type: decision.Choice, Instructions: "What is this website?",
		Criteria: map[string]string{"company": "A business.", "parked": "A placeholder page."},
	}},
}

// recordedDecision is what the test server saw of one request.
type recordedDecision struct {
	path, auth, googleKey string
	body                  map[string]json.RawMessage
}

func decisionServer(t *testing.T, status int, body []byte) (*httptest.Server, *recordedDecision) {
	t.Helper()
	seen := &recordedDecision{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen.path, seen.auth, seen.googleKey = r.URL.Path, r.Header.Get("Authorization"), r.Header.Get("x-goog-api-key")
		raw, err := io.ReadAll(r.Body)
		if err != nil || json.Unmarshal(raw, &seen.body) != nil {
			t.Errorf("the request body is not JSON: %s (%v)", raw, err)
		}
		w.WriteHeader(status)
		if _, err := w.Write(body); err != nil {
			t.Errorf("write: %v", err)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, seen
}

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestTheDecisionWireDecodesJevAndPostsItsShape(t *testing.T) {
	srv, seen := decisionServer(t, http.StatusOK, fixture(t, "decision_jev_response.json"))
	lane := DecisionsConfig{Provider: providerJevCompatible, Model: "typesafe/jev-1.13", BaseURL: srv.URL + "/api/alpha/decisions"}
	client, err := selectDeciderOn(lane, cloudKeyFor(providerJevCompatible, "or-key"), srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	got, err := client.Decide(context.Background(), triageQuestion)
	if err != nil {
		t.Fatal(err)
	}
	kind := got.Answers["kind"]
	if kind.Choice != "parked" || kind.Confidence != 1 || kind.Probabilities["parked"] != 1 {
		t.Errorf("answer = %+v", kind)
	}
	if got.InputTokens != 425 || got.ServedModel != "typesafe/jev-1.13-20260917" || got.ServedProvider != "TypeSafe" {
		t.Errorf("response = %+v", got)
	}
	if seen.path != "/api/alpha/decisions" || seen.auth != "Bearer or-key" {
		t.Errorf("posted to %q with Authorization %q", seen.path, seen.auth)
	}
	var questions map[string]decisionWireQuestion
	if err := json.Unmarshal(seen.body["questions"], &questions); err != nil {
		t.Fatal(err)
	}
	if q := questions["kind"]; q.Type != "choice" || q.Instructions == "" || q.Criteria["parked"] == "" {
		t.Errorf("question on the wire = %+v", q)
	}
	if string(seen.body["model"]) != `"typesafe/jev-1.13"` || !strings.Contains(string(seen.body["state"]), "Example Domain.") {
		t.Errorf("body = %s / %s", seen.body["model"], seen.body["state"])
	}
}

// A self-hosted server holds no key of this installation's, so with none
// sealed the client calls without one — and the endpoint is the one written,
// with nothing appended.
func TestTheDecisionWireDecodesASelfHostedServerAndSendsNoKeyItDoesNotHold(t *testing.T) {
	srv, seen := decisionServer(t, http.StatusOK, fixture(t, "decision_selfhosted_response.json"))
	lane := DecisionsConfig{Provider: providerJevCompatible, Model: "typed-decisions", BaseURL: srv.URL + "/v1/systemone"}
	client, err := selectDeciderOn(lane, noCloudKeys(), srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	got, err := client.Decide(context.Background(), triageQuestion)
	if err != nil {
		t.Fatal(err)
	}
	if got.Answers["kind"].Choice != "company" || got.InputTokens != 212 || got.ServedModel != "typed-decisions" || got.ServedProvider != "" {
		t.Errorf("response = %+v", got)
	}
	if seen.path != "/v1/systemone" || seen.auth != "" {
		t.Errorf("posted to %q with Authorization %q; a keyless adapter sends none", seen.path, seen.auth)
	}
}

// Google's dialect: the state goes as `input` text and the key in Google's
// header. Its answer names no model, so the reply leaves the served model
// empty and the trace grades it as the configured one.
func TestTheDecisionWireSpeaksGooglesDialect(t *testing.T) {
	srv, seen := decisionServer(t, http.StatusOK, fixture(t, "decision_gemini_response.json"))
	lane := DecisionsConfig{Provider: providerGeminiDecision, Model: geminiDecisionModel, BaseURL: srv.URL + "/v1beta/decisions"}
	client, err := selectDeciderOn(lane, cloudKeyFor(providerGeminiDecision, "g-key"), srv.Client())
	if err != nil {
		t.Fatal(err)
	}
	ask := triageQuestion
	ask.Model = geminiDecisionModel
	got, err := client.Decide(context.Background(), ask)
	if err != nil {
		t.Fatal(err)
	}
	if kind := got.Answers["kind"]; kind.Choice != "company" || kind.Confidence != 0.9945 || kind.Probabilities["parked"] != 0.0027 {
		t.Errorf("answer = %+v", kind)
	}
	if got.InputTokens != 163 || got.ServedModel != "" || got.ServedProvider != "" {
		t.Errorf("response = %+v, want 163 input tokens and no served identity", got)
	}
	if served, source := servedIdentity(providerGeminiDecision, geminiDecisionModel, got.ServedModel); served != geminiDecisionModel ||
		source != servedIdentitySourceConfigured {
		t.Errorf("served identity = %q (%s), want the configured model", served, source)
	}
	if seen.path != "/v1beta/decisions" || seen.googleKey != "g-key" || seen.auth != "" {
		t.Errorf("posted to %q with x-goog-api-key %q and Authorization %q", seen.path, seen.googleKey, seen.auth)
	}
	var input string
	if err := json.Unmarshal(seen.body["input"], &input); err != nil || input != string(triageQuestion.State) {
		t.Errorf("input = %s (%v), want the state as text", seen.body["input"], err)
	}
	if _, sent := seen.body["state"]; sent || string(seen.body["model"]) != `"gemini-decision-1"` || seen.body["questions"] == nil {
		t.Errorf("body = %v, want model, input and questions only", seen.body)
	}
}

func TestTheDecisionWireClassifiesEachRefusal(t *testing.T) {
	const planted = "sk-or-planted"
	cases := []struct {
		status            int
		body              string
		refused, rejected bool
		wantInMessage     string
	}{
		{http.StatusTooManyRequests, `{"error":{"message":"rate limit exceeded"}}`, true, false, "rate limit exceeded"},
		{529, `<html>overloaded ` + planted + `</html>`, false, false, "refused the call"},
		{http.StatusServiceUnavailable, `{"message":"down"}`, false, false, "down"},
		{http.StatusInternalServerError, `{"error":{"message":"boom"},"echo":"` + planted + `"}`, false, false, "boom"},
		{http.StatusUnauthorized, `{"error":{"message":"no auth"}}`, false, false, "no auth"},
		{http.StatusBadRequest, `{"error":{"message":"criteria must be an object"},"request":"` + planted + `"}`, false, true, "criteria must be"},
		{http.StatusUnprocessableEntity, `not json ` + planted, false, true, "refused the call"},
	}
	for _, tc := range cases {
		srv, _ := decisionServer(t, tc.status, []byte(tc.body))
		client, err := selectDeciderOn(DecisionsConfig{Provider: providerJevCompatible, Model: "m", BaseURL: srv.URL}, nil, srv.Client())
		if err != nil {
			t.Fatal(err)
		}
		_, err = client.Decide(context.Background(), triageQuestion)
		switch {
		case err == nil:
			t.Fatalf("http %d: no error", tc.status)
		case errors.Is(err, errProviderRefused) != tc.refused:
			t.Errorf("http %d: refused = %v, want %v (%v)", tc.status, !tc.refused, tc.refused, err)
		case errors.Is(err, model.ErrRequestRejected) != tc.rejected || errors.Is(err, errDecisionRejected) != tc.rejected:
			t.Errorf("http %d: rejected = %v, want %v (%v)", tc.status, !tc.rejected, tc.rejected, err)
		case strings.Contains(err.Error(), planted):
			t.Errorf("http %d: the raw body reached the message: %v", tc.status, err)
		case !strings.Contains(err.Error(), tc.wantInMessage):
			t.Errorf("http %d: %v, want it to say %q", tc.status, err, tc.wantInMessage)
		}
	}
}

func TestSelectDeciderNamesTheMissingKey(t *testing.T) {
	_, err := selectDecider(DecisionsConfig{Provider: providerJev, Model: "jev-1.13.0"}, noCloudKeys())
	if !errors.Is(err, errNoProviderKey) || !strings.Contains(err.Error(), "TYPESAFE_API_KEY") {
		t.Fatalf("err = %v, want the missing TYPESAFE_API_KEY named", err)
	}
	if _, err := selectDecider(DecisionsConfig{Provider: providerJevCompatible, Model: "m"}, noCloudKeys()); !errors.Is(err, errNoBaseURL) {
		t.Fatalf("a jev_compatible lane with no endpoint: err = %v, want errNoBaseURL", err)
	}
	if _, err := selectDecider(DecisionsConfig{Provider: providerOllama, Model: "m"}, noCloudKeys()); err == nil {
		t.Fatal("a chat provider built a decision client")
	}
	unbound, err := RoutingConfig{}.buildDecisionLane()
	if unbound != nil || err != nil {
		t.Fatalf("an unbound config built a lane: %v, %v", unbound, err)
	}
}

// The production constructor carries the lane's egress guard: the official
// lane sends this installation's TypeSafe key, so it may not be pointed at this
// host, while a self-hosted jev_compatible server must still reach loopback.
func TestSelectDeciderWiresTheEgressGuard(t *testing.T) {
	srv, _ := decisionServer(t, http.StatusOK, fixture(t, "decision_selfhosted_response.json"))
	official, err := selectDecider(DecisionsConfig{Provider: providerJev, Model: "m", BaseURL: srv.URL},
		cloudKeyFor(providerJev, "k"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := official.Decide(context.Background(), triageQuestion); err == nil || !strings.Contains(err.Error(), "netguard") {
		t.Fatalf("the official lane dialed loopback, or refused for another reason: %v", err)
	}
	selfHosted, err := selectDecider(DecisionsConfig{Provider: providerJevCompatible, Model: "m", BaseURL: srv.URL}, noCloudKeys())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := selfHosted.Decide(context.Background(), triageQuestion); err != nil {
		t.Fatalf("the self-hosted lane could not reach its own loopback endpoint: %v", err)
	}
}

// With no base_url the official lane posts to TypeSafe's own endpoint.
func TestTheOfficialLaneDefaultsToTypeSafesEndpoint(t *testing.T) {
	client, err := selectDeciderOn(DecisionsConfig{Provider: providerJev, Model: "jev-1.13.0"}, cloudKeyFor(providerJev, "ts-key"), nil)
	if err != nil {
		t.Fatal(err)
	}
	if client.url != "https://api.typesafe.ai/v1/systemone" || client.apiKey != "ts-key" {
		t.Errorf("url = %q, key = %q", client.url, client.apiKey)
	}
}

// The profile is the only refusal made before a list is asked: a decision
// adapter is listed at its host (decisionprobe.go) like any other, and a local
// one is not refused under sovereign as if it were cloud.
func TestOnlyTheProfileRefusesAListBeforeItIsAsked(t *testing.T) {
	cases := []struct {
		profile  Profile
		provider string
		want     ModelAvailability
	}{
		{ProfileCloudFrontier, providerJev, AvailabilityOK},
		{ProfileSovereign, providerJev, AvailabilityProfileForbids},
		{ProfileSovereign, providerJevCompatible, AvailabilityOK},
		{ProfileSovereign, providerOllama, AvailabilityOK},
		{ProfileSovereign, providerGemini, AvailabilityProfileForbids},
		{ProfileCloudFrontier, providerGemini, AvailabilityOK},
	}
	for _, tc := range cases {
		if got := listRefusal(tc.profile, tc.provider); got != tc.want {
			t.Errorf("listRefusal(%s, %s) = %q, want %q", tc.profile, tc.provider, got, tc.want)
		}
	}
}
