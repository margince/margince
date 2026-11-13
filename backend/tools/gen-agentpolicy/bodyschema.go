// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// maxBodyDepth bounds how deep a body schema's $refs are followed. A deeper or
// recursive member is published as a bare object; the surface reads no $ref.
const maxBodyDepth = 6

// proseMembers are left out of a published body schema: the agent reads the
// shape, and the words already reach it through the operation's summary.
var proseMembers = map[string]bool{"description": true, "example": true, "examples": true, "x-go-type": true}

// bodySchema is a route's JSON request body schema, $refs resolved, as one
// compact JSON document; empty when the route takes no JSON body. humanOnly
// names the top-level members the schema leaves out because a human alone sets them.
func bodySchema(requestBody *yaml.Node, schemas map[string]any) (schema string, humanOnly []string, err error) {
	if requestBody == nil || requestBody.Kind == 0 {
		return "", nil, nil
	}
	var body struct {
		Content map[string]struct {
			Schema any `yaml:"schema"`
		} `yaml:"content"`
	}
	if err := requestBody.Decode(&body); err != nil {
		return "", nil, err
	}
	media, ok := body.Content["application/json"]
	if !ok {
		return "", nil, nil
	}
	resolved, err := resolveRefs(media.Schema, schemas, 0, map[string]bool{})
	if err != nil {
		return "", nil, err
	}
	humanOnly, err = withoutHumanOnly(resolved)
	if err != nil {
		return "", nil, err
	}
	out, err := json.Marshal(unmarked(resolved))
	return string(out), humanOnly, err
}

// uploadField names the one binary field of a route's multipart body, and is
// empty for a route with no such body. The door sends that one field, so a
// multipart body with any other field is refused rather than sent short.
func uploadField(requestBody *yaml.Node) (string, error) {
	if requestBody == nil || requestBody.Kind == 0 {
		return "", nil
	}
	var body struct {
		Content map[string]struct {
			Schema struct {
				Properties map[string]struct {
					Format string `yaml:"format"`
				} `yaml:"properties"`
			} `yaml:"schema"`
		} `yaml:"content"`
	}
	if err := requestBody.Decode(&body); err != nil {
		return "", err
	}
	media, ok := body.Content["multipart/form-data"]
	if !ok {
		return "", nil
	}
	if len(media.Schema.Properties) != 1 {
		return "", fmt.Errorf("served_by: route uploads one file, and this multipart body has %d fields", len(media.Schema.Properties))
	}
	for name, field := range media.Schema.Properties {
		if field.Format == "binary" {
			return name, nil
		}
	}
	return "", errors.New("served_by: route uploads a binary field, and this multipart body has none")
}

// fileAnswerSchema is the answer of a route that returns a file's bytes.
const fileAnswerSchema = `{"type":"object","required":["content_type","size_bytes","content_base64"],"properties":{` +
	`"filename":{"type":"string"},"content_type":{"type":"string"},"size_bytes":{"type":"integer"},` +
	`"content_base64":{"type":"string"}}}`

// jsonMedia is the content of a response whose media type is JSON of any kind,
// such as application/schema+json. The door passes each on as JSON.
func jsonMedia[M any](content map[string]M) (M, bool) {
	if plain, ok := content["application/json"]; ok {
		return plain, true
	}
	for media, value := range content {
		if strings.Contains(media, "json") {
			return value, true
		}
	}
	var none M
	return none, false
}

// withoutRequired drops every nested required list from an answer's schema. The
// surface carries one type per member, so a required member that may be null has
// no true shape here. A member a route leaves out would fail a check that withholds
// the whole structured answer from the client.
//
//craft:ignore naked-any a JSON schema is walked as decoded JSON, whatever its shape
func withoutRequired(node any) any {
	schema, isSchema := node.(map[string]any)
	if !isSchema {
		return node
	}
	delete(schema, "required")
	for key, member := range schema {
		// A key under properties names a field, which may be called required.
		if named, isProperties := member.(map[string]any); key == "properties" && isProperties {
			for field, fieldSchema := range named {
				named[field] = withoutRequired(fieldSchema)
			}
			continue
		}
		schema[key] = withoutRequired(member)
	}
	return schema
}

