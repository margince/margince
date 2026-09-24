// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

// Whether an Ollama model thinks before it answers.
//
// /api/chat takes a `think` field, and its absence means "the model's own
// default". For a model that reasons that default is ON, and what it costs is
// not visible in the reply: the thinking is generated first, counts against
// num_predict, and arrives in `message.thinking`, a field this adapter does not
// read. A schema-constrained answer of thirty tokens therefore takes several
// hundred tokens of wall time, and a small output budget is spent on thinking
// before the answer starts, which comes back as an empty content field with
// done_reason "length".
//
// Measured on gemma4:12b with a 1,024-token budget: thinking on ran the whole
// budget and returned nothing (done_reason "length", 90s); thinking off answered
// in 22 tokens.
//
// A blanket `think: false` is not the answer either, because what a model
// accepts differs. Gemma 4 and Qwen3 take a boolean. gpt-oss takes only a level
// ("low", "medium", "high"): given `false` together with a `format` schema it
// answers with EMPTY content and done_reason "stop", worse than sending nothing.
// Ollama reports each model's choices in /api/show, so this adapter asks once
// per model and sends the cheapest thing that model accepts:
//
//   - `false` when the model can turn thinking off;
//   - otherwise the first (lowest) level it lists;
//   - nothing when the model does not think, when the server does not list the
//     choices, or when it has no /api/show at all (a proxy in front of Ollama):
//     that is what this adapter sent before the field existed, and a guess would
//     be a coin toss between the two ways a model can answer wrongly.
//
// A request that names its own value through
// Request.ProviderOptions["ollama"].think skips all of that — the same seam the
// gemini and openai adapters read, and like theirs it is set by no caller today.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
)

// ollamaOptionsNamespace is the ProviderOptions key this adapter reads.
const ollamaOptionsNamespace = "ollama"

var (
	ollamaThinkOff      = json.RawMessage("false")
	errOllamaThinkShape = errors.New("think must be a boolean or an effort-level string")
)

// ollamaProviderOptions is the vendor-only knob namespace read from
// Request.ProviderOptions["ollama"].
type ollamaProviderOptions struct {
	// Think is a boolean, or the effort level a model that grades its thinking
	// defines. It is forwarded as given: which levels a model accepts is the
	// server's to say, and a list copied here would go stale the first time one
	// is added.
	Think json.RawMessage `json:"think"`
}

// ollamaThinkOverride returns the value a request names for itself, if any.
func ollamaThinkOverride(opts map[string]json.RawMessage) (json.RawMessage, error) {
	raw, ok := opts[ollamaOptionsNamespace]
	if !ok {
		return nil, nil
	}
	var parsed ollamaProviderOptions
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return nil, fmt.Errorf("ai: ollama: provider options: %w", err)
	}
	if len(parsed.Think) == 0 {
		return nil, nil
	}
	var flag bool
	var level string
	if json.Unmarshal(parsed.Think, &flag) != nil && json.Unmarshal(parsed.Think, &level) != nil {
		return nil, fmt.Errorf("ai: ollama: provider options: %w", errOllamaThinkShape)
	}
	return parsed.Think, nil
}

// ollamaThinkCache remembers what /api/show said about each model, so the
// question costs one request per model per client rather than one per call. Its
// lifetime is the client's, which is the binding's: a routing rebind builds a new
// client, so re-pulling a tag under the same binding is noticed at the next
// rebind or restart. The zero value is ready to use.
type ollamaThinkCache struct {
	mu      sync.Mutex
	byModel map[string]json.RawMessage
}

// think resolves the `think` value for one call; nil means send none.
//
// The lock guards the map alone. A lookup is an HTTP call, and holding the lock
// across it would make every call on the client wait for one model's answer,
// including calls for models already resolved. Two concurrent first calls may
// both ask; the answers are equal, so the second write is harmless.
func (c *ollamaClient) think(ctx context.Context, model string, opts map[string]json.RawMessage) (json.RawMessage, error) {
	override, err := ollamaThinkOverride(opts)
	if err != nil || override != nil {
		return override, err
	}
	c.thinks.mu.Lock()
	cached, ok := c.thinks.byModel[model]
	c.thinks.mu.Unlock()
	if ok {
		return cached, nil
	}
	resolved, err := c.showThink(ctx, model)
	if err != nil {
		return nil, err
	}
	c.thinks.mu.Lock()
	defer c.thinks.mu.Unlock()
	if c.thinks.byModel == nil {
		c.thinks.byModel = map[string]json.RawMessage{}
	}
	c.thinks.byModel[model] = resolved
	return resolved, nil
}

// showThink asks the server what this model accepts for `think`.
func (c *ollamaClient) showThink(ctx context.Context, model string) (json.RawMessage, error) {
	payload, err := json.Marshal(struct {
		Model string `json:"model"`
	}{model})
	if err != nil {
		return nil, fmt.Errorf("ai: ollama: encode show request: %w", err)
	}
	body, err := c.post(ctx, "/api/show", payload)
	if err != nil {
		if showEndpointMissing(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("ai: ollama: describing model %q: %w", model, err)
	}
	//craft:ignore swallowed-errors best-effort close of a response body already read to completion — the decode result decides the outcome
	defer func() { _ = body.Close() }()
	var shown struct {
		Thinking struct {
			Values []json.RawMessage `json:"values"`
		} `json:"thinking"`
	}
	if err := json.NewDecoder(body).Decode(&shown); err != nil {
		return nil, fmt.Errorf("ai: ollama: decode show response: %w", err)
	}
	return cheapestThink(shown.Thinking.Values), nil
}

// showEndpointMissing reports a server that has no /api/show, as against one
// that has it and does not know the model. Ollama answers the second with a JSON
// {"error": …} body; a proxy's or an old server's missing route is a bare
// 404/405/501, and failing every chat call on it would break a deployment that
// worked before the field existed.
func showEndpointMissing(err error) bool {
	var status *ollamaStatusError
	if !errors.As(err, &status) {
		return false
	}
	switch status.code {
	case http.StatusMethodNotAllowed, http.StatusNotImplemented:
		return true
	case http.StatusNotFound:
		return !strings.HasPrefix(status.body, `{"error"`)
	}
	return false
}

// cheapestThink picks the value that spends the least on thinking. Ollama lists
// levels lowest first (gpt-oss: low, medium, high), so the first is the cheapest.
func cheapestThink(values []json.RawMessage) json.RawMessage {
	for _, v := range values {
		if bytes.Equal(v, ollamaThinkOff) {
			return ollamaThinkOff
		}
	}
	if len(values) > 0 {
		return values[0]
	}
	return nil
}
