// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// What each vendor serves, asked of the vendor rather than of a table we keep.
//
// The five wire shapes live in ONE file so a reader can compare them: they
// disagree about the envelope (`data` vs `models`), about the id (Gemini
// namespaces it `models/…`, Ollama tags it `:latest`), and about whether they
// say what a model is FOR at all. Split across five adapter files, the next
// person adding a vendor would have to find four precedents to match.
//
// None of them publishes a price on this endpoint except a broker, and reading
// a broker's would make this read the second answer to a question the
// effective-dated price sheet already owns. So this answers AVAILABILITY only,
// the sheet answers COST, and a model the vendor serves that the sheet cannot
// price is offered and reports UNPRICED.

// modelListLimit caps what one vendor may hand back. Gemini's catalog runs to
// dozens of entries and a broker's to thousands; a picker is unusable past a
// few hundred and an unbounded decode is a memory hole a vendor controls.
const modelListLimit = 500

// getListBody runs one authenticated GET and returns the body's bytes.
//
// Bytes rather than a decode target, so each vendor unmarshals into its OWN
// struct below: the five envelopes agree about nothing, and a shared decoder
// would have to take a caller-shaped destination — which is the same coupling
// spelled less legibly. What is genuinely shared is the request, the status
// rule and the size bound, and those are all that live here.
//
// The `authorize` callback rather than a key argument: the five vendors sign
// this request four different ways, and the alternative is a parameter for each
// scheme that four of the five callers pass empty.
func getListBody(
	ctx context.Context,
	httpc *http.Client,
	vendor, endpoint string,
	authorize func(*http.Request),
) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("ai: %s: build request: %w", vendor, err)
	}
	req.Header.Set("Accept", "application/json")
	authorize(req)
	resp, err := httpc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ai: %s: %w", vendor, err)
	}
	//craft:ignore swallowed-errors best-effort close on a body we have finished with
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		// The status and nothing else. A vendor's error body on this endpoint is
		// frequently HTML from a proxy, and echoing it into a log is how a
		// request — or a key in a redirected URL — ends up in one.
		return nil, fmt.Errorf("ai: %s: listing models: http %d", vendor, resp.StatusCode)
	}
	// Bounded, so a vendor cannot stream an unbounded body into memory on a read
	// nobody is metering.
	raw, err := io.ReadAll(io.LimitReader(resp.Body, listBodyLimit))
	if err != nil {
		return nil, fmt.Errorf("ai: %s: reading model list: %w", vendor, err)
	}
	return raw, nil
}

// listBodyLimit bounds one vendor's list response. A broker's full catalog with
// per-model metadata is comfortably under a megabyte; four is headroom, not an
// invitation.
const listBodyLimit = 4 << 20

// ---- anthropic ----

// ListModels reports the Messages API's own catalog (GET /v1/models), which is
// dated and returned newest first — so the order the vendor gives is the order
// worth showing, and this adds no sort of its own.
func (c *anthropicClient) ListModels(ctx context.Context) ([]model.Info, error) {
	var out struct {
		Data []struct {
			ID          string `json:"id"`
			DisplayName string `json:"display_name"`
		} `json:"data"`
	}
	endpoint := c.baseURL + "/v1/models?limit=" + strconv.Itoa(modelListLimit)
	raw, err := getListBody(ctx, c.http, "anthropic", endpoint, func(r *http.Request) {
		r.Header.Set("x-api-key", c.apiKey)
		r.Header.Set("anthropic-version", anthropicAPIVersion)
	})
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("ai: anthropic: decode model list: %w", err)
	}
	models := make([]model.Info, 0, len(out.Data))
	for _, m := range out.Data {
		// Every model on this endpoint serves the Messages API, so the lane is
		// stated rather than left unknown: Anthropic publishes no embedder here.
		models = append(models, model.Info{
			ID: m.ID, DisplayName: m.DisplayName, Lane: model.LaneChat,
		})
	}
	return models, nil
}

// ---- openai ----

// ListModels reports GET /v1/models. The list is undifferentiated — completion
// models, embedders, transcription and image models arrive together with
// nothing on the row saying which is which — so no lane is claimed. A caller
// that needs the distinction filters on a STATED lane and therefore offers
// these on the chat lanes only.
func (c *openaiClient) ListModels(ctx context.Context) ([]model.Info, error) {
	return openAIWireModels(ctx, c.http, "openai", c.baseURL, c.apiKey)
}

// ---- openai_compatible and vllm ----

// ListModels reports the generic OpenAI-wire catalog, which is what a broker
// (OpenRouter, Together, a gateway) and a local vLLM both publish.
//
// A broker returns per-model PRICES on this endpoint and they are deliberately
// dropped: the price sheet is the effective-dated record this product costs
// calls against, and a second price arriving by a different route is two
// answers to one question that drift the first time either moves.
func (c *openAICompatClient) ListModels(ctx context.Context) ([]model.Info, error) {
	return openAIWireModels(ctx, c.http, "openai-compat", c.baseURL, c.apiKey)
}