// answerSchema is the JSON schema of a route's first 2xx answer, $refs
// resolved. An array answer reads as the {items} object the door wraps it in.
// A route that answers a file reads as the door's file answer. Any other route
// with no JSON answer reads as the {status} the door answers.
func answerSchema(responses yaml.Node, schemas map[string]any) (string, error) {
	var byStatus map[string]struct {
		Content map[string]mediaSchema `yaml:"content"`
	}
	if responses.Kind != 0 {
		if err := responses.Decode(&byStatus); err != nil {
			return "", err
		}
	}
	codes := make([]string, 0, len(byStatus))
	for code := range byStatus {
		codes = append(codes, code)
	}
	sort.Strings(codes)
	for _, code := range codes {
		media, ok := jsonMedia(byStatus[code].Content)
		if strings.HasPrefix(code, "2") && !ok && len(byStatus[code].Content) > 0 {
			return fileAnswerSchema, nil
		}
		if !strings.HasPrefix(code, "2") || !ok {
			continue
		}
		resolved, err := resolveRefs(media.Schema, schemas, 0, map[string]bool{})
		if err != nil {
			return "", err
		}
		if shape, isMap := resolved.(map[string]any); isMap && shape["type"] == "array" {
			resolved = map[string]any{"type": "object", "properties": map[string]any{"items": shape}}
		}
		out, err := json.Marshal(withoutRequired(answered(resolved)))
		return string(out), err
	}
	return `{"type":"object","properties":{"status":{"type":"integer"}}}`, nil
}

// mediaSchema is one media type of a body or answer, read for its schema.
type mediaSchema struct {
	Schema any `yaml:"schema"`
}

// nullableMarker flags a member whose contract type admits null, which the
// surface's one-type schema cannot say. answered and unmarked strip it.
const nullableMarker = "x-nullable"

// answered is a resolved answer schema with each amount's display member
// declared and no marker left. withoutRequired then drops its required lists.
//
//craft:ignore naked-any a JSON schema is walked as decoded YAML, whatever its shape
func answered(node any) any {
	switch v := node.(type) {
	case map[string]any:
		props, _ := v["properties"].(map[string]any)
		withDisplays(props)
		delete(v, nullableMarker)
		delete(v, humanOnlyMarker)
		for _, member := range v {
			answered(member)
		}
	case []any:
		for _, member := range v {
			answered(member)
		}
	}
	return node
}

// withDisplays declares the `<name>_display` member the tool surface writes
// beside each `<name>_minor` of an object that names its currency. It reads the
// rule the surface writes it by, so a closed answer admits it.
//
//craft:ignore naked-any a JSON schema is walked as decoded YAML, whatever its shape
func withDisplays(props map[string]any) {
	if !slices.ContainsFunc(values.AmountCurrencyMembers(), func(member string) bool { return props[member] != nil }) {
		return
	}
	for name := range props {
		if display, isMinor := values.DisplayMemberOf(name); isMinor && props[display] == nil {
			props[display] = map[string]any{"type": "string"}
		}
	}
}

// unmarked is a resolved body schema with no marker left in it. A body's
// nullable member is still one the route requires the caller to send.
//
//craft:ignore naked-any a JSON schema is walked as decoded YAML, whatever its shape
func unmarked(node any) any {
	switch v := node.(type) {
	case map[string]any:
		delete(v, nullableMarker)
		for _, member := range v {
			unmarked(member)
		}
	case []any:
		for _, member := range v {
			unmarked(member)
		}
	}
	return node
}

