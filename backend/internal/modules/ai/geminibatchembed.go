// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"context"
	"encoding/json"
	"fmt"
)

// geminiBatchEmbedLimit is the most inputs one :batchEmbedContents call may
// carry; the API refuses a batch of 101.
const geminiBatchEmbedLimit = 100

// geminiBatchEmbedWire is the :batchEmbedContents request body: one
// :embedContent request per input, answered in the same order.
type geminiBatchEmbedWire struct {
	Requests []geminiEmbedWire `json:"requests"`
}

// embedInputs embeds the inputs in order, in as few round trips as the
// transport allows.
//
// AI Studio takes a batch in one :batchEmbedContents call. Vertex AI serves
// this model one instance per :predict, so there every input is its own call.
// A lone input keeps its :embedContent call on either, so a question's embed
// is the request it always was.
func (c *geminiClient) embedInputs(ctx context.Context, embedModel string, inputs []string, dims int) ([][]float32, error) {
	vectors := make([][]float32, 0, len(inputs))
	if _, vertex := c.transport.(vertexTransport); vertex || len(inputs) == 1 {
		for _, input := range inputs {
			values, err := c.embedOne(ctx, embedModel, input, dims)
			if err != nil {
				return nil, err
			}
			vectors = append(vectors, values)
		}
		return vectors, nil
	}
	for start := 0; start < len(inputs); start += geminiBatchEmbedLimit {
		batch, err := c.embedBatch(ctx, embedModel, inputs[start:min(start+geminiBatchEmbedLimit, len(inputs))], dims)
		if err != nil {
			return nil, err
		}
		vectors = append(vectors, batch...)
	}
	return vectors, nil
}

// embedBatch embeds at most geminiBatchEmbedLimit inputs in one call.
func (c *geminiClient) embedBatch(ctx context.Context, embedModel string, inputs []string, dims int) ([][]float32, error) {
	wire := geminiBatchEmbedWire{Requests: make([]geminiEmbedWire, len(inputs))}
	for i, input := range inputs {
		wire.Requests[i] = geminiEmbedWire{
			Model:                "models/" + embedModel,
			Content:              geminiContent{Parts: []geminiPart{{Text: input}}},
			OutputDimensionality: dims,
		}
	}
	payload, _, err := SendablePayload(ctx, wire, nil)
	if err != nil {
		return nil, err
	}
	var out struct {
		Embeddings []struct {
			Values []float32 `json:"values"`
		} `json:"embeddings"`
	}
	decode := func(d *json.Decoder) error { return d.Decode(&out) }
	if err := c.postEmbed(ctx, c.transport.modelURL(embedModel, "batchEmbedContents"), payload, decode); err != nil {
		return nil, err
	}
	// The answer is positional, so a short one would put every later vector
	// on the wrong input.
	if len(out.Embeddings) != len(inputs) {
		return nil, fmt.Errorf("ai: gemini: batch embed answered %d vectors for %d inputs", len(out.Embeddings), len(inputs))
	}
	vectors := make([][]float32, len(out.Embeddings))
	for i, e := range out.Embeddings {
		vectors[i] = e.Values
	}
	return vectors, nil
}
