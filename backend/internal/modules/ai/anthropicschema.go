// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

// Fitting a response schema to what Anthropic's structured output enforces.
//
// output_config.format constrains generation to a JSON Schema, but only to a
// subset of the vocabulary: every object closed with additionalProperties
// false, no numeric bounds, no string length, no array bound beyond a minItems
// of 0 or 1, no pattern, and string formats from a short list. A schema outside
// the subset is answered with a 400, and the adapter used to find that out by
// sending it: any 400 on a schema-carrying request was retried with the schema
// cleared, so a schema this tree writes every day (a reply draft's maxLength, a
// score's minimum) silently bought an unconstrained completion — and a 400 for
// any other reason bought a second call that failed the same way.
//
// The fit is decided before sending, the way Anthropic's own SDKs do it: an
// unenforceable bound moves into the description, where the model reads it
// even though the decoder cannot hold it, and an object that declares its
// properties is closed. The caller's validator still holds the whole original
// schema, so a moved bound is checked after generation rather than lost. A
// shape no fit can express — a free-form object, a type union, a reference —
// is not sent at all. Either downgrade is reported on the Response, so the call
// record says which answers generation did not fully hold.

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// errSchemaIneligible is a response schema Anthropic's structured output cannot
// express in any form, so the request goes without one.
var errSchemaIneligible = errors.New("ai: anthropic: response schema cannot be enforced")

// anthropicSchemaTypes are the JSON types the decoder accepts, one per `type`.
var anthropicSchemaTypes = map[string]bool{
	"object": true, "array": true, "string": true, "integer": true, "number": true, "boolean": true, "null": true,
}

// anthropicStringFormats are the `format` values the decoder enforces.
var anthropicStringFormats = map[string]bool{
	"date-time": true, "time": true, "date": true, "duration": true, "email": true,
	"hostname": true, "uri": true, "ipv4": true, "ipv6": true, "uuid": true,
}

// anthropicUnenforcedKeywords are the validation keywords the decoder cannot
// hold. Each moves into its node's description rather than failing the schema:
// the bound still reaches the model, and the caller's validator still checks it.
var anthropicUnenforcedKeywords = map[string]bool{
	"minimum": true, "maximum": true, "exclusiveMinimum": true, "exclusiveMaximum": true, "multipleOf": true,
	"minLength": true, "maxLength": true, "pattern": true,
	"maxItems": true, "uniqueItems": true, "minProperties": true, "maxProperties": true,
}

// anthropicOutputSchema is what to send as output_config.format for a
// request's response schema, and the downgrade that cost: the schema verbatim
// and "" when it already fits, a fitted copy and model.SchemaRelaxed when a
// bound had to move into a description, and nil with model.SchemaDropped when
// no fit exists.
func anthropicOutputSchema(raw json.RawMessage) (json.RawMessage, string) {
	if len(raw) == 0 {
		return nil, ""
	}
	fitted, relaxed, err := fitAnthropicSchema(raw)
	switch {
	case err != nil:
		return nil, model.SchemaDropped
	case relaxed:
		return fitted, model.SchemaRelaxed
	default:
		return raw, ""
	}
}

// fitAnthropicSchema returns raw fitted to the decoder's subset, and whether
// fitting changed anything the decoder would have enforced as written.
func fitAnthropicSchema(raw json.RawMessage) (json.RawMessage, bool, error) {
	var fitter anthropicSchemaFitter
	fitted, err := fitter.fitRaw(raw)
	if err != nil {
		return nil, false, err
	}
	out, err := json.Marshal(fitted)
	if err != nil {
		return nil, false, fmt.Errorf("ai: anthropic: encode fitted schema: %w", err)
	}
	return out, fitter.relaxed, nil
}

// anthropicSchemaFitter walks one schema, remembering whether any node had to
// give something up.
type anthropicSchemaFitter struct{ relaxed bool }

func (f *anthropicSchemaFitter) fitRaw(raw json.RawMessage) (map[string]json.RawMessage, error) {
	var node map[string]json.RawMessage
	if err := json.Unmarshal(raw, &node); err != nil {
		return nil, fmt.Errorf("%w: a subschema is not an object", errSchemaIneligible)
	}
	return f.fit(node)
}

// fit is one node: every keyword kept, fitted or moved, then the node closed if
// it is an object, then the moved keywords written into its description.
func (f *anthropicSchemaFitter) fit(node map[string]json.RawMessage) (map[string]json.RawMessage, error) {
	out := make(map[string]json.RawMessage, len(node))
	moved := map[string]json.RawMessage{}
	for key, value := range node {
		fitted, keep, err := f.fitKeyword(key, value)
		switch {
		case err != nil:
			return nil, err
		case keep:
			out[key] = fitted
		default:
			moved[key] = value
		}
	}
	if err := f.close(node, out); err != nil {
		return nil, err
	}
	if len(moved) > 0 {
		f.relaxed = true
		out["description"] = describeMoved(out["description"], moved)
	}
	return out, nil
}

