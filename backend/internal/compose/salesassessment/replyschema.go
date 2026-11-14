// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package salesassessment

import (
	"encoding/json"
	"maps"
	"slices"

	"github.com/margince/margince/backend/internal/shared/schema"
)

// replySchema is replyShape in the form the prompt asks for, closed to the
// refs the request showed. A cell or an action it did not supply refuses the
// whole reply. The schema leaves out the flat "sentences" key and a paragraph
// written as one object. Parse still reads both when a provider ignores it.
func replySchema(in Input) json.RawMessage {
	shown, _ := asShown(in)
	var cells, records, actions, deals, owners []string
	for _, cell := range shown.Cells {
		cells = append(cells, cell.ID)
	}
	for _, action := range shown.Actions {
		actions = append(actions, action.ID)
	}
	for _, candidate := range shown.Candidates {
		deals = append(deals, candidate.ID)
		owners = append(owners, candidate.OwnerID)
	}
	records = slices.Clone(deals)
	for _, decision := range shown.Decisions {
		records = append(records, decision.ID)
	}
	return schema.Must(schema.Record(
		schema.Field("paragraphs", schema.Array(schema.Array(schema.Record(
			schema.Field("text", schema.String()),
			schema.Field("kind", schema.Enum(slices.Sorted(maps.Keys(sentenceKinds))...)),
			schema.Field("cells", schema.Array(schema.Enum(distinct(cells)...))),
			schema.Field("records", schema.Array(schema.Enum(distinct(records)...))),
		)))),
		schema.Field("action_ids", schema.Array(schema.Enum(distinct(actions)...))),
		schema.Field("next_decision", schema.Optional(schema.Record(
			schema.Field("deal_id", schema.Enum(distinct(deals)...)),
			schema.Field("owner_id", schema.Enum(distinct(owners)...)),
			schema.Field("text", schema.String()),
		))),
		schema.Field("confidence", schema.Enum(slices.Sorted(maps.Keys(confidences))...)),
	))
}

// distinct is ids sorted with each named once, so an enum lists no value twice.
func distinct(ids []string) []string {
	return slices.Compact(slices.Sorted(slices.Values(ids)))
}
