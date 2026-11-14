// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package meetingbrief

import (
	"encoding/json"
	"maps"
	"slices"

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
				schema.Field("evidence", schema.Array(schema.Record(
					schema.Field("entity_type", schema.Enum(citeActivity, citeDeal, citeContact)),
					schema.Field("entity_id", schema.String()),
				))),
			))),
		))),
	))
}