// openAIWireModels is the shared GET /v1/models round-trip: the native OpenAI
// adapter, every broker and a local vLLM speak the identical wire, so the
// request and decode live here once — the same reason openAIWireEmbed does.
func openAIWireModels(
	ctx context.Context,
	httpc *http.Client,
	vendor, baseURL, apiKey string,
) ([]model.Info, error) {
	var out struct {
		Data []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"data"`
	}
	raw, err := getListBody(ctx, httpc, vendor, baseURL+"/v1/models", func(r *http.Request) {
		// Empty on a local vLLM, which takes no auth — the same condition the
		// adapter's own post() applies.
		if apiKey != "" {
			r.Header.Set("Authorization", "Bearer "+apiKey)
		}
	})
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("ai: %s: decode model list: %w", vendor, err)
	}
	models := make([]model.Info, 0, len(out.Data))
	for _, m := range out.Data {
		if len(models) == modelListLimit {
			break
		}
		// `name` is a broker's human label and absent from OpenAI's own rows;
		// the caller shows the id where it is empty rather than inventing one.
		models = append(models, model.Info{ID: m.ID, DisplayName: m.Name})
	}
	return models, nil
}

// ---- gemini ----

// ListModels reports GET /v1beta/models, the one vendor list that says what
// each model is FOR: `supportedGenerationMethods` names `embedContent` for an
// embedder and `generateContent` for a chat model, so the embeddings lane can
// be offered real suggestions here where the other vendors leave it to the
// sheet.
//
// Paginated, and the pages are followed: the catalog runs past one page and a
// reader who stopped at the first would be told a current model does not exist.
func (c *geminiClient) ListModels(ctx context.Context) ([]model.Info, error) {
	var models []model.Info
	pageToken := ""
	for {
		var out struct {
			Models []struct {
				Name        string   `json:"name"`
				DisplayName string   `json:"displayName"`                //nolint:tagliatelle // Google's wire format (camelCase)
				Methods     []string `json:"supportedGenerationMethods"` //nolint:tagliatelle // Google's wire format (camelCase)
			} `json:"models"`
			NextPageToken string `json:"nextPageToken"` //nolint:tagliatelle // Google's wire format (camelCase)
		}
		endpoint := c.baseURL + "/models?pageSize=100"
		if pageToken != "" {
			endpoint += "&pageToken=" + url.QueryEscape(pageToken)
		}
		raw, err := getListBody(ctx, c.http, "gemini", endpoint, func(r *http.Request) {
			r.Header.Set("x-goog-api-key", c.apiKey)
		})
		if err != nil {
			return nil, err
		}
		if err := json.Unmarshal(raw, &out); err != nil {
			return nil, fmt.Errorf("ai: gemini: decode model list: %w", err)
		}
		for _, m := range out.Models {
			models = append(models, model.Info{
				// The wire namespaces every id `models/gemini-…`; a binding
				// names the bare id, which is what the adapter puts back when it
				// builds a request path.
				ID:          strings.TrimPrefix(m.Name, "models/"),
				DisplayName: m.DisplayName,
				Lane:        geminiLane(m.Methods),
			})
		}
		// Stopping on the cap as well as on the last page: a vendor that keeps
		// handing back a token must not turn this into an unbounded loop.
		if out.NextPageToken == "" || len(models) >= modelListLimit {
			return models, nil
		}
		pageToken = out.NextPageToken
	}
}

// geminiLane reads what a Gemini model is for off the methods it supports.
//
// An empty answer for anything that is neither: the catalog also carries
// rankers, token counters and tuned-model bases, and calling one of those a
// chat model would offer a binding that cannot serve a call.
func geminiLane(methods []string) string {
	for _, m := range methods {
		switch m {
		case "generateContent":
			return model.LaneChat
		case "embedContent":
			return model.LaneEmbeddings
		}
	}
	return ""
}

// ---- ollama ----

// ListModels reports what is PULLED onto this host (GET /api/tags) rather than
// what Ollama's library offers — a model nobody has pulled cannot serve a call,
// and offering one would bind a lane to a download.
func (c *ollamaClient) ListModels(ctx context.Context) ([]model.Info, error) {
	var out struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	raw, err := getListBody(ctx, c.http, "ollama", c.baseURL+"/api/tags", func(*http.Request) {})
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, fmt.Errorf("ai: ollama: decode model list: %w", err)
	}
	models := make([]model.Info, 0, len(out.Models))
	for _, m := range out.Models {
		models = append(models, model.Info{ID: m.Name})
	}
	return models, nil
}

// ---- fake ----

// ListModels answers the offline fake with a fixed pair, so the surface that
// lists models is exercisable with no vendor, no key and no network — the same
// reason every other method on this client exists.
func (c *FakeClient) ListModels(context.Context) ([]model.Info, error) {
	return []model.Info{
		{ID: "fake-chat", DisplayName: "Fake chat", Lane: model.LaneChat},
		{ID: "fake-embed", DisplayName: "Fake embedder", Lane: model.LaneEmbeddings},
	}, nil
}
