// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

// Fitting a response schema to what Anthropic's structured output enforces.
//
// output_config.format constrains generation to a JSON Schema, but only to a
// subset of the vocabulary: every object closed with additionalProperties
// false, no numeric bounds, no string length, no array bound beyond a minItems
// of 0 or 1, no pattern, and string formats from a short list. A schema outside
// the subset is answered with a 400 that fails the call, so the fit is decided
// before sending, the way Anthropic's own SDKs decide it.
//
// An unenforceable bound moves into the description, where the model reads it
// even though the decoder cannot hold it, and an object that declares its
// properties is closed. The caller's validator still holds the whole original
// schema, so a moved bound is checked after generation rather than lost. A
// shape no fit can express — a free-form object, a type union, a reference —
// is not sent at all. Either downgrade is reported on the Response, so the call
// record says which answers generation did not fully hold.
//
// The shared schema builder emits nothing outside the subset, so its schemas go
// verbatim; what reaches the fit is a hand-written schema, or one an extension
// supplies.

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
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
// and "" when it already fits; a fitted copy and "" when fitting only closed an
// object, which enforces more rather than less; a fitted copy and
// model.SchemaRelaxed when a bound had to move into a description; nil and
// model.SchemaDropped when no fit exists.
func anthropicOutputSchema(raw json.RawMessage) (json.RawMessage, string) {
	if len(raw) == 0 {
		return nil, ""
	}
	var fitter anthropicSchemaFitter
	fitted, err := fitter.fitSubschema(raw)
	switch {
	case err != nil:
		return nil, model.SchemaDropped
	case fitter.relaxed:
		return fitted, model.SchemaRelaxed
	case fitter.changed:
		return fitted, ""
	default:
		return raw, ""
	}
}

// anthropicSchemaFitter walks one schema. changed says the fitted copy differs
// from what was written, relaxed that some bound in it is no longer enforced —
// two facts, because closing an object is the first without the second.
type anthropicSchemaFitter struct{ changed, relaxed bool }

func (f *anthropicSchemaFitter) fitSubschema(raw json.RawMessage) (json.RawMessage, error) {
	node, err := decodeSchemaObject(raw)
	if err != nil {
		return nil, err
	}
	fitted, err := f.fit(node)
	if err != nil {
		return nil, err
	}
	return fitted.encode()
}

