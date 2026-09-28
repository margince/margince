// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

// model.ErrRequestRejected stops a ladder, so it is a claim that the request is
// malformed as sent, and only a vendor's own error code can make it. A 400 is
// also how every vendor here says "too long for this model", "this model does
// not take that parameter", "that key is not valid" and "not in your region",
// and each of those is a reason to try the next rung, not to stop.

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// refusalFixture is one vendor error as the vendor sends it.
type refusalFixture struct {
	provider string
	status   int
	body     string
	// rejected is whether the vendor's own code says the request is malformed.
	rejected bool
}

// vendorRefusals holds, per wire, the shapes a status alone misreads: context
// length, an unsupported parameter, credentials, account and region, beside
// the one code per vendor that does name a malformed request.
func vendorRefusals() map[string]refusalFixture {
	return map[string]refusalFixture{
		"anthropic/context length": {
			providerAnthropic, http.StatusBadRequest,
			`{"type":"error","error":{"type":"invalid_request_error","message":"prompt is too long: 215000 tokens > 200000 maximum"}}`, false,
		},
		"anthropic/request too large": {
			providerAnthropic, http.StatusRequestEntityTooLarge,
			`{"type":"error","error":{"type":"request_too_large","message":"Request exceeds the maximum allowed number of bytes."}}`, false,
		},
		"anthropic/unsupported parameter": {
			providerAnthropic, http.StatusBadRequest,
			`{"type":"error","error":{"type":"invalid_request_error","message":"\"thinking.type.enabled\" is not supported for this model."}}`, false,
		},
		"anthropic/spend limit": {
			providerAnthropic, http.StatusBadRequest,
			`{"type":"error","error":{"type":"invalid_request_error","message":"You have reached your specified workspace API usage limits."}}`, false,
		},
		"anthropic/credential": {
			providerAnthropic, http.StatusUnauthorized,
			`{"type":"error","error":{"type":"authentication_error","message":"invalid x-api-key"}}`, false,
		},

		"openai/context length": {
			providerOpenAI, http.StatusBadRequest,
			`{"error":{"message":"Your input exceeds the context window of this model.","type":"invalid_request_error","param":"input","code":"context_length_exceeded"}}`, false,
		},
		"openai/unsupported parameter": {
			providerOpenAI, http.StatusBadRequest,
			`{"error":{"message":"Unsupported parameter: 'temperature' is not supported with this model.","type":"invalid_request_error","param":"temperature","code":"unsupported_parameter"}}`, false,
		},
		"openai/credential": {
			providerOpenAI, http.StatusUnauthorized,
			`{"error":{"message":"Incorrect API key provided.","type":"invalid_request_error","param":null,"code":"invalid_api_key"}}`, false,
		},
		"openai/no code": {
			providerOpenAI, http.StatusBadRequest,
			`{"error":{"message":"bad field","type":"invalid_request_error","param":null,"code":null}}`, false,
		},
		"openai/invalid schema": {
			providerOpenAI, http.StatusBadRequest,
			`{"error":{"message":"Invalid schema for response_format 'answer': In context=(), 'additionalProperties' is required to be supplied and to be false.","type":"invalid_request_error","param":"text.format.schema","code":"invalid_json_schema"}}`, true,
		},

		"gemini/context length": {
			providerGemini, http.StatusBadRequest,
			`{"error":{"code":400,"message":"The input token count (1200000) exceeds the maximum number of tokens allowed (1048576).","status":"INVALID_ARGUMENT"}}`, false,
		},
		"gemini/credential": {
			providerGemini, http.StatusBadRequest,
			`{"error":{"code":400,"message":"API key not valid. Please pass a valid API key.","status":"INVALID_ARGUMENT",` +
				`"details":[{"@type":"type.googleapis.com/google.rpc.ErrorInfo","reason":"API_KEY_INVALID","domain":"googleapis.com"}]}}`, false,
		},
		"gemini/region": {
			providerGemini, http.StatusBadRequest,
			`{"error":{"code":400,"message":"User location is not supported for the API use.","status":"FAILED_PRECONDITION"}}`, false,
		},
		"gemini/invalid field": {
			providerGemini, http.StatusBadRequest,
			`{"error":{"code":400,"message":"Invalid JSON payload received. Unknown name \"labels\": Cannot find field.","status":"INVALID_ARGUMENT",` +
				`"details":[{"@type":"type.googleapis.com/google.rpc.BadRequest","fieldViolations":[{"description":"Invalid JSON payload received. Unknown name \"labels\": Cannot find field."}]}]}}`, true,
		},

		"gemini/model-dependent setting": {
			providerGemini, http.StatusBadRequest,
			`{"error":{"code":400,"message":"Invalid value at 'generation_config.thinking_config.thinking_budget' (TYPE_INT32), \"high\"","status":"INVALID_ARGUMENT",` +
				`"details":[{"@type":"type.googleapis.com/google.rpc.BadRequest","fieldViolations":[{"field":"generation_config.thinking_config.thinking_budget","description":"Invalid value at 'generation_config.thinking_config.thinking_budget' (TYPE_INT32), \"high\""}]}]}}`, false,
		},
		"gemini/camel-cased setting": {
			providerGemini, http.StatusBadRequest,
			`{"error":{"code":400,"message":"Invalid value at 'generationConfig.responseMimeType'.","status":"INVALID_ARGUMENT",` +
				`"details":[{"@type":"type.googleapis.com/google.rpc.BadRequest","fieldViolations":[{"field":"generationConfig.responseMimeType","description":"Invalid value."}]}]}}`, false,
		},
		"gemini/malformed content": {
			providerGemini, http.StatusBadRequest,
			`{"error":{"code":400,"message":"Invalid value at 'contents[0].role' (TYPE_STRING), \"assistant\"","status":"INVALID_ARGUMENT",` +
				`"details":[{"@type":"type.googleapis.com/google.rpc.BadRequest","fieldViolations":[{"field":"contents[0].role","description":"Invalid value at 'contents[0].role' (TYPE_STRING), \"assistant\""}]}]}}`, true,
		},

		"openai-compat/context length": {
			providerOpenAICompatible, http.StatusBadRequest,
			`{"error":{"message":"This endpoint's maximum context length is 131072 tokens. However, you requested about 200000 tokens.","code":400}}`, false,
		},
		"openai-compat/credential": {
			providerOpenAICompatible, http.StatusUnauthorized,
			`{"error":{"message":"No auth credentials found","code":401}}`, false,
		},
		"openai-compat/unsupported parameter": {
			providerOpenAICompatible, http.StatusBadRequest,
			`{"error":{"message":"Unsupported parameter: 'temperature'.","type":"invalid_request_error","code":"unsupported_parameter"}}`, false,
		},
		"openai-compat/no upstream host": {
			providerOpenAICompatible, http.StatusNotFound,
			`{"error":{"message":"No allowed providers are available for the selected model.","code":404}}`, false,
		},
		"openai-compat/invalid schema": {
			providerOpenAICompatible, http.StatusBadRequest,
			`{"error":{"message":"Invalid schema for response_format 'answer'.","type":"invalid_request_error","code":"invalid_json_schema"}}`, true,
		},

		"ollama/unsupported parameter": {
			providerOllama, http.StatusBadRequest,
			`{"error":"registry.ollama.ai/library/gemma3:latest does not support tools"}`, false,
		},
		"ollama/context length": {
			providerOllama, http.StatusBadRequest,
			`{"error":"the input length exceeds the context length"}`, false,
		},
		"ollama/credential": {providerOllama, http.StatusUnauthorized, `{"error":"unauthorized"}`, false},
	}
}

