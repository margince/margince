// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"context"
	"net/http"
)

// geminiTransport is what differs between the hosts that speak the Gemini
// wire: where a model is addressed and how a request is authorised. The body,
// the stream and the error shapes are the wire's own and are shared.
type geminiTransport interface {
	// modelURL is the absolute URL of one verb (generateContent, embedContent,
	// …) on one model, given its bare id.
	modelURL(model, verb string) string
	// modelsURL is the absolute URL of the model collection this host lists.
	modelsURL() string
	authorize(ctx context.Context, r *http.Request) error
}

// aiStudioTransport is Google AI Studio: a version-prefixed base URL
// (defaultGeminiBaseURL, or a proxy override) and an API key header.
type aiStudioTransport struct {
	baseURL string
	apiKey  string
}

func (t aiStudioTransport) modelURL(model, verb string) string {
	return t.modelsURL() + "/" + model + ":" + verb
}

func (t aiStudioTransport) modelsURL() string { return t.baseURL + "/models" }

func (t aiStudioTransport) authorize(_ context.Context, r *http.Request) error {
	r.Header.Set("x-goog-api-key", t.apiKey)
	return nil
}
