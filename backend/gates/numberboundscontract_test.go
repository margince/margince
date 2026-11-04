// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H3

package gates

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/margince/margince/backend/internal/shared/gatekit"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// untaggableBounds are the bounds no generated field can carry. Each bounded
// value is no struct field: an array item, a map value, a oneOf branch, or a
// named scalar. No decoder holds them, so each says who does instead.
var untaggableBounds = gatekit.Waive(map[string]string{
	"components.schemas.RowVersion":                                                        "readOnly: the server writes it and no request decodes it",
	"components.schemas.WorkingHours.properties.days.items":                                "a response; the request twin is SaveWorkingHoursRequest",
	"components.schemas.SaveWorkingHoursRequest.properties.days.items":                     "identity.validWorkingHours refuses a weekday outside 1..7",
	"components.schemas.ApprovalEvidence.properties.source_lines.items":                    "a response; the server numbers the lines",
	"components.schemas.AiDecisionSummary.properties.fallbacks.additionalProperties":       "a response map the server fills; no request decodes it",
	"components.schemas.ProviderCredits.properties.pools.additionalProperties":             "a response map the server fills; no request decodes it",
	"components.schemas.ProviderCategoryCost.properties.cost.additionalProperties":         "a response map the server fills; no request decodes it",
	"components.schemas.VoiceCorpusSummary.properties.register_words.additionalProperties": "a response map the server fills; no request decodes it",
	"components.schemas.AiOpenRouterProvider.properties.preferred_min_throughput.oneOf.0":  "ai.nonNegative refuses a negative one when routing is saved",
	"components.schemas.AiOpenRouterProvider.properties.preferred_max_latency.oneOf.0":     "ai.nonNegative refuses a negative one when routing is saved",
})

// Every `minimum` or `maximum` on a schema property or a request body field
// must reach a generated field tagged with the same bounds. The tags are all
// that datasource.RejectOutOfBoundsNumbers reads. A parameter's bound is held
// by the router (queryMinimumTable). An inline response is never decoded.
// Both sit outside this scan.
func TestEveryNumberBoundIsTaggedForTheDecoders(t *testing.T) {
	t.Parallel()
	defer untaggableBounds.AssertAllMatched(t)
	raw, err := os.ReadFile(contractPath)
	if err != nil {
		t.Fatal(err)
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatal(err)
	}
	declared := map[string]string{}
	boundDeclarations(&doc, nil, declared)
	if len(declared) < 100 {
		t.Fatalf("found %d bounded numbers in the contract; the scan no longer reads it", len(declared))
	}
	tagged := boundTaggedFields(t, "internal/contracts/api_gen.go")
	operations := operationIDsByPath(&doc)
	for _, site := range sortedKeys(declared) {
		if boundOutsideDecoders(strings.Split(site, ".")) {
			continue
		}
		field, ok := boundFieldSite(strings.Split(site, "."), operations)
		if !ok {
			if !untaggableBounds.Waived(t, site) {
				t.Errorf("%s declares a number bound that no generated field can carry; "+
					"move it onto a property, or add it to untaggableBounds with who holds it", site)
			}
			continue
		}
		if got := tagged[field]; got != declared[site] {
			t.Errorf("%s declares bounds %q but the generated field %s carries %q; the decoders hold the field's",
				site, declared[site], field, got)
		}
	}
}

func boundOutsideDecoders(path []string) bool {
	if len(path) > 1 && path[0] == "components" && path[1] == "parameters" {
		return true
	}
	return path[0] == "paths" && (slices.Contains(path, "parameters") || slices.Contains(path, "responses"))
}

// boundDeclarations records each schema declaring minimum or maximum under
// its dotted document path, valued "min,max" with an absent end left empty.
func boundDeclarations(node *yaml.Node, path []string, into map[string]string) {
	switch node.Kind {
	case yaml.DocumentNode:
		for _, child := range node.Content {
			boundDeclarations(child, path, into)
		}
	case yaml.MappingNode:
		low, high := mappingScalar(node, "minimum"), mappingScalar(node, "maximum")
		if low != "" || high != "" {
			into[strings.Join(path, ".")] = low + "," + high
		}
		for i := 0; i+1 < len(node.Content); i += 2 {
			key, val := node.Content[i].Value, node.Content[i+1]
			if key == "example" || key == "default" || key == "examples" {
				continue
			}
			boundDeclarations(val, append(slices.Clone(path), key), into)
		}
	case yaml.SequenceNode:
		for i, child := range node.Content {
			boundDeclarations(child, append(slices.Clone(path), strconv.Itoa(i)), into)
		}
	}
}

