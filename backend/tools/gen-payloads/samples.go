// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
)

// maxSampleDepth stops a recursive schema from recursing forever; a sample
// only has to show a template author the shape.
const maxSampleDepth = 6

// eventSamples renders samplesVar: each event type's payload schema as a
// sample `data` object, built from the schema alone so every subscribable
// type has one and none can drift from the contract.
func eventSamples(spec *openapi3.T, samplesVar string) (string, error) {
	samples := map[string]string{}
	for name, ref := range spec.Components.Schemas {
		if ref == nil || ref.Value == nil {
			continue
		}
		eventType, ok, err := stringExtension(ref.Value.Extensions, "x-event-type", name)
		if err != nil {
			return "", err
		}
		if !ok {
			continue
		}
		raw, err := json.Marshal(sampleValue(ref.Value, "", 0))
		if err != nil {
			return "", fmt.Errorf("sample for %s: %w", name, err)
		}
		samples[eventType] = string(raw)
	}
	types := make([]string, 0, len(samples))
	for t := range samples {
		types = append(types, t)
	}
	sort.Strings(types)
	var b strings.Builder
	b.WriteString("// " + samplesVar + " maps every event type carrying a PublicEvent<Event>\n")
	b.WriteString("// schema to a sample of its data, built from the schema's properties.\n")
	b.WriteString("var " + samplesVar + " = map[string]string{\n")
	for _, t := range types {
		fmt.Fprintf(&b, "\t%q: %q,\n", t, samples[t])
	}
	b.WriteString("}\n")
	return b.String(), nil
}

func sampleValue(s *openapi3.Schema, name string, depth int) any {
	if len(s.Enum) > 0 {
		for _, v := range s.Enum {
			if v != nil {
				return v
			}
		}
	}
	for _, alt := range append(append([]*openapi3.SchemaRef{}, s.OneOf...), s.AnyOf...) {
		if alt != nil && alt.Value != nil {
			return sampleValue(alt.Value, name, depth)
		}
	}
	switch {
	case s.Type.Is(openapi3.TypeObject) || len(s.Properties) > 0:
		obj := map[string]any{}
		if depth >= maxSampleDepth {
			return obj
		}
		for prop, ref := range s.Properties {
			if ref != nil && ref.Value != nil {
				obj[prop] = sampleValue(ref.Value, prop, depth+1)
			}
		}
		return obj
	case s.Type.Is(openapi3.TypeArray):
		if s.Items == nil || s.Items.Value == nil || depth >= maxSampleDepth {
			return []any{}
		}
		return []any{sampleValue(s.Items.Value, name, depth+1)}
	case s.Type.Is(openapi3.TypeInteger):
		return 1
	case s.Type.Is(openapi3.TypeNumber):
		return 1.5
	case s.Type.Is(openapi3.TypeBoolean):
		return true
	}
	switch s.Format {
	case "uuid":
		return "0190a7f2-0000-7000-8000-000000000010"
	case "date-time":
		return "2026-10-14T09:12:00Z"
	case "date":
		return "2026-10-14"
	case "email":
		return "ada@example.com"
	case "uri":
		return "https://example.com"
	}
	if name == "" {
		return "sample"
	}
	return "Sample " + strings.ReplaceAll(name, "_", " ")
}
