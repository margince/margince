// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The one model call of the tag suggestion pass: which suggestible tags each
// captured item shows interest in. The request, its schema and its validator
// are pure functions of the batch, so the certification case issues and judges
// the call that ships.

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/margince/margince/backend/internal/compose/tagsuggest"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/collections"
	"github.com/margince/margince/backend/internal/shared/kernel/promptfence"
	"github.com/margince/margince/backend/internal/shared/ports/model"
	"github.com/margince/margince/backend/internal/shared/schema"
)

const (
	// tagSuggestBatchSize is how many items one call reads, like the other
	// batched mail passes on the same small-model ladder.
	tagSuggestBatchSize = 10
	// tagSuggestBodyLimit truncates each item's text for the prompt.
	tagSuggestBodyLimit = 1500
)

// The JSON keys the schema declares and the payload decodes.
const (
	tagSuggestItemsKey = "items"
	tagSuggestTagsKey  = "tags"
)

// tagSuggestAnswer is the model's reading of one item: the tag ids it shows
// interest in, possibly none.
type tagSuggestAnswer struct {
	ID   string   `json:"id"`
	Tags []string `json:"tags"` // tagSuggestTagsKey
}

func (a tagSuggestAnswer) answeredID() string { return a.ID }

type tagSuggestPayload struct {
	Items []tagSuggestAnswer `json:"items"` // tagSuggestItemsKey
}

// tagSuggestRequest builds the call that reads one batch against the
// vocabulary. One fence wraps each item in its own span, so no item can close
// its span and reach another's.
//
//promptlang:exempt the reply is tag ids keyed by item id, never a sentence; a language instruction could only turn an enum into a parse failure.
//promptvoice:exempt the reply is tag ids keyed by item id, never a sentence.
func tagSuggestRequest(tags []collections.SuggestibleTag, items []tagsuggest.Item) model.Request {
	fence := promptfence.New()
	return model.Request{
		System:         tagsuggest.SystemFor(fence),
		Messages:       []model.Message{{Role: chatRoleUser, Content: tagsuggest.Prompt(fence, tags, items)}},
		MaxTokens:      ai.ReasoningOutputMaxTokens,
		ResponseSchema: tagSuggestSchema(tagSuggestItemIDs(items), tagSuggestTagIDs(tags)),
		SecretStripper: ai.NewSecretStripper(),
	}
}

// tagSuggestSchema holds both the item ids and the tag ids to the ones this
// call sent. No confidence field: a bare number the model must invent per item
// costs whole batches to a validator and buys no decision.
func tagSuggestSchema(itemIDs, tagIDs []string) json.RawMessage {
	return schema.Must(schema.Object(
		map[string]schema.Node{
			tagSuggestItemsKey: schema.Array(schema.Object(
				map[string]schema.Node{
					"id":              requestedIDNode(itemIDs),
					tagSuggestTagsKey: schema.Array(schema.Enum(tagIDs...)),
				},
				"id", tagSuggestTagsKey)),
		},
		tagSuggestItemsKey))
}

// tagSuggestShapeValid is the deterministic floor beneath the schema: each sent
// item answered, none twice, and no tag the call did not list.
func tagSuggestShapeValid(tags []collections.SuggestibleTag, items []tagsuggest.Item) ai.Validator {
	return func(text string) error {
		_, err := parseTagSuggestReply(text, tags, items)
		return err
	}
}

// parseTagSuggestReply decodes and validates one reply.
func parseTagSuggestReply(text string, tags []collections.SuggestibleTag, items []tagsuggest.Item) ([]tagSuggestAnswer, error) {
	var payload tagSuggestPayload
	if err := json.Unmarshal([]byte(ai.Unfence(text)), &payload); err != nil {
		return nil, fmt.Errorf("output is not the required JSON shape: %w", err)
	}
	if msg := validateTagSuggestPayload(payload, tags, items); msg != "" {
		return nil, errors.New(msg)
	}
	return payload.Items, nil
}

// validateTagSuggestPayload names the first violation, or "" when the payload
// answers the sent items with listed tags.
func validateTagSuggestPayload(payload tagSuggestPayload, tags []collections.SuggestibleTag, items []tagsuggest.Item) string {
	if msg := checkBatchFidelity(payload.Items, tagSuggestItemIDs(items)); msg != "" {
		return msg
	}
	listed := map[string]bool{}
	for _, id := range tagSuggestTagIDs(tags) {
		listed[id] = true
	}
	for _, answer := range payload.Items {
		for _, tag := range answer.Tags {
			if !listed[tag] {
				return fmt.Sprintf("tag %q for item %q is not one of the listed tags", clampToken(tag), answer.ID)
			}
		}
	}
	return ""
}

func tagSuggestItemIDs(items []tagsuggest.Item) []string {
	out := make([]string, len(items))
	for i, item := range items {
		out[i] = item.ID.String()
	}
	return out
}

func tagSuggestTagIDs(tags []collections.SuggestibleTag) []string {
	out := make([]string, len(tags))
	for i, t := range tags {
		out[i] = t.ID.String()
	}
	return out
}