func mappingScalar(node *yaml.Node, key string) string {
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key && node.Content[i+1].Kind == yaml.ScalarNode {
			return node.Content[i+1].Value
		}
	}
	return ""
}

// operationIDsByPath maps "paths.<path>.<method>" to its operationId.
func operationIDsByPath(doc *yaml.Node) map[string]string {
	out := map[string]string{}
	paths := mappingChild(doc.Content[0], "paths")
	if paths == nil {
		return out
	}
	for i := 0; i+1 < len(paths.Content); i += 2 {
		item := paths.Content[i+1]
		for j := 0; j+1 < len(item.Content); j += 2 {
			if id := mappingScalar(item.Content[j+1], "operationId"); id != "" {
				out["paths."+paths.Content[i].Value+"."+item.Content[j].Value] = id
			}
		}
	}
	return out
}

func mappingChild(node *yaml.Node, key string) *yaml.Node {
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return node.Content[i+1]
		}
	}
	return nil
}

// boundFieldSite names the generated field a bounded schema becomes, as
// Type.json[.json…]: a named schema's property, or a property of an inline
// request body. allOf merges into one struct, so its branches add no
// segment. A path through anything else, or ending on an array item, has no
// field. A path's own segments may hold dots (a route), so a request body
// is found by its tail.
func boundFieldSite(path []string, operations map[string]string) (string, bool) {
	var owner string
	var rest []string
	switch {
	case len(path) > 3 && path[0] == "components" && path[1] == "schemas":
		owner, rest = path[2], path[3:]
	case path[0] == "paths":
		body := slices.Index(path, "requestBody")
		if body < 0 || len(path) < body+4 || path[body+3] != "schema" {
			return "", false
		}
		id, ok := operations[strings.Join(path[:body], ".")]
		if !ok {
			return "", false
		}
		owner, rest = strings.ToUpper(id[:1])+id[1:]+"JSONBody", path[body+4:]
	default:
		return "", false
	}
	field := owner
	for i := 0; i < len(rest); i++ {
		switch {
		case rest[i] == "properties" && i+1 < len(rest):
			i++
			field += "." + rest[i]
		case rest[i] == "items" && i+1 < len(rest):
		case rest[i] == "allOf" && i+1 < len(rest):
			i++
		default:
			return "", false
		}
	}
	return field, field != owner
}

// boundTaggedFields maps each generated field tagged with a bound, named
// Type.json[.json…] through inline structs and lists of them, to "min,max".
func boundTaggedFields(t *testing.T, path string) map[string]string {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.SkipObjectResolution)
	if err != nil {
		t.Fatal(err)
	}
	tagged := map[string]string{}
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok {
			continue
		}
		for _, spec := range gen.Specs {
			if typeSpec, ok := spec.(*ast.TypeSpec); ok {
				collectBoundTags(t, typeSpec.Type, typeSpec.Name.Name, tagged)
			}
		}
	}
	return tagged
}

func collectBoundTags(t *testing.T, expr ast.Expr, owner string, into map[string]string) {
	t.Helper()
	switch shape := expr.(type) {
	case *ast.StarExpr:
		collectBoundTags(t, shape.X, owner, into)
	case *ast.ArrayType:
		collectBoundTags(t, shape.Elt, owner, into)
	case *ast.StructType:
		for _, field := range shape.Fields.List {
			name := owner
			var tag reflect.StructTag
			if field.Tag != nil {
				raw, err := strconv.Unquote(field.Tag.Value)
				if err != nil {
					t.Fatalf("unreadable struct tag %s: %v", field.Tag.Value, err)
				}
				tag = reflect.StructTag(raw)
				json, _, _ := strings.Cut(tag.Get("json"), ",")
				name = owner + "." + json
			}
			low, high := tag.Get(datasource.MinimumTag), tag.Get(datasource.MaximumTag)
			if low != "" || high != "" {
				into[name] = low + "," + high
			}
			collectBoundTags(t, field.Type, name, into)
		}
	}
}
