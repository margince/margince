// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

// A broker that finds no upstream host for a request says so with a 404 and a
// sentence of its own. The bodies below are OpenRouter's, as it sends them.

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/shared/ports/model"
)

func TestABrokerWithNoHostForTheRequestIsNamed(t *testing.T) {
	for name, tc := range map[string]struct {
		status int
		body   string
		noHost bool
	}{
		"data policy": {
			http.StatusNotFound,
			`{"error":{"message":"No endpoints found matching your data policy (Free model publication). Configure: https://openrouter.ai/settings/privacy","code":404}}`, true,
		},
		"provider allowlist": {
			http.StatusNotFound,
			`{"error":{"message":"No allowed providers are available for the selected model.","code":404}}`, true,
		},
		"unsupported parameters": {
			http.StatusNotFound,
			`{"error":{"message":"No endpoints found that support tool use. To learn more about provider routing, visit: https://openrouter.ai/docs/provider-routing","code":404}}`, true,
		},
		"guardrail": {
			http.StatusNotFound,
			`{"error":{"message":"No endpoints available matching your guardrail restrictions and data policy.","code":404}}`, true,
		},
		"unknown model": {
			http.StatusNotFound,
			`{"error":{"message":"z-ai/glm-9 is not a valid model ID","code":404}}`, false,
		},
		"the same words on an upstream failure": {
			http.StatusBadGateway,
			`{"error":{"message":"Provider returned error","code":502,"metadata":{"raw":"No endpoints found","provider_name":"AtlasCloud"}}}`, false,
		},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := refusingAdapter(t, refusalFixture{provider: providerOpenAICompatible, status: tc.status, body: tc.body}).Complete(
				context.Background(), model.Request{Messages: []model.Message{{Role: "user", Content: "q"}}},
			)
			if err == nil {
				t.Fatal("an HTTP error status was accepted as an answer")
			}
			if got := errors.Is(err, ErrNoUpstreamHost); got != tc.noHost {
				t.Errorf("errors.Is(err, ErrNoUpstreamHost) = %v, want %v: %v", got, tc.noHost, err)
			}
			if errors.Is(err, model.ErrRequestRejected) {
				t.Errorf("a broker with no host read as a malformed request, which stops the ladder another rung may pass: %v", err)
			}
		})
	}
}