// statusServer answers every request with one status and body.
func statusServer(t *testing.T, status int, body string) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if _, err := w.Write([]byte(body)); err != nil {
			t.Errorf("writing fixture reply: %v", err)
		}
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}

func refusingAdapter(t *testing.T, fixture refusalFixture) model.Client {
	t.Helper()
	client, err := selectLocalBrain(ProviderConfig{
		Provider: fixture.provider, BaseURL: statusServer(t, fixture.status, fixture.body), Model: "m",
	}, allCloudKeys())
	if err != nil {
		t.Fatalf("building the %s adapter: %v", fixture.provider, err)
	}
	return client
}

func TestOnlyAVendorsMalformedCodeRejectsTheRequest(t *testing.T) {
	for name, fixture := range vendorRefusals() {
		t.Run(name, func(t *testing.T) {
			_, err := refusingAdapter(t, fixture).Complete(context.Background(), model.Request{
				Messages: []model.Message{{Role: "user", Content: "q"}},
			})
			if err == nil {
				t.Fatal("an HTTP error status was accepted as an answer")
			}
			if got := errors.Is(err, model.ErrRequestRejected); got != fixture.rejected {
				t.Errorf("errors.Is(err, model.ErrRequestRejected) = %v, want %v: %v", got, fixture.rejected, err)
			}
		})
	}
}

// A status says nothing about which of those a 4xx was, so on its own it never
// rejects: a bare 400, 413 or 422 walks exactly as any other provider failure.
func TestNoAdapterReadsAStatusAloneAsARejectedRequest(t *testing.T) {
	for _, provider := range []string{providerAnthropic, providerOpenAI, providerGemini, providerOpenAICompatible, providerOllama} {
		for _, status := range []int{http.StatusBadRequest, http.StatusRequestEntityTooLarge, http.StatusUnprocessableEntity} {
			t.Run(fmt.Sprintf("%s/%d", provider, status), func(t *testing.T) {
				_, err := refusingAdapter(t, refusalFixture{provider: provider, status: status, body: `{}`}).Complete(
					context.Background(), model.Request{Messages: []model.Message{{Role: "user", Content: "q"}}},
				)
				if err == nil || errors.Is(err, model.ErrRequestRejected) {
					t.Errorf("an unclassified %d read as %v, want a provider failure that walks", status, err)
				}
			})
		}
	}
}