// fit is one node: every keyword kept, fitted or moved, in the order it was
// written; then the node closed if it is an object; then the moved keywords
// written into its description.
func (f *anthropicSchemaFitter) fit(node schemaObject) (schemaObject, error) {
	out := make(schemaObject, 0, len(node))
	var moved schemaObject
	for _, member := range node {
		fitted, keep, err := f.fitKeyword(member.key, member.value)
		switch {
		case err != nil:
			return nil, err
		case keep:
			out = append(out, schemaMember{key: member.key, value: fitted})
		default:
			moved = append(moved, member)
		}
	}
	out, err := f.close(node, out)
	if err != nil || len(moved) == 0 {
		return out, err
	}
	f.changed, f.relaxed = true, true
	description, err := describeMoved(out.get("description"), moved)
	if err != nil {
		return nil, err
	}
	return out.with("description", description), nil
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
// false the decoder requires. The model may then no longer add a key the
// schema would have tolerated, which enforces more than was written rather
// than less, so it changes the schema without relaxing it. An object with no
// properties to close over is a free-form map, which no closed object can
// stand in for.
func (f *anthropicSchemaFitter) close(node, out schemaObject) (schemaObject, error) {
	if _, declared := node.lookup("additionalProperties"); declared {
		return out, nil
	}
	var name string
	isObject := json.Unmarshal(node.get("type"), &name) == nil && name == "object"
	props, hasProps := node.lookup("properties")
	if !isObject && !hasProps {
		return out, nil
	}
	if !hasProps {
		return nil, fmt.Errorf("%w: an object with no declared properties", errSchemaIneligible)
	}
	if declared, err := decodeSchemaObject(props); err != nil || len(declared) == 0 {
		return nil, fmt.Errorf("%w: an object with no declared properties", errSchemaIneligible)
	}
	f.changed = true
	return out.with("additionalProperties", json.RawMessage("false")), nil
}

// fitProperties fits every schema in a `properties` map, whose keys are the
// caller's property names rather than keywords, keeping them in the order the
// caller wrote them: a model fills fields in the order it is shown them.
func (f *anthropicSchemaFitter) fitProperties(value json.RawMessage) (json.RawMessage, bool, error) {
	byName, err := decodeSchemaObject(value)
	if err != nil {
		return nil, false, err
	}
	fitted := make(schemaObject, 0, len(byName))
	for _, property := range byName {
		one, err := f.fitSubschema(property.value)
		if err != nil {
			return nil, false, err
		}
		fitted = append(fitted, schemaMember{key: property.key, value: one})
	}
	out, err := fitted.encode()
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
// spelling Anthropic's SDK uses — "{minLength: 1, maxLength: 998}" after a
// blank line — in the order they were written.
func describeMoved(description json.RawMessage, moved schemaObject) (json.RawMessage, error) {
	bounds := make([]string, 0, len(moved))
	for _, member := range moved {
		bounds = append(bounds, member.key+": "+string(bytes.TrimSpace(member.value)))
	}
	text := "{" + strings.Join(bounds, ", ") + "}"
	var existing string
	if json.Unmarshal(description, &existing) == nil && existing != "" {
		text = existing + "\n\n" + text
	}
	out, err := json.Marshal(text)
	if err != nil {
		return nil, fmt.Errorf("ai: anthropic: encode a fitted description: %w", err)
	}
	return out, nil
}

// schemaMember is one key of a schema object and its undecoded value.
type schemaMember struct {
	key   string
	value json.RawMessage
}

// schemaObject is a JSON object decoded with its key order intact, which a Go
// map would sort away. The order is load-bearing in `properties` — a model
// writes fields in the order a schema lists them, and schema.Record keeps its
// caller's order for that reason — so a fitted schema must list them as the
// caller did.
type schemaObject []schemaMember

func decodeSchemaObject(raw json.RawMessage) (schemaObject, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	if open, err := dec.Token(); err != nil || open != json.Delim('{') {
		return nil, fmt.Errorf("%w: a subschema is not an object", errSchemaIneligible)
	}
	var out schemaObject
	for dec.More() {
		token, err := dec.Token()
		if err != nil {
			return nil, fmt.Errorf("%w: %w", errSchemaIneligible, err)
		}
		key, isKey := token.(string)
		if !isKey {
			return nil, fmt.Errorf("%w: an object key that is not a string", errSchemaIneligible)
		}
		var value json.RawMessage
		if err := dec.Decode(&value); err != nil {
			return nil, fmt.Errorf("%w: %w", errSchemaIneligible, err)
		}
		out = append(out, schemaMember{key: key, value: value})
	}
	if _, err := dec.Token(); err != nil {
		return nil, fmt.Errorf("%w: %w", errSchemaIneligible, err)
	}
	return out, nil
}

func (o schemaObject) lookup(key string) (json.RawMessage, bool) {
	for _, member := range o {
		if member.key == key {
			return member.value, true
		}
	}
	return nil, false
}

// get is lookup for a caller that reads an absent key as a nil value.
func (o schemaObject) get(key string) json.RawMessage {
	if value, present := o.lookup(key); present {
		return value
	}
	return nil
}

// with sets key to value: in place when the key is already there, appended
// after the caller's keys when it is not.
func (o schemaObject) with(key string, value json.RawMessage) schemaObject {
	for i, member := range o {
		if member.key == key {
			o[i].value = value
			return o
		}
	}
	return append(o, schemaMember{key: key, value: value})
}

// encode writes the object back in its own key order.
func (o schemaObject) encode() (json.RawMessage, error) {
	var buf bytes.Buffer
	buf.WriteByte('{')
	for i, member := range o {
		if i > 0 {
			buf.WriteByte(',')
		}
		key, err := json.Marshal(member.key)
		if err != nil {
			return nil, fmt.Errorf("ai: anthropic: encode a schema key: %w", err)
		}
		buf.Write(key)
		buf.WriteByte(':')
		buf.Write(member.value)
	}
	buf.WriteByte('}')
	var compact bytes.Buffer
	if err := json.Compact(&compact, buf.Bytes()); err != nil {
		return nil, fmt.Errorf("ai: anthropic: encode a fitted schema: %w", err)
	}
	return compact.Bytes(), nil
}
