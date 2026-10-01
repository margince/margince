// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"encoding/json"
	"os"
	"testing"

	"gopkg.in/yaml.v3"
)

// The contract and the editor schema each spell the location rule as a
// pattern, and both must be vertexLocationShape exactly: a looser copy admits
// a location the parser refuses, a stricter one refuses one it serves.
//
// Every pattern on a field named location is collected, wherever it sits, and
// the count is pinned, so a copy moved or dropped fails instead of vanishing.
func TestEveryLocationPatternIsTheParsersShape(t *testing.T) {
	t.Parallel()
	for _, doc := range []struct {
		path   string
		decode func([]byte, any) error
		copies int
	}{
		{"../../../api/crm.yaml", yaml.Unmarshal, 2},
		{"../../../../config/margince.schema.json", json.Unmarshal, 2},
	} {
		raw, err := os.ReadFile(doc.path)
		if err != nil {
			t.Fatal(err)
		}
		var tree any
		if err := doc.decode(raw, &tree); err != nil {
			t.Fatalf("%s: %v", doc.path, err)
		}
		patterns := locationPatterns(tree)
		if len(patterns) != doc.copies {
			t.Errorf("%s carries %d location pattern(s), want %d — a copy was added or lost; update this count only after checking it", doc.path, len(patterns), doc.copies)
		}
		for _, p := range patterns {
			if p != vertexLocationShape.String() {
				t.Errorf("%s: location pattern %q, want vertexLocationShape %q", doc.path, p, vertexLocationShape.String())
			}
		}
	}
}

// locationPatterns finds a `location` property's pattern and a parameter
// named location's schema pattern, anywhere in a decoded document.
//
//craft:ignore naked-any a decoded YAML or JSON document has no static shape; the walk type-switches every node
func locationPatterns(node any) []string {
	var found []string
	switch n := node.(type) {
	case map[string]any:
		if property, ok := n["location"].(map[string]any); ok {
			found = appendPattern(found, property)
		}
		if n["name"] == "location" {
			if schema, ok := n["schema"].(map[string]any); ok {
				found = appendPattern(found, schema)
			}
		}
		for _, child := range n {
			found = append(found, locationPatterns(child)...)
		}
	case []any:
		for _, child := range n {
			found = append(found, locationPatterns(child)...)
		}
	}
	return found
}

func appendPattern(found []string, schema map[string]any) []string {
	if pattern, ok := schema["pattern"].(string); ok {
		return append(found, pattern)
	}
	return found
}