// What the classification is for, over the real wire: every excluded shape
// walks to the next rung even when that rung is the SAME provider and model,
// the one binding where a real rejection stops.
func TestAnExcludedRefusalWalksTheLadder(t *testing.T) {
	for name, fixture := range vendorRefusals() {
		t.Run(name, func(t *testing.T) {
			next := NewFakeClient().Script("served")
			same := routeMeta{provider: fixture.provider, model: "m"}
			r := assembleRouter(map[Tier]model.Client{TierCheapCloud: refusingAdapter(t, fixture), TierPremium: next},
				NewFakeClient(), ProfileEUHosted, &memMeter{}, DefaultMonthlyTokens, nil,
				map[Tier]routeMeta{TierCheapCloud: same, TierPremium: same}, false, nil)
			resp, _, err := r.Complete(wsContext(t), TaskColdStart, model.Request{Messages: []model.Message{{Role: "user", Content: "q"}}})
			if fixture.rejected {
				if !errors.Is(err, model.ErrRequestRejected) || len(next.Calls()) != 0 {
					t.Errorf("a malformed request was re-sent to the binding that refused it: err=%v, calls=%d", err, len(next.Calls()))
				}
				return
			}
			if err != nil || resp.Text != "served" {
				t.Errorf("the walk stopped at a refusal the next rung may not share: resp=%q err=%v", resp.Text, err)
			}
		})
	}
}

// A rejected request stops the walk only when the next bound rung is the same
// provider and model, because only then is the identical request re-sent to
// the API that refused it; a withheld answer always walks. Neither ends as
// ErrAllTiersFailed: that sentinel says no model was reached, and a caller
// re-drives on it as an outage.
func TestTheLadderTreatsAnOutcomeAsAnOutcome(t *testing.T) {
	rejected := fmt.Errorf("%w: bad field", model.ErrRequestRejected)
	for name, tc := range map[string]struct {
		cause     error
		above     routeMeta
		want      error
		wantCalls int
	}{
		"withheld":                      {withheldError{wire: "fake", reason: "refusal"}, routeMeta{provider: "fake", model: "m"}, model.ErrOutputWithheld, 2},
		"rejected under the same model": {rejected, routeMeta{provider: "fake", model: "m"}, model.ErrRequestRejected, 1},
		"rejected under another model":  {rejected, routeMeta{provider: "fake", model: "other"}, model.ErrRequestRejected, 2},
	} {
		t.Run(name, func(t *testing.T) {
			fake := NewFakeClient().ScriptSteps(FakeStep{Err: tc.cause}, FakeStep{Err: tc.cause})
			r := assembleRouter(map[Tier]model.Client{TierCheapCloud: fake, TierPremium: fake},
				NewFakeClient(), ProfileEUHosted, &memMeter{}, DefaultMonthlyTokens, nil,
				map[Tier]routeMeta{TierCheapCloud: {provider: "fake", model: "m"}, TierPremium: tc.above}, false, nil)
			_, _, err := r.Complete(wsContext(t), TaskColdStart, model.Request{Messages: []model.Message{{Role: "user", Content: "q"}}})
			if !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
			if errors.Is(err, ErrAllTiersFailed) {
				t.Errorf("an outcome was reported as every tier failing, which the cert lane re-drives as an outage: %v", err)
			}
			if got := len(fake.Calls()); got != tc.wantCalls {
				t.Errorf("the ladder made %d call(s), want %d", got, tc.wantCalls)
			}
		})
	}
}

// The same provider and model behind another base URL is another API, which may
// accept what this one refused, so a rejection there walks rather than stops.
func TestTwoEndpointsOfOneModelAreTwoBindings(t *testing.T) {
	meta := embedInclusiveMeta(RoutingConfig{Tiers: map[Tier]ProviderConfig{
		TierCheapCloud: {Provider: providerOpenAICompatible, Model: "m", BaseURL: "https://one.example"},
		TierPremium:    {Provider: providerOpenAICompatible, Model: "m", BaseURL: "https://two.example"},
	}})
	b := &binding{routeMeta: meta}
	if rejectedAgainAbove(b, rejectedRequest(errors.New("bad schema")), []Tier{TierCheapCloud, TierPremium}) {
		t.Error("a rejection at one base URL stopped the walk before another base URL was asked")
	}
}
