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

// untaggableBounds are the bounds no generated field carries: an array item, a
// map value, a oneOf branch, an inline response. No decoder holds them, so each
// says who does instead.
var untaggableBounds = gatekit.Waive(map[string]string{
	"components.schemas.WorkingHours.properties.days.items":                                                  "a response; the request twin is SaveWorkingHoursRequest",
	"components.schemas.SaveWorkingHoursRequest.properties.days.items":                                       "identity.validWorkingHours refuses a weekday outside 1..7",
	"components.schemas.ApprovalEvidence.properties.source_lines.items":                                      "a response; the server numbers the lines",
	"components.schemas.AiDecisionSummary.properties.fallbacks.additionalProperties":                         "a response map the server fills; no request decodes it",
	"components.schemas.ProviderCredits.properties.pools.additionalProperties":                               "a response map the server fills; no request decodes it",
	"components.schemas.ProviderCategoryCost.properties.cost.additionalProperties":                           "a response map the server fills; no request decodes it",
	"components.schemas.VoiceCorpusSummary.properties.register_words.additionalProperties":                   "a response map the server fills; no request decodes it",
	"components.schemas.AiOpenRouterProvider.properties.preferred_min_throughput.oneOf.0":                    "ai.nonNegative refuses a negative one when routing is saved",
	"components.schemas.AiOpenRouterProvider.properties.preferred_max_latency.oneOf.0":                       "ai.nonNegative refuses a negative one when routing is saved",
	"components.parameters.VoiceProfileVersionNumber.schema":                                                 "a path parameter; a number below 1 names no version and answers 404",
	"paths./analytics/reports/{id}/revisions/{revision}/restore.parameters.1.schema":                         "a path parameter; a number below 1 names no revision and answers 404",
	"paths./contacts/{id}/research/save.post.responses.200.content.application/json.schema.properties.saved": "an inline response; no request decodes it",
	// A property typed through a one-branch allOf keeps none of the referenced
	// schema's tags, so these fields are named rather than declarations.
	"AttentionItem.version":      "a response; the row version is the server's",
	"Company360NextStep.version": "a response; the row version is the server's",
	"EmailPresentation.version":  "a response; the row version is the server's",
	"EmailSummary.version":       "a response; the row version is the server's",
	"WorklistItem.version":       "a response; the row version is the server's",
})

// The generated fields that carry a bound are the fields the contract bounds,
// no more and no fewer. The expected set comes from walking the contract as the
// generator does. It covers named schemas, allOf merges, $ref'd scalars, inline
// request bodies and query parameters. The two sets must match, bound for bound.
// Every bounded schema in the contract must also be reached by that walk or be
// waived, so a bound the generator drops cannot hide.
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
	census := newBoundCensus(t, doc.Content[0])
	census.walkContract()
	tagged := boundTaggedFields(t, "internal/contracts/api_gen.go")
	for _, field := range sortedKeys(census.expected) {
		if got := tagged[field]; got != census.expected[field] {
			t.Errorf("the contract bounds %s to %q but the generated field carries %q; the decoders hold the field's",
				field, census.expected[field], got)
		}
	}
	for _, field := range sortedKeys(tagged) {
		if _, ok := census.expected[field]; !ok {
			t.Errorf("the generated field %s carries bounds %q that the contract walk does not place there", field, tagged[field])
		}
	}
	declared := map[string]*yaml.Node{}
	boundDeclarations(&doc, nil, declared)
	for _, site := range sortedKeys(declared) {
		if !census.reached[declared[site]] && !untaggableBounds.Waived(t, site) {
			t.Errorf("%s declares a number bound that no generated field carries; "+
				"move it onto a property, or add it to untaggableBounds with who holds it", site)
		}
	}
}

// boundCensus is the walk of the contract that names each field a bound lands on.
type boundCensus struct {
	t          *testing.T
	schemas    *yaml.Node
	parameters *yaml.Node
	paths      *yaml.Node
	expected   map[string]string
	reached    map[*yaml.Node]bool
}

func newBoundCensus(t *testing.T, root *yaml.Node) *boundCensus {
	components := mappingChild(root, "components")
	if components == nil {
		t.Fatal("the contract has no components")
	}
	return &boundCensus{
		t: t, schemas: mappingChild(components, "schemas"), parameters: mappingChild(components, "parameters"),
		paths: mappingChild(root, "paths"), expected: map[string]string{}, reached: map[*yaml.Node]bool{},
	}
}

func (c *boundCensus) walkContract() {
	for i := 0; i+1 < len(c.schemas.Content); i += 2 {
		c.walkObject(c.schemas.Content[i+1], c.schemas.Content[i].Value)
	}
	for i := 0; i+1 < len(c.paths.Content); i += 2 {
		item := c.paths.Content[i+1]
		shared := mappingChild(item, "parameters")
		for j := 0; j+1 < len(item.Content); j += 2 {
			id := mappingScalar(item.Content[j+1], "operationId")
			if id == "" {
				continue
			}
			c.walkOperation(item.Content[j+1], strings.ToUpper(id[:1])+id[1:], shared)
		}
	}
}

