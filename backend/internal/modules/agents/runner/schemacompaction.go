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
//     a fact about the surface and not about the tool it lands on.
//   - `"additionalProperties":false`. It is how this surface answers an unknown
//     key EVERYWHERE, and the runtime enforces that independently of the schema
//     — RejectNonCanonicalKeys on the core path, strictDecodeReportPlan on the
//     report plan — so the frame's sentence is true of the surface rather than a
//     promise the schema was making alone.
//
// KEYED ON OWNERSHIP, never on description text, and ownership means the name
// AT THE LEVEL the surface writes it. Dropping a member because its wording
// matched would go wrong the first time a tool worded its own; dropping one
// because its NAME matched at any depth would go wrong the first time a batch
// tool carried a per-item key of the same name, which is why the level is part
// of the rule (atSchemaRoot below). `approval_id` is deliberately NOT compacted
// at all: it carries three descriptions and one of them is a per-tool replay
// instruction that no frame sentence replaces.
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
func CompactSchema(spec mcp.ToolSpec) string {
	var shape map[string]json.RawMessage
	if err := json.Unmarshal(spec.InputSchema, &shape); err != nil {
		return string(spec.InputSchema)
	}
	// No predicate on the tool. assertNoDeclaredRetryKey refuses ANY tool that
	// declares the member at its own root, so the only root `idempotency_key` on
	// the surface is the one spliceRetryKey put there — and stripping the
	// description of the surface's own member is what this is for. Deciding it
	// from the spec instead would mean a second predicate beside withRetryKey's,
	// and the two disagreed on extension-owned tools.
	compacted, err := json.Marshal(compactSchemaShape(shape, atSchemaRoot))
	if err != nil {
		return string(spec.InputSchema)
	}
	return string(compacted)
}

// atSchemaRoot / nestedInSchema say which level compactSchemaShape is walking,
// because ONE of the two omissions is level-sensitive and the other is not.
//
// The surface owns `idempotency_key` at the ROOT of a mutating tool's schema and
// nowhere else: spliceRetryKey writes it into the top-level `properties` and
// refuses a tool that declares it there itself. A member of that name nested
// inside an array's items is somebody's own per-item key — a batch tool's, say —
// and the frame's sentence is not about it, so taking its description away would
// be silently removing per-tool meaning.
//
// `"additionalProperties":false` is not level-sensitive: it means the same thing
// wherever it appears, and the frame's sentence covers all of it.
const (
	atSchemaRoot   = true
	nestedInSchema = false
)

// compactSchemaShape rewrites one schema object and everything nested under it.
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
func compactSchemaShape(shape map[string]json.RawMessage, root bool) map[string]json.RawMessage {
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
				continue
			}
			out[key] = compactNestedSchema(raw)
		case schemaProperties:
			out[key] = compactSchemaProperties(raw, root)
		case schemaItems:
			out[key] = compactNestedSchema(raw)
		default:
			out[key] = raw
		}
	}
	return out
}

// compactSchemaProperties compacts each property's own schema, and at the ROOT
// strips the description from the one member the surface owns there.
func compactSchemaProperties(raw json.RawMessage, root bool) json.RawMessage {
	var properties map[string]json.RawMessage
	if err := json.Unmarshal(raw, &properties); err != nil {
		return raw
	}
	rewritten := make(map[string]json.RawMessage, len(properties))
	for name, property := range properties {
		if root && name == mcp.ReservedIdempotencyKeyArg {
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
	encoded, err := json.Marshal(compactSchemaShape(nested, nestedInSchema))
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