//craft:ignore naked-any a JSON schema is walked as decoded YAML, whatever its shape
func resolveRefs(node any, schemas map[string]any, depth int, open map[string]bool) (any, error) {
	switch v := node.(type) {
	case map[string]any:
		if ref, ok := v["$ref"].(string); ok {
			name := strings.TrimPrefix(ref, "#/components/schemas/")
			target, known := schemas[name]
			if !known {
				return nil, fmt.Errorf("body names schema %s, which components.schemas does not declare", ref)
			}
			if depth >= maxBodyDepth || open[name] {
				return map[string]any{"type": "object"}, nil
			}
			open[name] = true
			defer delete(open, name)
			return resolveRefs(target, schemas, depth+1, open)
		}
		if flat, combined, err := flattenCombinator(v, schemas, depth, open); combined {
			return flat, err
		}
		out := make(map[string]any, len(v))
		for key, member := range v {
			if proseMembers[key] {
				continue
			}
			if named, isProperties := member.(map[string]any); key == "properties" && isProperties {
				resolved, err := resolveProperties(named, schemas, depth, open)
				if err != nil {
					return nil, err
				}
				out[key] = resolved
				continue
			}
			if types, isList := member.([]any); key == "type" && isList {
				out[key] = firstNonNull(types)
				if slices.Contains(types, any("null")) {
					out[nullableMarker] = true
				}
				continue
			}
			if members, isList := member.([]any); key == "enum" && isList {
				if kept, scalar := enumValues(members); scalar {
					out[key] = kept
				}
				continue
			}
			resolved, err := resolveRefs(member, schemas, depth, open)
			if err != nil {
				return nil, err
			}
			out[key] = resolved
		}
		return out, nil
	case []any:
		out := make([]any, len(v))
		for i, member := range v {
			resolved, err := resolveRefs(member, schemas, depth, open)
			if err != nil {
				return nil, err
			}
			out[i] = resolved
		}
		return out, nil
	}
	return node, nil
}

// resolveProperties resolves a `properties` map, whose keys are field names and
// so are never prose: a field may be called description.
//
//craft:ignore naked-any a JSON schema is walked as decoded YAML, whatever its shape
func resolveProperties(named map[string]any, schemas map[string]any, depth int, open map[string]bool) (map[string]any, error) {
	out := make(map[string]any, len(named))
	for field, member := range named {
		resolved, err := resolveRefs(member, schemas, depth, open)
		if err != nil {
			return nil, err
		}
		out[field] = resolved
	}
	return out, nil
}

// flattenCombinator reads allOf, oneOf and anyOf, which the agent surface
// refuses, as one plain schema. allOf merges its resolved members. oneOf and
// anyOf keep their first member that is not null.
//
//craft:ignore naked-any a JSON schema is walked as decoded YAML, whatever its shape
func flattenCombinator(node map[string]any, schemas map[string]any, depth int, open map[string]bool) (any, bool, error) {
	for _, key := range []string{"allOf", "oneOf", "anyOf"} {
		members, ok := node[key].([]any)
		if !ok {
			continue
		}
		merged := map[string]any{}
		for k, v := range node {
			if k != key {
				merged[k] = v
			}
		}
		for _, member := range members {
			if m, isMap := member.(map[string]any); isMap && m["type"] == "null" {
				merged[nullableMarker] = true
				continue
			}
			resolved, err := resolveRefs(member, schemas, depth, open)
			if err != nil {
				return nil, true, err
			}
			mergeInto(merged, resolved)
			if key != "allOf" {
				break
			}
		}
		out, err := resolveRefs(merged, schemas, depth, open)
		return out, true, err
	}
	return nil, false, nil
}

// mergeInto adds one resolved schema's members to another, uniting their
// properties and required lists.
//
//craft:ignore naked-any a JSON schema is walked as decoded YAML, whatever its shape
func mergeInto(into map[string]any, member any) {
	m, ok := member.(map[string]any)
	if !ok {
		return
	}
	for key, value := range m {
		switch key {
		case "properties":
			props, _ := into[key].(map[string]any)
			if props == nil {
				props = map[string]any{}
			}
			if more, ok := value.(map[string]any); ok {
				maps.Copy(props, more)
			}
			into[key] = props
		case "required":
			have, _ := into[key].([]any)
			more, _ := value.([]any)
			into[key] = append(have, more...)
		default:
			into[key] = value
		}
	}
}

// enumValues is an enum's members bar null, and whether all are words or
// numbers. Null leaves because the surface's schema names one type, and the
// nullable marker already says it.
//
//craft:ignore naked-any a JSON schema is walked as decoded YAML, whatever its shape
func enumValues(members []any) ([]any, bool) {
	kept := make([]any, 0, len(members))
	for _, member := range members {
		switch member.(type) {
		case nil:
		case string, int, int64, float64:
			kept = append(kept, member)
		default:
			return nil, false
		}
	}
	return kept, true
}

// firstNonNull reads a nullable type list (["string","null"]) as its one
// non-null member, the single type the agent surface's schemas carry.
//
//craft:ignore naked-any a JSON schema is walked as decoded YAML, whatever its shape
func firstNonNull(types []any) any {
	for _, t := range types {
		if name, ok := t.(string); ok && name != "null" {
			return name
		}
	}
	return "string"
}
