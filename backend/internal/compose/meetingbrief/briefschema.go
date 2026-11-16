// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package meetingbrief

import (
	"encoding/json"
	"maps"
	"slices"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/schema"
)

// briefSchema is wireReply, the shape ParseBriefSections decodes, with each
// closed set read from the table the parser filters by. A nature is closed to
// every section's union: a nature one section refuses drops that sentence, not
// the reply.
func briefSchema() json.RawMessage {
	natures := map[string]bool{}
	for _, allowed := range natureAllowed {
		maps.Copy(natures, allowed)
	}
	return schema.Must(schema.Record(
		schema.Field("sections", schema.Array(schema.Record(
			schema.Field("kind", schema.Enum(schema.Names(specSequence...)...)),
			schema.Field("sentences", schema.Array(schema.Record(
				schema.Field("text", schema.String()),
				schema.Field("nature", schema.Enum(slices.Sorted(maps.Keys(natures))...)),
				schema.Field("evidence", citations()),
			))),
		))),
	))
}

// citations is the evidence list both meeting replies carry.
func citations() schema.Node {
	return schema.Array(schema.Record(
		schema.Field("entity_type", schema.Enum(citeActivity, citeDeal, citeContact)),
		schema.Field("entity_id", schema.String()),
	))
}

// planSchema is planReply as the plan prompt spells it. It leaves out each
// nature the prompt never asks for, so the parser's per-field default labels it.
func planSchema() json.RawMessage {
	cited := schema.Record(
		schema.Field("text", schema.String()),
		schema.Field("evidence", citations()),
	)
	return schema.Must(schema.Record(
		schema.Field("objective", schema.Optional(cited)),
		schema.Field("opening", schema.Optional(cited)),
		schema.Field("top_risk", schema.Optional(schema.Record(
			schema.Field("text", schema.String()),
			schema.Field("evidence", citations()),
			schema.Field("say", schema.String()),
			schema.Field("show", schema.String()),
			schema.Field("avoid", schema.String()),
		))),
		schema.Field("likely_asks", schema.Array(schema.Record(
			schema.Field("question", schema.String()),
			schema.Field("basis", schema.String()),
			schema.Field("evidence", citations()),
			schema.Field("relevance", schema.Enum(schema.Names(
				crmcontracts.MeetingPlanTierMeetingPlanTierHigh,
				crmcontracts.MeetingPlanTierMeetingPlanTierMedium,
				crmcontracts.MeetingPlanTierMeetingPlanTierLow,
			)...)),
			schema.Field("prepare", schema.String()),
		))),
		schema.Field("questions", schema.Array(schema.Record(
			schema.Field("ask", schema.String()),
			schema.Field("why", schema.String()),
			schema.Field("listen_for", schema.String()),
			schema.Field("evidence", citations()),
		))),
		schema.Field("scenarios", schema.Array(schema.Record(
			schema.Field("label", schema.String()),
			schema.Field("play", schema.String()),
			schema.Field("coach", schema.Optional(schema.String())),
			schema.Field("evidence", citations()),
		))),
	))
}
