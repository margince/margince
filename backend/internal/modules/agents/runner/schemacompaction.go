// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package runner

// The listing's schema compaction: what the SURFACE owns comes out of every
// rendered schema and is stated once in the frame instead (surfaceSchemaRules
// in window.go), because a per-tool sentence is paid for on every step of every
// run and a frame sentence is paid for once.

import (
	"encoding/json"

	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

// CompactSchema removes `"additionalProperties":false` from a rendered schema,
// at every depth, because the frame states it once (surfaceSchemaRules). The
// runtime refuses an unknown key without the schema: RejectNonCanonicalKeys on
// the core path, strictDecodeReportPlan on the report plan.
//
// Exported because the composition, which knows the whole served catalog, holds
// the listing to this compaction of every served schema. A schema that will not
// parse is rendered verbatim, since the registry refuses one at boot.
func CompactSchema(spec mcp.ToolSpec) string {
	return listingCompaction.render(spec)
}

// stepArguments is the args schema a step's tool-call branch carries: the
// listing's compaction, except that every object the tool closes stays closed.
// The frame's sentence covers the listing, but the step schema is ENFORCED at
// generation, and decodeArgs refuses the unknown key an open args would admit.
func stepArguments(spec mcp.ToolSpec) string {
	return argumentCompaction.render(spec)
}

// compaction is which of the surface-owned members a rendering takes out.
type compaction struct {
	keepClosed bool
}

var (
	listingCompaction  = compaction{keepClosed: false}
	argumentCompaction = compaction{keepClosed: true}
)

func (c compaction) render(spec mcp.ToolSpec) string {
	var shape map[string]json.RawMessage
	if err := json.Unmarshal(spec.InputSchema, &shape); err != nil {
		return string(spec.InputSchema)
	}
	compacted, err := json.Marshal(c.shape(shape))
	if err != nil {
		return string(spec.InputSchema)
	}
	return string(compacted)
}

// shape rewrites one schema object and everything nested under it.
//
// It recurses because `additionalProperties` is not only a top-level member:
// run_report's `aggregates` items close themselves, and the whole-catalog count
// exceeds the tool count for that reason.
//
// THE RECURSION WALKS `properties`, `items` AND `additionalProperties`, which is
// every keyword this surface nests an object schema under — checked against the
// served catalog rather than assumed, and `additionalProperties` was the one an
// earlier pass missed (qualify_lead nests a `properties` tree under one). The
// claim is HELD rather than asserted: assertObjectSchemas refuses a served
// schema that composes with allOf/anyOf/oneOf/$ref under those same three keys,
// at any depth, so there is no fourth place for an object schema to hide.
//
// Read back as members and re-marshalled, the way spliceRetryKey does it:
// marshalling a map sorts its keys, so every process renders the same bytes and
// the equivalence gate can compare them.
func (c compaction) shape(shape map[string]json.RawMessage) map[string]json.RawMessage {
	out := make(map[string]json.RawMessage, len(shape))
	for key, raw := range shape {
		switch key {
		case schemaAdditionalProperties:
			// Only the CLOSED form is dropped. A schema constraining what an
			// open object's extra members must look like is saying something
			// the frame does not, so it survives — and it is RECURSED INTO,
			// because it is an object schema like any other and can close
			// itself. qualify_lead nests a whole `properties` tree under one.
			if string(raw) == "false" {
				if c.keepClosed {
					out[key] = raw
				}
				continue
			}
			out[key] = c.nested(raw)
		case schemaProperties:
			out[key] = c.properties(raw)
		case schemaItems:
			out[key] = c.nested(raw)
		default:
			out[key] = raw
		}
	}
	return out
}

// properties compacts each property's own schema.
func (c compaction) properties(raw json.RawMessage) json.RawMessage {
	var properties map[string]json.RawMessage
	if err := json.Unmarshal(raw, &properties); err != nil {
		return raw
	}
	rewritten := make(map[string]json.RawMessage, len(properties))
	for name, property := range properties {
		rewritten[name] = c.nested(property)
	}
	encoded, err := json.Marshal(rewritten)
	if err != nil {
		return raw
	}
	return encoded
}

// nested applies the compaction one level down, leaving anything that is not an
// object schema exactly as it arrived.
func (c compaction) nested(raw json.RawMessage) json.RawMessage {
	var nested map[string]json.RawMessage
	if err := json.Unmarshal(raw, &nested); err != nil {
		return raw
	}
	encoded, err := json.Marshal(c.shape(nested))
	if err != nil {
		return raw
	}
	return encoded
}

// The JSON Schema keywords the compaction reasons about.
//
// A typo here makes the compaction a silent NO-OP — it would look for a member
// no schema has, remove nothing, and render a listing identical to the served
// surface with the saving quietly gone. What catches that is the composition's
// census, which counts what came out of the real catalog and refuses to pass on
// nothing: TestTheCompactionStillRemovesWhatTheFrameStatesOnce
// (internal/compose/agenttoollistingcompaction_test.go). It reads the keyword as
// a literal rather than through these constants, on purpose — a gate that
// imported the spelling it is checking would agree with the typo.
const (
	schemaAdditionalProperties = "additionalProperties"
	schemaProperties           = "properties"
	schemaItems                = "items"
)