// fitKeyword answers one keyword: kept (possibly with a fitted value), moved
// into the description (keep false, no error), or ineligible.
//
// An allowlist, like schemastrict.go's and for its reason: a keyword this walk
// has never seen is a keyword it cannot vouch for, and vouching wrongly is a
// 400 on a call that would have been answered without the schema.
func (f *anthropicSchemaFitter) fitKeyword(key string, value json.RawMessage) (json.RawMessage, bool, error) {
	switch key {
	case "description", "title", "default", "const", "required":
		return value, true, nil
	case "type":
		var name string
		if json.Unmarshal(value, &name) != nil || !anthropicSchemaTypes[name] {
			return nil, false, fmt.Errorf("%w: type %s", errSchemaIneligible, value)
		}
		return value, true, nil
	case "enum":
		return value, true, scalarEnum(value)
	case "additionalProperties":
		// Only a closed object is enforceable; one that admits extra keys on
		// purpose cannot be closed without refusing the keys it asked for.
		if !bytes.Equal(bytes.TrimSpace(value), []byte("false")) {
			return nil, false, fmt.Errorf("%w: an object open to extra properties", errSchemaIneligible)
		}
		return value, true, nil
	case "properties":
		return f.fitProperties(value)
	case "items":
		fitted, err := f.fitSubschema(value)
		return fitted, err == nil, err
	case "anyOf", "allOf":
		return f.fitBranches(value)
	case "format":
		var format string
		return value, json.Unmarshal(value, &format) == nil && anthropicStringFormats[format], nil
	case "minItems":
		var least int
		return value, json.Unmarshal(value, &least) == nil && (least == 0 || least == 1), nil
	}
	if anthropicUnenforcedKeywords[key] {
		return nil, false, nil
	}
	return nil, false, fmt.Errorf("%w: keyword %q", errSchemaIneligible, key)
}

// close gives an object that declares properties the additionalProperties
// false the decoder requires. That is a real narrowing — the model may no
// longer add a key the schema would have tolerated — so it counts as relaxing
// the fit even though nothing the caller asked for is lost. An object with no
// properties to close over is a free-form map, which no closed object can
// stand in for.
func (f *anthropicSchemaFitter) close(node, out map[string]json.RawMessage) error {
	if _, declared := node["additionalProperties"]; declared {
		return nil
	}
	var name string
	isObject := json.Unmarshal(node["type"], &name) == nil && name == "object"
	props, hasProps := node["properties"]
	if !isObject && !hasProps {
		return nil
	}
	if !hasProps || bytes.Equal(bytes.TrimSpace(props), []byte("{}")) {
		return fmt.Errorf("%w: an object with no declared properties", errSchemaIneligible)
	}
	out["additionalProperties"] = json.RawMessage("false")
	f.relaxed = true
	return nil
}

// fitProperties fits every schema in a `properties` map, whose keys are the
// caller's property names rather than keywords.
func (f *anthropicSchemaFitter) fitProperties(value json.RawMessage) (json.RawMessage, bool, error) {
	var byName map[string]json.RawMessage
	if err := json.Unmarshal(value, &byName); err != nil {
		return nil, false, fmt.Errorf("%w: properties is not a map", errSchemaIneligible)
	}
	fitted := make(map[string]json.RawMessage, len(byName))
	for name, sub := range byName {
		one, err := f.fitSubschema(sub)
		if err != nil {
			return nil, false, err
		}
		fitted[name] = one
	}
	out, err := json.Marshal(fitted)
	return out, err == nil, err
}

// fitBranches fits every branch of an anyOf or allOf.
func (f *anthropicSchemaFitter) fitBranches(value json.RawMessage) (json.RawMessage, bool, error) {
	var branches []json.RawMessage
	if err := json.Unmarshal(value, &branches); err != nil {
		return nil, false, fmt.Errorf("%w: a union is not a list", errSchemaIneligible)
	}
	fitted := make([]json.RawMessage, 0, len(branches))
	for _, branch := range branches {
		one, err := f.fitSubschema(branch)
		if err != nil {
			return nil, false, err
		}
		fitted = append(fitted, one)
	}
	out, err := json.Marshal(fitted)
	return out, err == nil, err
}

func (f *anthropicSchemaFitter) fitSubschema(raw json.RawMessage) (json.RawMessage, error) {
	fitted, err := f.fitRaw(raw)
	if err != nil {
		return nil, err
	}
	out, err := json.Marshal(fitted)
	if err != nil {
		return nil, fmt.Errorf("ai: anthropic: encode fitted schema: %w", err)
	}
	return out, nil
}

// scalarEnum refuses an enum holding an object or an array, which the decoder
// does not take.
func scalarEnum(value json.RawMessage) error {
	var members []json.RawMessage
	if err := json.Unmarshal(value, &members); err != nil {
		return fmt.Errorf("%w: enum is not a list", errSchemaIneligible)
	}
	for _, member := range members {
		if trimmed := bytes.TrimSpace(member); len(trimmed) > 0 && (trimmed[0] == '{' || trimmed[0] == '[') {
			return fmt.Errorf("%w: an enum member that is not a scalar", errSchemaIneligible)
		}
	}
	return nil
}

// describeMoved appends the moved keywords to a node's description in the
// spelling Anthropic's SDK uses — "{maxLength: 998, minLength: 1}" after a
// blank line — sorted so one schema always fits to the same bytes and the
// result cache keys it once.
func describeMoved(description json.RawMessage, moved map[string]json.RawMessage) json.RawMessage {
	keys := make([]string, 0, len(moved))
	for key := range moved {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	bounds := make([]string, 0, len(keys))
	for _, key := range keys {
		bounds = append(bounds, key+": "+string(bytes.TrimSpace(moved[key])))
	}
	text := "{" + strings.Join(bounds, ", ") + "}"
	var existing string
	if json.Unmarshal(description, &existing) == nil && existing != "" {
		text = existing + "\n\n" + text
	}
	out, err := json.Marshal(text)
	if err != nil {
		// A Go string always marshals; the branch exists because the
		// signature says it can fail, and an empty description is the
		// harmless answer if it ever did.
		return json.RawMessage(`""`)
	}
	return out
}
