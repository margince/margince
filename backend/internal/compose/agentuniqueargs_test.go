// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// A tool and the routes it runs as take one list under one rule. When a route's
// request body declares a list uniqueItems, the tool's argument of that name
// declares it too. Registry.Invoke then refuses a repeat on the tool door. The
// sweep reads every tool's OpenAPIOp and the contract's body schemas.
func TestEveryToolListArgumentIsUniqueWhereItsRouteSaysSo(t *testing.T) {
	routeLists := uniqueBodyListsByOperation(t)
	registry := NewRegistry(nil, SendPath{})
	checked := 0
	for _, spec := range registry.Specs() {
		if _, isRouter := registry.RouteTargets(spec.Name); isRouter {
			continue
		}
		own := toolListUniqueness(t, spec.Name, spec.InputSchema)
		for id := range strings.SplitSeq(spec.OpenAPIOp, "/") {
			for name := range routeLists[id] {
				declared, takes := own[name]
				if !takes {
					continue
				}
				checked++
				if !declared {
					t.Errorf("%s: argument %q allows repeats, but %s declares it uniqueItems; "+
						"add \"uniqueItems\":true to the tool's schema", spec.Name, name, id)
				}
			}
		}
	}
	if checked == 0 {
		t.Fatal("no tool takes a list its route declares uniqueItems; the sweep checked nothing")
	}
}

// toolListUniqueness maps each top-level argument to whether it declares
// uniqueItems.
func toolListUniqueness(t *testing.T, tool string, schema json.RawMessage) map[string]bool {
	t.Helper()
	var shape struct {
		Properties map[string]struct {
			UniqueItems bool `json:"uniqueItems"` //nolint:tagliatelle // JSON Schema's keyword, not ours to rename
		} `json:"properties"`
	}
	if err := json.Unmarshal(schema, &shape); err != nil {
		t.Fatalf("%s: unreadable input schema: %v", tool, err)
	}
	out := make(map[string]bool, len(shape.Properties))
	for name, prop := range shape.Properties {
		out[name] = prop.UniqueItems
	}
	return out
}

// contractSchema is the part of a contract schema this sweep reads.
type contractSchema struct {
	Ref         string                    `yaml:"$ref"`
	UniqueItems bool                      `yaml:"uniqueItems"` //nolint:tagliatelle // JSON Schema's keyword, not ours to rename
	Properties  map[string]contractSchema `yaml:"properties"`
}

// uniqueBodyListsByOperation names, per operationId, the top-level request body
// properties that declare uniqueItems, with $refs resolved.
func uniqueBodyListsByOperation(t *testing.T) map[string]map[string]bool {
	t.Helper()
	src, err := os.ReadFile("../../api/crm.yaml")
	if err != nil {
		t.Fatalf("reading the contract: %v", err)
	}
	var doc struct {
		Paths      map[string]map[string]yaml.Node `yaml:"paths"`
		Components struct {
			Schemas map[string]contractSchema `yaml:"schemas"`
		} `yaml:"components"`
	}
	if err := yaml.Unmarshal(src, &doc); err != nil {
		t.Fatalf("parsing the contract: %v", err)
	}
	resolve := func(s contractSchema) contractSchema {
		if name, ok := strings.CutPrefix(s.Ref, "#/components/schemas/"); ok {
			return doc.Components.Schemas[name]
		}
		return s
	}
	out := map[string]map[string]bool{}
	for _, item := range doc.Paths {
		for _, method := range httpMethods {
			node, present := item[method]
			if !present {
				continue
			}
			//nolint:tagliatelle // OpenAPI's own field names are camelCase.
			var op struct {
				ID          string `yaml:"operationId"`
				RequestBody struct {
					Content map[string]struct {
						Schema contractSchema `yaml:"schema"`
					} `yaml:"content"`
				} `yaml:"requestBody"`
			}
			if err := node.Decode(&op); err != nil {
				t.Fatalf("%s: %v", method, err)
			}
			body := resolve(op.RequestBody.Content["application/json"].Schema)
			for name, prop := range body.Properties {
				if resolve(prop).UniqueItems {
					if out[op.ID] == nil {
						out[op.ID] = map[string]bool{}
					}
					out[op.ID][name] = true
				}
			}
		}
	}
	return out
}