// walkOperation places a query or header parameter's bound on OpParams and an
// inline JSON body's on OpJSONBody. A path parameter is a handler argument.
func (c *boundCensus) walkOperation(op *yaml.Node, name string, shared *yaml.Node) {
	for _, list := range []*yaml.Node{shared, mappingChild(op, "parameters")} {
		if list == nil {
			continue
		}
		for _, param := range list.Content {
			param = c.resolve(param)
			if in := mappingScalar(param, "in"); in != "query" && in != "header" {
				continue
			}
			if schema := mappingChild(param, "schema"); schema != nil {
				c.expectBound(c.resolve(schema), name+"Params."+mappingScalar(param, "name"))
			}
		}
	}
	body := mappingChild(op, "requestBody")
	if body == nil {
		return
	}
	if content := mappingChild(body, "content"); content != nil {
		if media := mappingChild(content, "application/json"); media != nil {
			if schema := mappingChild(media, "schema"); schema != nil && mappingScalar(schema, "$ref") == "" {
				c.walkObject(schema, name+"JSONBody")
			}
		}
	}
}

// walkObject places the bounds of an object's properties under prefix; allOf
// branches merge into the same struct.
func (c *boundCensus) walkObject(schema *yaml.Node, prefix string) {
	schema = deref(schema)
	if all := mappingChild(schema, "allOf"); all != nil {
		for _, branch := range all.Content {
			c.walkObject(c.resolve(branch), prefix)
		}
	}
	props := mappingChild(schema, "properties")
	if props == nil {
		return
	}
	for i := 0; i+1 < len(props.Content); i += 2 {
		c.walkProperty(props.Content[i+1], prefix+"."+props.Content[i].Value)
	}
}

func (c *boundCensus) walkProperty(prop *yaml.Node, field string) {
	prop = deref(prop)
	if mappingScalar(prop, "$ref") != "" {
		c.expectBound(c.resolve(prop), field)
		return
	}
	if ref := c.wrappedRef(prop); ref != nil {
		if bounded(ref) && !untaggableBounds.Waived(c.t, field) {
			c.t.Errorf("%s reaches a bounded schema through a one-branch allOf, which drops its tags; "+
				"use a plain $ref, or add the field to untaggableBounds with who holds it", field)
		}
		return
	}
	c.expectBound(prop, field)
	if items := mappingChild(prop, "items"); items != nil && mappingScalar(deref(items), "$ref") == "" {
		c.walkObject(deref(items), field)
	}
	c.walkObject(prop, field)
}

// wrappedRef is the schema a property names as the one branch of an allOf.
// The generator types the field as that schema without its extra tags.
func (c *boundCensus) wrappedRef(prop *yaml.Node) *yaml.Node {
	if all := mappingChild(prop, "allOf"); all != nil && len(all.Content) == 1 && mappingScalar(all.Content[0], "$ref") != "" {
		return c.resolve(all.Content[0])
	}
	return nil
}

func bounded(schema *yaml.Node) bool {
	return mappingScalar(schema, "minimum") != "" || mappingScalar(schema, "maximum") != ""
}

// deref follows a YAML alias to the node it repeats.
func deref(node *yaml.Node) *yaml.Node {
	for node.Kind == yaml.AliasNode {
		node = node.Alias
	}
	return node
}

func (c *boundCensus) expectBound(schema *yaml.Node, field string) {
	low, high := mappingScalar(schema, "minimum"), mappingScalar(schema, "maximum")
	if low == "" && high == "" {
		return
	}
	c.reached[schema] = true
	if earlier, seen := c.expected[field]; seen && earlier != low+","+high {
		c.t.Errorf("the contract walk places two bounds on %s: %q and %q", field, earlier, low+","+high)
	}
	c.expected[field] = low + "," + high
}

// resolve follows a local $ref to the schema or parameter it names.
func (c *boundCensus) resolve(node *yaml.Node) *yaml.Node {
	for range 8 {
		node = deref(node)
		ref := mappingScalar(node, "$ref")
		if ref == "" {
			return node
		}
		var into *yaml.Node
		name := ""
		switch {
		case strings.HasPrefix(ref, "#/components/schemas/"):
			into, name = c.schemas, strings.TrimPrefix(ref, "#/components/schemas/")
		case strings.HasPrefix(ref, "#/components/parameters/"):
			into, name = c.parameters, strings.TrimPrefix(ref, "#/components/parameters/")
		default:
			c.t.Fatalf("the contract walk cannot follow %s", ref)
		}
		next := mappingChild(into, name)
		if next == nil {
			c.t.Fatalf("%s names nothing", ref)
		}
		node = next
	}
	c.t.Fatal("a $ref chain longer than eight links")
	return nil
}

// boundDeclarations records each schema declaring minimum or maximum under
// its dotted document path.
func boundDeclarations(node *yaml.Node, path []string, into map[string]*yaml.Node) {
	switch node.Kind {
	case yaml.DocumentNode:
		for _, child := range node.Content {
			boundDeclarations(child, path, into)
		}
	case yaml.MappingNode:
		if mappingScalar(node, "minimum") != "" || mappingScalar(node, "maximum") != "" {
			into[strings.Join(path, ".")] = node
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

func mappingChild(node *yaml.Node, key string) *yaml.Node {
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return node.Content[i+1]
		}
	}
	return nil
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
