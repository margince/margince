// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

// A thinking floor on the OpenRouter broker, as its `reasoning` block. The
// broker's model list says, per model, whether reasoning is on by default and
// which efforts it takes, so the floor is mapped from that and never guessed:
//
//   - on by default at an effort that meets the floor: nothing is sent;
//   - otherwise the lowest listed effort that meets it;
//   - off, with no effort that meets it: `enabled: true`;
//   - a model listing no reasoning, or a list that cannot be read: nothing.
//
// A binding's own `routing.reasoning_effort` outranks the floor: the operator
// chose it. `exclude` is never sent, so reasoning stays out of the answer.

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

// openRouterModelsTimeout bounds the model-list read, which a chat call waits
// on the first time it meets a model; the call proceeds without a floor past it.
const openRouterModelsTimeout = 10 * time.Second

// openRouterReasoning is one model's `reasoning` object in GET /api/v1/models.
type openRouterReasoning struct {
	// listed is false for a model the list does not describe as reasoning.
	listed           bool
	Mandatory        bool     `json:"mandatory"`
	SupportedEfforts []string `json:"supported_efforts"`
	DefaultEffort    string   `json:"default_effort"`
	// DefaultEnabled is absent on a model that reasons unless told not to.
	DefaultEnabled *bool `json:"default_enabled"`
}

// openRouterReasoningFacts is the metadata cache a binding on baseURL maps its
// floors through, nil for a binding that is not the broker.
func openRouterReasoningFacts(baseURL string) *perModelFacts[openRouterReasoning] {
	if !IsOpenRouterHost(baseURL) {
		return nil
	}
	return &perModelFacts[openRouterReasoning]{}
}

// openRouterReasoningFor is the `reasoning` block floor sends to a model
// described by meta, nil for none.
func openRouterReasoningFor(meta openRouterReasoning, floor string) *openAICompatReasoningWire {
	if !meta.listed || floor == "" {
		return nil
	}
	on := meta.Mandatory || meta.DefaultEnabled == nil || *meta.DefaultEnabled
	if on && (meta.DefaultEffort == "" || effortAtLeast(meta.DefaultEffort, floor)) {
		return nil
	}
	if effort := lowestEffortAtLeast(floor, meta.SupportedEfforts); effort != "" {
		return &openAICompatReasoningWire{Effort: effort}
	}
	if on {
		return nil
	}
	enabled := true
	return &openAICompatReasoningWire{Enabled: &enabled}
}

// reasoningFloor is the `reasoning` block a request's floor sends on this
// binding, nil off the broker. A model list that cannot be read is logged and
// sends nothing: the floor is a hint, and the call it would fail worked before.
func (c *openAICompatClient) reasoningFloor(ctx context.Context, modelID, floor string) *openAICompatReasoningWire {
	if c.reasoning == nil || floor == "" {
		return nil
	}
	meta, err := c.reasoning.lookup(ctx, modelID, c.fetchReasoning)
	if err != nil {
		slog.WarnContext(ctx, "the broker's model list could not be read; this call is sent without its thinking floor",
			"model", modelID, "floor", floor, "error", err)
		return nil
	}
	return openRouterReasoningFor(meta, floor)
}

// fetchReasoning reads modelID's reasoning object from the broker's model
// list; unlisted when the model is absent or lists none.
func (c *openAICompatClient) fetchReasoning(ctx context.Context, modelID string) (openRouterReasoning, error) {
	ctx, cancel := context.WithTimeout(ctx, openRouterModelsTimeout)
	defer cancel()
	raw, err := getListBody(ctx, c.http, "openai-compat", c.baseURL+"/v1/models", func(r *http.Request) {
		r.Header.Set("Authorization", "Bearer "+c.apiKey)
	})
	if err != nil {
		return openRouterReasoning{}, err
	}
	var list struct {
		Data []struct {
			ID        string               `json:"id"`
			Reasoning *openRouterReasoning `json:"reasoning"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw, &list); err != nil {
		return openRouterReasoning{}, fmt.Errorf("ai: openai-compat: decode model list: %w", err)
	}
	for _, listed := range list.Data {
		if listed.ID == modelID && listed.Reasoning != nil {
			found := *listed.Reasoning
			found.listed = true
			return found, nil
		}
	}
	return openRouterReasoning{}, nil
}
