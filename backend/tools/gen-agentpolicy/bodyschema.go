// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"encoding/json"
	"fmt"
	"maps"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// maxBodyDepth bounds how deep a body schema's $refs are followed. A deeper or
// recursive member is published as a bare object; the surface reads no $ref.
const maxBodyDepth = 6

// proseMembers are left out of a published body schema: the agent reads the
// shape, and the words already reach it through the operation's summary.
var proseMembers = map[string]bool{"description": true, "example": true, "examples": true, "x-go-type": true}

// bodySchema is a route's JSON request body schema, $refs resolved, as one
// compact JSON document; empty when the route takes no JSON body.
func bodySchema(requestBody *yaml.Node, schemas map[string]any) (string, error) {
	if requestBody == nil || requestBody.Kind == 0 {
		return "", nil
	}
	var body struct {
		Content map[string]struct {
			Schema any `yaml:"schema"`
		} `yaml:"content"`
	}
	if err := requestBody.Decode(&body); err != nil {
		return "", err
	}
	media, ok := body.Content["application/json"]
	if !ok {
		return "", nil
	}
	resolved, err := resolveRefs(media.Schema, schemas, 0, map[string]bool{})
	if err != nil {
		return "", err
	}
	out, err := json.Marshal(resolved)
	return string(out), err
}

// fileAnswerSchema is the answer of a route that returns a file's bytes.
const fileAnswerSchema = `{"type":"object","required":["content_type","size_bytes","content_base64"],"properties":{` +
	`"filename":{"type":"string"},"content_type":{"type":"string"},"size_bytes":{"type":"integer"},` +
	`"content_base64":{"type":"string"}}}`

// answerSchema is the JSON schema of a route's first 2xx answer, $refs
// resolved. An array answer reads as the {items} object the door wraps it in.
// A route that answers a file reads as the door's file answer. Any other route
// with no JSON answer reads as the {status} the door answers.
func answerSchema(responses yaml.Node, schemas map[string]any) (string, error) {
	var byStatus map[string]struct {
		Content map[string]struct {
			Schema any `yaml:"schema"`
		} `yaml:"content"`
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
		media, ok := byStatus[code].Content["application/json"]
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
		out, err := json.Marshal(resolved)
		return string(out), err
	}
	return `{"type":"object","properties":{"status":{"type":"integer"}}}`, nil
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
