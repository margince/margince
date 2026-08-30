// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The whole chain, over a real HTTP response: a provider answering 429 with a
// spending-cap body must reach the operator as a budget message, not as a
// claim that the model produced something unusable.
func TestAQuotaRefusalReachesTheOperatorAsABudgetMessage(t *testing.T) {
	const capReached = `{"error":{"status":"RESOURCE_EXHAUSTED","message":"Your project has exceeded its monthly spending cap."}}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		if _, writeErr := io.WriteString(w, capReached); writeErr != nil {
			t.Errorf("writing the provider's refusal body: %v", writeErr)
		}
	}))
	defer server.Close()

	resp, err := http.Post(server.URL, "application/json", strings.NewReader("{}"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if closeErr := resp.Body.Close(); closeErr != nil {
			t.Fatalf("closing the response body: %v", closeErr)
		}
	}()

	providerErr := geminiError(resp)
	if !errors.Is(providerErr, ErrProviderQuota) {
		t.Fatalf("a 429 from the provider is a quota refusal: %v", providerErr)
	}
	message := SafeVoiceBuildFailure(fmt.Errorf("voice build model call: %w", providerErr))
	if !strings.Contains(strings.ToLower(message), "out of budget") {
		t.Fatalf("the operator is told the budget ran out: %q", message)
	}
	if strings.Contains(message, "RESOURCE_EXHAUSTED") || strings.Contains(message, "429") {
		t.Fatalf("the provider's own payload never reaches the operator: %q", message)
	}
}
