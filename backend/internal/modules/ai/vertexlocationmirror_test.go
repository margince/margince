// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"encoding/json"
	"os"
	"regexp"
	"testing"

	"gopkg.in/yaml.v3"
)

// The editor schema states each provider's location shape: the Vertex shape
// for gemini_vertex and the Azure region shape for azure_openai. A looser Vertex
// copy admits a location the parser refuses. The contract's location fields
// carry no pattern; only the Vertex-only query parameter keeps one.
//
// Every pattern on a field named location is collected, wherever it sits, and
// the count is pinned, so a copy moved or dropped fails instead of vanishing.
// azureRegionSchemaPattern is the editor's spelling of an Azure region name. The
// parser decides by lookup, so the pattern only has to admit every listed region.
const azureRegionSchemaPattern = "^[a-z][a-z0-9]{2,30}$"

func TestEveryAzureRegionFitsTheSchemaPattern(t *testing.T) {
	t.Parallel()
	pattern := regexp.MustCompile(azureRegionSchemaPattern)
	for region := range azureRegionCountries {
		if !pattern.MatchString(region) {
			t.Errorf("region %q is listed but the editor schema's pattern refuses it", region)
		}
	}
}

func TestEveryLocationPatternIsTheParsersShape(t *testing.T) {
	t.Parallel()
	for _, doc := range []struct {
		path   string
		decode func([]byte, any) error
		copies int
	}{
		{"../../../api/crm.yaml", yaml.Unmarshal, 1},
		{"../../../../config/margince.schema.json", json.Unmarshal, 6},
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
			if p != vertexLocationShape.String() && p != azureRegionSchemaPattern {
				t.Errorf("%s: location pattern %q, want vertexLocationShape %q or the Azure region pattern %q", doc.path, p, vertexLocationShape.String(), azureRegionSchemaPattern)
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
