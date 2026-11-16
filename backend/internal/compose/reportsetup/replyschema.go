// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package reportsetup

import (
	"encoding/json"

	"github.com/margince/margince/backend/internal/shared/schema"
)

// replySchema is replyShape. Ids stay open strings because the gate also
// resolves a value by its label, and names back what the catalog lacks.
func replySchema() json.RawMessage {
	picked := schema.Array(schema.Record(
		schema.Field("id", schema.String()),
		schema.Field("phrase", schema.String()),
	))
	return schema.Must(schema.Record(
		schema.Field("name", schema.String()),
		schema.Field("filters", schema.Array(schema.Record(
			schema.Field("field", schema.Enum("product", "industry", "country", "period")),
			schema.Field("value", schema.String()),
			schema.Field("phrase", schema.String()),
		))),
		schema.Field("metrics", picked),
		schema.Field("charts", picked),
		schema.Field("objective", schema.Optional(schema.Record(
			schema.Field("metric", schema.String()),
			schema.Field("target_value", schema.Optional(schema.Integer())),
			schema.Field("deadline", schema.Optional(schema.String())),
			schema.Field("phrase", schema.String()),
		))),
		schema.Field("clarifications", schema.Array(schema.String())),
		schema.Field("unsupported", schema.Array(schema.Record(
			schema.Field("phrase", schema.String()),
			schema.Field("reason", schema.String()),
		))),
	))
}
