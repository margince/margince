// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

// What a SERVED SCHEMA may look like, refused at boot.
//
// Split from registration.go on the 500-line cap, and the boundary is a real
// one: everything here judges the SHAPE of a schema — is it an object, does it
// compose, is there a branch the surface cannot walk — while registration.go
// judges the spec's identity and the two arguments the surface owns. Only these
// checks recurse, and they are the ones three other things depend on:
// spliceRetryKey cannot splice a composed schema, the runner's listing renderer
// cannot walk one, and the response assembler cannot read a result shape behind
// a `$ref`.

import (
	"encoding/json"
	"fmt"

	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

// assertObjectSchemas holds two promises tools/list and tools/call have to
// keep, at the one door every tool comes through.
//
// The first is ENCODABILITY. Both schemas are hand-written JSON literals
// spliced together from constants, and they reach the client by being embedded
// verbatim into the tools/list response — so ONE misplaced brace does not
// break one tool, it makes the whole listing unencodable and every tool
// disappears behind a 500. That is a boot-time defect discovered on a client's
// first request, which is exactly the wrong end.
//
// The second is that both are OBJECT schemas. MCP requires an object input
// schema, and a declared outputSchema obliges the server to answer with
// structured content conforming to it — which the dispatcher can only do for
// an object, because structuredContent is typed as one. A schema written some
// other way (a $ref, a bare allOf) fails here on purpose: not wrong, but not
// something the dispatcher has been taught to honour, and failing at boot
// beats advertising a shape the results miss.
func assertObjectSchemas(spec mcp.ToolSpec) error {
	if spec.InputSchema == nil {
		// The protocol requires one. A tool taking no arguments still declares
		// `{"type":"object"}`; nil would put a bare null on tools/list.
		return fmt.Errorf("%s declares no InputSchema; MCP requires every tool to advertise an object input schema", spec.Name)
	}
	for _, s := range []struct {
		field string
		raw   json.RawMessage
	}{
		{field: "InputSchema", raw: spec.InputSchema},
		// Optional: a tool promising no output shape owes tools/call no
		// structured content.
		{field: "OutputSchema", raw: spec.OutputSchema},
	} {
		if s.raw == nil {
			continue
		}
		// Decoded ONCE, as members, because two things are judged from it: the
		// declared type and whether the schema composes. Decoding twice would
		// mean a second error to either report redundantly or swallow.
		var declared map[string]json.RawMessage
		if err := json.Unmarshal(s.raw, &declared); err != nil {
			return fmt.Errorf("%s has an %s that is not valid JSON, which makes the whole tools/list response unencodable: %w",
				spec.Name, s.field, err)
		}
		var declaredType string
		if raw, stated := declared["type"]; stated {
			if err := json.Unmarshal(raw, &declaredType); err != nil {
				return fmt.Errorf("%s's %s declares a `type` that is not a string: %w", spec.Name, s.field, err)
			}
		}
		if declaredType != "object" {
			return fmt.Errorf("%s declares %s type %q; this surface serves object schemas only",
				spec.Name, s.field, declaredType)
		}
		// A COMPOSING schema is refused for EVERY served spec, mutating or not.
		// The runner's listing renderer reaches an object schema only through the
		// keys assertNoSchemaComposition walks, so a composed branch keeps its
		// closed form and the headroom the budget page publishes is larger than
		// the headroom that exists — with nothing failing.
		if err := assertNoSchemaComposition(spec.Name, s.field, declared); err != nil {
			return err
		}
	}
	return nil
}

// composingKeywords are the JSON Schema branches this surface cannot follow:
// the retry-key splice cannot reach inside one, the runner's listing renderer
// does not walk one, and the response assembler cannot read a result shape
// behind one.
//
// TestAComposedInputSchemaIsRefusedAtBoot iterates this list rather than naming
// keywords, so a branch added here is refused and exercised in the same commit.
var composingKeywords = []string{"allOf", "anyOf", "oneOf", "$ref"}

// assertNoSchemaComposition refuses a schema this surface cannot reason about,
// AT EVERY DEPTH.
//
// Three things need every object schema to be reachable by walking the keys
// below, and each breaks silently rather than loudly without this:
//
//   - spliceRetryKey cannot add a top-level member to a schema whose closed
//     branch would then reject it, so a schema-aware client would be told to
//     send an argument its own validator refuses.
//   - the runner's listing renderer walks those same keys and no others, so a
//     composed branch keeps its `"additionalProperties":false` and the headroom
//     the budget page publishes is larger than the headroom that exists.
//   - the dispatcher can only honour an object `structuredContent`, which is
//     why this binds OutputSchema too: a result shape behind a `$ref` is a
//     promise the response assembler cannot read.
//
// RECURSIVE, because a root-only check holds none of the three. A branch spelled
// `properties.foo.allOf` passes a root check, and then the renderer copies
// `allOf` through verbatim without ever entering it. That is the same
// fail-short shape as a census that reads a smaller tree and reports PASS.
//
// It takes the DECODED members rather than the bytes, so there is one decode of
// each object and no second error to swallow.
func assertNoSchemaComposition(tool, field string, shape map[string]json.RawMessage) error {
	for _, keyword := range composingKeywords {
		if _, composed := shape[keyword]; composed {
			return fmt.Errorf("%s's %s uses `%s`, which this surface cannot reason about: "+
				"the retry-key splice cannot reach inside it, the runner's listing renderer "+
				"would not walk it, and the response assembler cannot read a result shape "+
				"behind it", tool, field, keyword)
		}
	}
	// The same keys the renderer walks, so the refusal covers exactly what the
	// renderer can reach and nothing it cannot. `additionalProperties` belongs
	// here as much as the other two: it can be a full object schema, and
	// qualify_lead nests a `properties` tree under one.
	for _, nested := range []string{"properties", "items", "additionalProperties"} {
		raw, present := shape[nested]
		if !present {
			continue
		}
		if err := assertNoCompositionUnder(tool, field, nested, raw); err != nil {
			return err
		}
	}
	return nil
}

// assertNoCompositionUnder descends one `properties` map, or the one schema
// `items` / `additionalProperties` holds.
//
// A TUPLE-FORM `items` — an ARRAY of schemas rather than one — is REFUSED rather
// than skipped, which is what makes "the walk reaches every object schema" a
// held claim rather than an assertion. Neither this walk nor the renderer's
// descends into an array of schemas, so a closed object or a `$ref` in a tuple
// slot would be invisible to both: the listing would keep bytes the frame says
// it omits, and the refusal would pass a branch it cannot reason about.
//
// Anything else that does not decode as an object — a boolean schema, say —
// carries no branch to refuse and is left alone.
func assertNoCompositionUnder(tool, field, keyword string, raw json.RawMessage) error {
	var decoded map[string]json.RawMessage
	if err := json.Unmarshal(raw, &decoded); err != nil {
		var tuple []json.RawMessage
		if json.Unmarshal(raw, &tuple) == nil && keyword == "items" {
			return fmt.Errorf("%s's %s uses the tuple form of `items` (an array of schemas), which "+
				"this surface cannot reason about: neither the composition refusal nor the runner's "+
				"listing renderer descends into it, so anything nested there would be invisible to "+
				"both", tool, field)
		}
		// A boolean schema or another non-object: it carries no branch to
		// refuse, and assertObjectSchemas has already judged the root's shape.
		return nil
	}
	// `items` and `additionalProperties` hold ONE schema; `properties` holds a
	// map of them.
	if keyword != "properties" {
		return assertNoSchemaComposition(tool, field, decoded)
	}
	for name, property := range decoded {
		var member map[string]json.RawMessage
		if err := json.Unmarshal(property, &member); err != nil {
			continue
		}
		if err := assertNoSchemaComposition(tool, field+"."+name, member); err != nil {
			return err
		}
	}
	return nil
}
