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

// The two fixtures: decision_jev_response.json is one real answer from
// OpenRouter's decisions endpoint, captured 2026-09-25 with only its id
// replaced. decision_laya_response.json is CONSTRUCTED, shaped from the
// 2026-09-24 observation of Laya's /v1/systemone: Jev's fields plus
// answer_confidence, action and routing, output_tokens 0 and no cost.

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
	path, auth string
	body       map[string]json.RawMessage
}

func decisionServer(t *testing.T, status int, body []byte) (*httptest.Server, *recordedDecision) {
	t.Helper()
	seen := &recordedDecision{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen.path, seen.auth = r.URL.Path, r.Header.Get("Authorization")
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
	lane := DecisionsConfig{Provider: providerOpenRouterDecision, Model: "typesafe/jev-1.13", BaseURL: srv.URL + "/api/"}
	client, err := selectDeciderOn(lane, cloudKeyFor(providerOpenAICompatible, "or-key"), srv.Client())
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

func TestTheDecisionWireDecodesLayaAndSendsNoKey(t *testing.T) {
	srv, seen := decisionServer(t, http.StatusOK, fixture(t, "decision_laya_response.json"))
	lane := DecisionsConfig{Provider: providerLaya, Model: "typed-decisions", BaseURL: srv.URL}
	client, err := selectDeciderOn(lane, allCloudKeys(), srv.Client())
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
		client, err := selectDeciderOn(DecisionsConfig{Provider: providerLaya, Model: "m", BaseURL: srv.URL}, nil, srv.Client())
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
	lane := DecisionsConfig{Provider: providerOpenRouterDecision, Model: "typesafe/jev-1.13", BaseURL: "https://openrouter.ai/api"}
	_, err := selectDecider(lane, noCloudKeys())
	if !errors.Is(err, errNoProviderKey) || !strings.Contains(err.Error(), "OPENAI_COMPATIBLE_API_KEY") {
		t.Fatalf("err = %v, want the missing OPENAI_COMPATIBLE_API_KEY named", err)
	}
	if _, err := selectDecider(DecisionsConfig{Provider: providerOllama, Model: "m"}, noCloudKeys()); err == nil {
		t.Fatal("a chat provider built a decision client")
	}
	unbound, err := RoutingConfig{}.buildDecisionLane()
	if unbound != nil || err != nil {
		t.Fatalf("an unbound config built a lane: %v, %v", unbound, err)
	}
}

// The production constructor carries the lane's egress guard: the Jev lane
// sends this installation's OpenRouter key, so it may not be pointed at this
// host, while a same-host Laya must still reach loopback.
func TestSelectDeciderWiresTheEgressGuard(t *testing.T) {
	srv, _ := decisionServer(t, http.StatusOK, fixture(t, "decision_laya_response.json"))
	jev, err := selectDecider(DecisionsConfig{Provider: providerOpenRouterDecision, Model: "m", BaseURL: srv.URL},
		cloudKeyFor(providerOpenAICompatible, "k"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := jev.Decide(context.Background(), triageQuestion); err == nil || !strings.Contains(err.Error(), "netguard") {
		t.Fatalf("the Jev lane dialed loopback, or refused for another reason: %v", err)
	}
	laya, err := selectDecider(DecisionsConfig{Provider: providerLaya, Model: "m", BaseURL: srv.URL}, noCloudKeys())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := laya.Decide(context.Background(), triageQuestion); err != nil {
		t.Fatalf("the Laya lane could not reach its own loopback endpoint: %v", err)
	}
}
