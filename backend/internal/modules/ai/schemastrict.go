// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import "encoding/json"

// schemaNode is a decoded JSON Schema object — the only shape this walk
// recurses into. Named rather than spelled inline so the recursion reads as
// "an object, and the objects under it" instead of as untyped traversal.
type schemaNode map[string]any

// schemaAllowsStrict reports whether a response schema already satisfies the
// rules OpenAI's strict structured output imposes: every object closed with
// additionalProperties:false, and every one of its properties named in
// required.
//
// The wire's `strict` flag decides whether the endpoint ENFORCES the schema or
// merely passes it along as a suggestion. Withheld, stage_evidence_extract on
// ministral-14b returned a reply carrying no `claims` key at all on 15 of 27
// certification runs; enforced, on 3.
//
// Enforcement is not a substitute for capability, and this is worth stating
// because the first reading of that result was that it would be: the same
// model, asked on the same wire for a multi-paragraph email body, breaks the
// JSON string at the paragraph boundary whether or not the schema is enforced.
// A shape the model cannot hold is not held by insisting on it.
//
// So strictness is derived rather than assumed in either direction. Hard-coding
// it true would have the endpoint reject any schema a caller did not write to
// OpenAI's shape; hard-coding it false — which is what this replaced — withholds
// the enforcement from the callers that did write one.
func schemaAllowsStrict(raw json.RawMessage) bool {
	if len(raw) == 0 {
		return false
	}
	var root schemaNode
	if err := json.Unmarshal(raw, &root); err != nil {
		// A schema this adapter cannot read is one it cannot vouch for, and
		// claiming the stricter contract on the caller's behalf would turn a
		// schema the endpoint might have tolerated into a refused request.
		return false
	}
	return root.allowsStrict()
}

// allowsStrict walks every subschema, because the rule binds all of them: an
// impeccable outer object with one open object nested inside it is refused
// whole, and a check that only read the top level would send strict for it.
func (n schemaNode) allowsStrict() bool {
	if n["type"] == "object" && !n.isClosed() {
		return false
	}
	for key, child := range n {
		// `required` is the one keyword whose value is a list of property
		// NAMES rather than of subschemas, so walking it would read a name as
		// a schema. Everything else either holds subschemas or is scalar, and
		// a scalar answers true.
		if key == "required" {
			continue
		}
		if !allSubschemasAllowStrict(child) {
			return false
		}
	}
	return true
}

// allSubschemasAllowStrict recurses into whichever shape a keyword's value
// holds — one subschema (`items`), a list of them (`anyOf`), or a map of them
// (`properties`) — and passes over anything that is not a schema at all.
//
//craft:ignore naked-any a decoded JSON Schema keyword's value is object, array or scalar by turns; any concrete type here would be one of the three and would have to be asserted from `any` anyway
func allSubschemasAllowStrict(value any) bool {
	switch shape := value.(type) {
	case map[string]any:
		// Either one subschema or a map of them; both are answered by asking
		// this node and everything below it, since a plain keyword map holds
		// no "type":"object" to fail on.
		return schemaNode(shape).allowsStrict()
	case []any:
		for _, item := range shape {
			if nested, isObject := item.(map[string]any); isObject && !schemaNode(nested).allowsStrict() {
				return false
			}
		}
		return true
	default:
		return true
	}
}

// isClosed holds one object to the two rules: additionalProperties is exactly
// false, and required names every declared property.
func (n schemaNode) isClosed() bool {
	if closed, declared := n["additionalProperties"].(bool); !declared || closed {
		return false
	}
	props, hasProps := n["properties"].(map[string]any)
	if !hasProps {
		// No properties to omit, so nothing can be missing from required.
		return true
	}
	required := map[string]bool{}
	names, _ := n["required"].([]any)
	for _, name := range names {
		if text, isText := name.(string); isText {
			required[text] = true
		}
	}
	for name := range props {
		if !required[name] {
			return false
		}
	}
	return true
}
