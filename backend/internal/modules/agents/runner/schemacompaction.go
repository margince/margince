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

// CompactSchema removes from a rendered schema the things the SURFACE owns and
// the frame therefore states once, rather than once per tool.
//
// Two of them, and both were printed in full on every step of every run:
//
//   - `idempotency_key`'s description. There is ONE definition of that member
//     (the surface splices it into every mutating core tool), so its sentence is
//     a fact about the surface and not about the tool it lands on. It was 32
//     copies of one sentence.
//   - `"additionalProperties":false`. It is how this surface answers an unknown
//     key EVERYWHERE, and the runtime enforces that independently of the schema
//     — RejectNonCanonicalKeys on the core path, strictDecodeReportPlan on the
//     report plan — so the frame's sentence is true of the surface rather than a
//     promise the schema was making alone.
//
// KEYED ON OWNERSHIP, never on description text. Dropping a member because its
// wording matched would go wrong the first time a tool worded its own; the
// surface owns `idempotency_key` by name, which is why the name lives in the
// port. `approval_id` is deliberately NOT compacted: it carries three
// descriptions and one of them is a per-tool replay instruction that no frame
// sentence replaces.
//
// The member itself STAYS — only its description goes. A model still has to
// know the argument exists, its type and its bound to send one.
//
// tools/list is untouched: this is the runner's rendering, and mcp-info.md is
// the check on that.
//
// EXPORTED for the same reason ToolListing is, and it is the same gate: the
// invariant to hold is that applying this to every SERVED schema equals the
// schema the listing renders, and the only package that knows the whole served
// catalog is the composition. A second, hand-written idea of the compaction over
// there would drift from this one silently, and the drift would read as headroom
// that is not there.
//
// A schema that will not parse is rendered VERBATIM rather than dropped or
// replaced. The registry refuses one at boot (assertObjectSchemas), so this is
// unreachable from the real surface; a caller who reached it anyway is better
// served by an unhelpfully large schema than by a silently absent one.
func CompactSchema(inputSchema json.RawMessage) string {
	var shape map[string]json.RawMessage
	if err := json.Unmarshal(inputSchema, &shape); err != nil {
		return string(inputSchema)
	}
	compacted, err := json.Marshal(compactSchemaShape(shape))
	if err != nil {
		return string(inputSchema)
	}
	return string(compacted)
}

// compactSchemaShape rewrites one schema object and everything nested under it.
//
// It recurses because `additionalProperties` is not only a top-level member:
// run_report's `aggregates` items close themselves, and the whole-catalog count
// is 77 across 70 tools for that reason. The recursion walks `properties` and
// `items`, which are the two places this surface nests an object schema.
//
// Read back as members and re-marshalled, the way spliceRetryKey does it:
// marshalling a map sorts its keys, so every process renders the same bytes and
// the equivalence gate can compare them.
func compactSchemaShape(shape map[string]json.RawMessage) map[string]json.RawMessage {
	out := make(map[string]json.RawMessage, len(shape))
	for key, raw := range shape {
		switch key {
		case schemaAdditionalProperties:
			// Only the CLOSED form. A schema constraining what an open object's
			// extra members must look like is saying something the frame does
			// not, so it survives.
			if string(raw) == "false" {
				continue
			}
			out[key] = raw
		case schemaProperties:
			out[key] = compactSchemaProperties(raw)
		case schemaItems:
			out[key] = compactNestedSchema(raw)
		default:
			out[key] = raw
		}
	}
	return out
}

// compactSchemaProperties compacts each property's own schema, and strips the
// description from the one member the surface owns.
func compactSchemaProperties(raw json.RawMessage) json.RawMessage {
	var properties map[string]json.RawMessage
	if err := json.Unmarshal(raw, &properties); err != nil {
		return raw
	}
	rewritten := make(map[string]json.RawMessage, len(properties))
	for name, property := range properties {
		if name == mcp.ReservedIdempotencyKeyArg {
			rewritten[name] = withoutDescription(property)
			continue
		}
		rewritten[name] = compactNestedSchema(property)
	}
	encoded, err := json.Marshal(rewritten)
	if err != nil {
		return raw
	}
	return encoded
}

// compactNestedSchema applies the compaction one level down, leaving anything
// that is not an object schema exactly as it arrived.
func compactNestedSchema(raw json.RawMessage) json.RawMessage {
	var nested map[string]json.RawMessage
	if err := json.Unmarshal(raw, &nested); err != nil {
		return raw
	}
	encoded, err := json.Marshal(compactSchemaShape(nested))
	if err != nil {
		return raw
	}
	return encoded
}

// withoutDescription drops the `description` member and keeps everything else,
// so the argument's type and bound still reach the model.
func withoutDescription(raw json.RawMessage) json.RawMessage {
	var member map[string]json.RawMessage
	if err := json.Unmarshal(raw, &member); err != nil {
		return raw
	}
	delete(member, schemaDescription)
	encoded, err := json.Marshal(member)
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
	schemaDescription          = "description"
)
