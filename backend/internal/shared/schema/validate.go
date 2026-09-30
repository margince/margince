// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package schema

// ValidateJSON checks that value parses as JSON conforming to schemaJSON.
// This is deliberately NOT a general JSON Schema implementation: it walks
// only the restricted vocabulary the constructors in schema.go build (type,
// properties+required, additionalProperties, items, enum, and the value-or-null
// anyOf Optional writes) — the one
// shape every provider adapter in this codebase can both emit a
// ResponseSchema for and produce output against. A corpus's json_schema
// check exercises exactly that shape, so a bespoke draft-07/2020-12
// validator would cover ground this codebase never uses. Extend the
// vocabulary here only alongside a new Node constructor above.
import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strconv"
)

// ValidateJSON parses schemaJSON into a Node (the same wire shape Must
// renders) and reports the first way value's parsed JSON diverges from it.
// A nil error means value is valid JSON conforming to the schema.
func ValidateJSON(schemaJSON json.RawMessage, value string) error {
	var node Node
	if err := json.Unmarshal(schemaJSON, &node); err != nil {
		return fmt.Errorf("schema: parsing schema: %w", err)
	}
	// Numbers stay the literal the model wrote (json.Number), because an
	// integer is judged by its spelling: a float64 has already forgotten
	// whether it was written 3 or 3.0.
	dec := json.NewDecoder(bytes.NewReader([]byte(value)))
	dec.UseNumber()
	var data any
	if err := dec.Decode(&data); err != nil {
		return fmt.Errorf("schema: value is not valid JSON: %w", err)
	}
	// json.Unmarshal refuses anything after the value; a Decoder stops at the
	// value's end, so it is asked what follows, which must be nothing.
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return errors.New("schema: value is not valid JSON: something follows the value")
	}
	return node.validate(data, "$")
}

// validate checks v against n at path (a dotted/bracketed JSON-pointer-ish
// trail used only for the error message, e.g. "$.facts[2].name").
//
//craft:ignore naked-any v is whatever json.Unmarshal decoded — a JSON validator's input is arbitrary by nature
func (n Node) validate(v any, path string) error {
	switch n.Type {
	case typeObject:
		return n.validateObject(v, path)
	case typeArray:
		return n.validateArray(v, path)
	case typeString:
		s, ok := v.(string)
		if !ok {
			return fmt.Errorf("%s: want string, got %T", path, v)
		}
		if len(n.Enum) > 0 && !slices.Contains(n.Enum, s) {
			return fmt.Errorf("%s: %q is not one of %v", path, s, n.Enum)
		}
		return nil
	case typeNumber:
		if _, ok := v.(json.Number); !ok {
			return fmt.Errorf("%s: want number, got %T", path, v)
		}
		return nil
	case typeInteger:
		return validateInteger(v, path)
	case typeNull:
		if v != nil {
			return fmt.Errorf("%s: want null, got %T", path, v)
		}
		return nil
	case "":
		return n.validateAnyOf(v, path)
	default:
		return fmt.Errorf("%s: unsupported schema type %q", path, n.Type)
	}
}

// validateInteger admits a number written as a whole-number literal that fits
// a Go int, and nothing else. JSON Schema itself counts 3.0 as an integer, but
// every reader of an integer field here decodes it into an int, and
// encoding/json refuses 3.0, 1e2 and 1e300 there — so a validator that
// admitted them would pass the answer its reader then fails on. The validator
// holds the reader's rule, not the looser one.
//
//craft:ignore naked-any same decoded-JSON contract as validate
func validateInteger(v any, path string) error {
	number, ok := v.(json.Number)
	if !ok {
		return fmt.Errorf("%s: want integer, got %T", path, v)
	}
	if _, err := strconv.ParseInt(number.String(), 10, strconv.IntSize); err != nil {
		return fmt.Errorf("%s: want a whole number an int holds, got %s", path, number)
	}
	return nil
}

//craft:ignore naked-any same decoded-JSON contract as validate
func (n Node) validateObject(v any, path string) error {
	obj, ok := v.(map[string]any)
	if !ok {
		return fmt.Errorf("%s: want object, got %T", path, v)
	}
	for _, req := range n.Required {
		if _, present := obj[req]; !present {
			return fmt.Errorf("%s: missing required property %q", path, req)
		}
	}
	if n.AdditionalProperties != nil && !*n.AdditionalProperties {
		for key := range obj {
			if _, known := n.Properties[key]; !known {
				return fmt.Errorf("%s: unexpected property %q", path, key)
			}
		}
	}
	for key, sub := range n.Properties {
		val, present := obj[key]
		if !present {
			continue // absence of an optional property is not itself a violation
		}
		if err := sub.validate(val, path+"."+key); err != nil {
			return err
		}
	}
	return nil
}

//craft:ignore naked-any same decoded-JSON contract as validate
func (n Node) validateArray(v any, path string) error {
	arr, ok := v.([]any)
	if !ok {
		return fmt.Errorf("%s: want array, got %T", path, v)
	}
	if n.Items == nil {
		return nil
	}
	for i, item := range arr {
		if err := n.Items.validate(item, fmt.Sprintf("%s[%d]", path, i)); err != nil {
			return err
		}
	}
	return nil
}

// validateAnyOf admits v when any branch does. A node with neither a type nor
// branches describes nothing this package builds, and is refused rather than
// read as "anything goes".
//
//craft:ignore naked-any same decoded-JSON contract as validate
func (n Node) validateAnyOf(v any, path string) error {
	if len(n.AnyOf) == 0 {
		return fmt.Errorf("%s: schema node declares neither a type nor anyOf", path)
	}
	var refusals []error
	for _, branch := range n.AnyOf {
		err := branch.validate(v, path)
		if err == nil {
			return nil
		}
		refusals = append(refusals, err)
	}
	return fmt.Errorf("%s: matches no allowed shape: %w", path, errors.Join(refusals...))
}
