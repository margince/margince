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
// A request with a thinking floor is sent the least thinking that meets it
// instead (floorThink): the lowest level at or above it, else `true`. A request
// that names its own value through Request.ProviderOptions["ollama"].think
// skips all of that — the same seam the gemini and openai adapters read.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// ollamaOptionsNamespace is the ProviderOptions key this adapter reads.
const ollamaOptionsNamespace = "ollama"

var (
	ollamaThinkOff      = json.RawMessage("false")
	ollamaThinkOn       = json.RawMessage("true")
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

// think resolves the `think` value for one call; nil means send none. Without
// a floor it is the cheapest value the model lists; with one, the least
// thinking that meets it (floorThink).
func (c *ollamaClient) think(ctx context.Context, model string, req model.Request) (json.RawMessage, error) {
	override, err := ollamaThinkOverride(req.ProviderOptions)
	if err != nil || override != nil {
		return override, err
	}
	values, err := c.thinks.lookup(ctx, model, c.showThink)
	if err != nil {
		return nil, err
	}
	if req.ThinkingFloor != "" {
		return floorThink(values, req.ThinkingFloor), nil
	}
	return cheapestThink(values), nil
}

// showThink asks the server what this model accepts for `think`: nil when the
// model does not think or the server cannot be asked.
func (c *ollamaClient) showThink(ctx context.Context, model string) ([]json.RawMessage, error) {
	payload, err := json.Marshal(struct {
		Model string `json:"model"`
	}{model})
	if err != nil {
		return nil, fmt.Errorf("ai: ollama: encode show request: %w", err)
	}
	body, err := c.post(ctx, "/api/show", payload)
	if err != nil {
		if showUnavailable(err) {
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
	return shown.Thinking.Values, nil
}

// showUnavailable reports a server that cannot be asked about the model, as
// against one that has /api/show and does not know the model. Ollama answers the
// second with a JSON {"error": …} body. The first is a proxy or an old server: a
// bare 404 or 405/501 for a route it does not have, or a 401/403 from a proxy that
// lets chat through and refuses inspection. Failing every chat call on any of
// them would break a deployment that worked before the field existed; if the
// chat call is refused too, it reports its own error.
func showUnavailable(err error) bool {
	var status *ollamaStatusError
	if !errors.As(err, &status) {
		return false
	}
	switch status.code {
	case http.StatusUnauthorized, http.StatusForbidden, http.StatusMethodNotAllowed, http.StatusNotImplemented:
		return true
	case http.StatusNotFound:
		return !strings.HasPrefix(status.body, `{"error"`)
	}
	return false
}

// cheapestThink picks the value that spends the least on thinking: `false` if
// the model can turn it off, else the lowest level it lists by name, else (a
// level this adapter does not know) the first one listed.
func cheapestThink(values []json.RawMessage) json.RawMessage {
	for _, v := range values {
		if bytes.Equal(v, ollamaThinkOff) {
			return ollamaThinkOff
		}
	}
	if lowest := lowestThinkLevel(values, "minimal"); lowest != nil {
		return lowest
	}
	if len(values) > 0 {
		return values[0]
	}
	return nil
}

// floorThink is the least thinking values offer that meets floor: the lowest
// level at or above it, else `true`. A model listing neither (it does not
// think, or the server does not say) is sent nothing, as before the floor.
func floorThink(values []json.RawMessage, floor string) json.RawMessage {
	if level := lowestThinkLevel(values, floor); level != nil {
		return level
	}
	for _, v := range values {
		if bytes.Equal(v, ollamaThinkOn) {
			return ollamaThinkOn
		}
	}
	return nil
}

// lowestThinkLevel is the shallowest level string in values that meets floor,
// by name: the order of a model's `values` is not part of Ollama's contract.
func lowestThinkLevel(values []json.RawMessage, floor string) json.RawMessage {
	levels := make([]string, len(values))
	for i, v := range values {
		if json.Unmarshal(v, &levels[i]) != nil {
			levels[i] = ""
		}
	}
	lowest := lowestEffortAtLeast(floor, levels)
	if i := slices.Index(levels, lowest); lowest != "" && i >= 0 {
		return values[i]
	}
	return nil
}
