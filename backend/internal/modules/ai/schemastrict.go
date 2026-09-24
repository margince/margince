// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"encoding/json"

	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// schemaNode is a decoded JSON Schema object — the only shape this walk
// recurses into. Named rather than spelled inline so the recursion reads as
// "an object, and the objects under it" instead of as untyped traversal.
type schemaNode map[string]any

// strictKeywords is the vocabulary a strict schema may use: the structural
// keywords plus the annotations, and nothing that VALIDATES a value.
//
// An allowlist because the failure is asymmetric. A blocklist admits every
// keyword nobody thought of, and the endpoint answers an unknown one with a
// 400 that fails the whole call — strictly worse than the unenforced answer it
// was meant to improve. `uniqueItems` is how this was found: cold_start's
// schema carries it, and Mistral refused the request outright (code 3051,
// "Invalid structured output syntax") where before it had answered.
//
// `$ref`, `$defs` and `definitions` are absent deliberately. A reference is
// only as eligible as its target, and answering that needs resolution this
// walk does not do — a dangling or open target is a 400. Refusing the keyword
// costs a schema its enforcement, which is the direction this may fail in.
var strictKeywords = map[string]bool{
	kwType: true, kwProperties: true, "required": true, kwAdditionalProperties: true,
	kwItems: true, kwAnyOf: true, "enum": true, "const": true,
	kwDescription: true, "title": true,
}

// strictDowngrade is the downgrade an OpenAI-wire request goes under: a
// schema schemaAllowsStrict refuses is still sent, with strict false, which
// leaves enforcing it to the endpoint — model.SchemaUnenforced.
// Both wires that send the flag report it, from this one predicate, so the
// record and the wire cannot disagree.
func strictDowngrade(raw json.RawMessage) string {
	if len(raw) == 0 || schemaAllowsStrict(raw) {
		return ""
	}
	return model.SchemaUnenforced
}

// schemaAllowsStrict reports whether a response schema already satisfies the
// rules OpenAI's strict structured output imposes: an object at the root,
// every object closed with additionalProperties:false, every one of its
// properties named in required, and no keyword outside the strict vocabulary.
//
// The wire's `strict` flag decides whether the endpoint ENFORCES the schema or
// merely passes it along as a suggestion. Withheld, stage_evidence_extract on
// ministral-14b returned replies carrying no `claims` key at all on most of a
// 27-run certification; enforced, on three of them.
//
// Enforcement is not a substitute for capability: a shape the model cannot
// hold is not held by insisting on it, and which models hold which shapes is
// what a certification record says, not this function.
//
// EVERY unreadable shape answers false. The two mistakes are not equals — a
// false no withholds enforcement and leaves today's behaviour, a false yes
// turns an answered call into a 400 — so a shape this walk does not recognise
// is refused rather than assumed harmless.
func schemaAllowsStrict(raw json.RawMessage) bool {
	if len(raw) == 0 {
		return false
	}
	var root schemaNode
	if err := json.Unmarshal(raw, &root); err != nil {
		return false
	}
	// The strict profile takes an object at the root and nothing else: a bare
	// string, an array, or a root `anyOf` is refused by the endpoint.
	if !root.isObject() {
		return false
	}
	return root.allowsStrict()
}

// isObject reports whether this node describes an object, by any of the
// spellings a schema may use: `type: object`, a union that includes it
// (`["object","null"]`, the ordinary nullable struct), or a bare `properties`.
//
// All three, because a node this misses is never asked whether it is closed —
// so an outer schema can be impeccable while an open object nests inside it.
func (n schemaNode) isObject() bool {
	if n[kwType] == kwObject {
		return true
	}
	if union, isUnion := n[kwType].([]any); isUnion {
		for _, member := range union {
			if member == kwObject {
				return true
			}
		}
	}
	_, declaresProperties := n[kwProperties]
	return declaresProperties
}

// allowsStrict walks every subschema, because the rule binds all of them: an
// impeccable outer object with one open object nested inside it is refused
// whole, and a check that only read the top level would send strict for it.
func (n schemaNode) allowsStrict() bool {
	if n.isObject() && !n.isClosed() {
		return false
	}
	for key, child := range n {
		if !strictKeywords[key] {
			return false
		}
		// Only the keywords whose values are SCHEMAS are walked. The rest hold
		// names, values or annotations — `enum` may hold objects that are not
		// schemas — and isClosed is what reads `required` and
		// `additionalProperties`.
		switch key {
		case kwAdditionalProperties:
			if _, isBool := child.(bool); !isBool {
				return false
			}
		// A map KEYED BY NAME: its keys are the caller's property names, which
		// must not be held to the keyword vocabulary, while its values are
		// schemas that must be.
		case kwProperties:
			if !propertiesAllowStrict(child) {
				return false
			}
		// One subschema only. The tuple form (`items` as a list) is not in the
		// strict profile, so a list here is a refusal rather than a walk.
		case kwItems:
			if !oneSubschemaAllowsStrict(child) {
				return false
			}
		case kwAnyOf:
			branches, isList := child.([]any)
			if !isList {
				return false
			}
			for _, branch := range branches {
				if !oneSubschemaAllowsStrict(branch) {
					return false
				}
			}
		}
	}
	return true
}

// oneSubschemaAllowsStrict asks the question of a value that must be a single
// schema. Anything else — a bool, a string, a list — is a shape this walk does
// not model, and so is refused.
//
//craft:ignore naked-any the caller holds a decoded JSON Schema keyword whose value is object, array or scalar by turns, so the assertion has to happen somewhere and here is where it decides something
func oneSubschemaAllowsStrict(value any) bool {
	sub, isObject := value.(map[string]any)
	if !isObject {
		return false
	}
	return schemaNode(sub).allowsStrict()
}

// propertiesAllowStrict walks the `properties` map, whose KEYS are the
// caller's own property names and whose VALUES are schemas.
//
//craft:ignore naked-any same decoded-keyword reason as oneSubschemaAllowsStrict
func propertiesAllowStrict(value any) bool {
	byName, isMap := value.(map[string]any)
	if !isMap {
		return false
	}
	for _, sub := range byName {
		if !oneSubschemaAllowsStrict(sub) {
			return false
		}
	}
	return true
}

// isClosed holds one object to the two rules: additionalProperties is exactly
// false, and required names every declared property and nothing else.
//
// Both directions, because the endpoint refuses both: a property missing from
// required, and a required naming a property that does not exist.
func (n schemaNode) isClosed() bool {
	if allowsExtra, declared := n[kwAdditionalProperties].(bool); !declared || allowsExtra {
		return false
	}
	declaredProps, hasProps := n[kwProperties]
	if !hasProps {
		// No properties to omit, so nothing can be missing from required.
		return true
	}
	props, isMap := declaredProps.(map[string]any)
	if !isMap {
		return false
	}
	names, isList := n["required"].([]any)
	if !isList {
		// A `required` that is absent or not a list names nothing, which is
		// closed only when there is nothing to name.
		return len(props) == 0
	}
	// A SET, not a count: `required: ["a","a"]` against two properties matches
	// on length while leaving the second one unrequired.
	required := make(map[string]bool, len(names))
	for _, name := range names {
		text, isText := name.(string)
		if !isText {
			return false
		}
		if _, declared := props[text]; !declared {
			return false
		}
		required[text] = true
	}
	return len(required) == len(props)
}
