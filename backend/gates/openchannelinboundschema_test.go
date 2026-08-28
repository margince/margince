// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates_test

// The published OpenAPI body schema and the `arrival` struct it documents are
// ONE invariant spelled on both sides of a wire.
//
// extensions/openchannel/inbound.openapi.yaml exists because the core admits
// these bytes on a signature and interprets nothing in them: the unit alone
// decides what the document means, and having decided it, the unit is what
// publishes that decision to whoever configures a sender. Nobody reads Go
// source to integrate against a webhook.
//
// A schema that drifts from the struct is worse than no schema. Drift in one
// direction — the YAML claims a field record.go's `arrival` does not carry —
// tells an integrator to fill in a field the connector reads nothing from and
// never uses. Drift in the other — the struct grows a field the YAML never
// mentions — leaves that field undiscoverable except by reading Go, which is
// the exact gap this document exists to close. Both are held here, and the
// required-field set is held too: `message_id` is the only field record.go
// refuses a body for lacking, and the YAML must say so and nothing more.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

const (
	// arrivalFile is where the connector's own package declares the document a
	// sender posts. Read fresh by this gate rather than assumed, so a struct
	// that moves under this test is still the one compared.
	arrivalFile = "../extensions/openchannel/record.go"
	// inboundSchemaFile is the published contract, read the same way.
	inboundSchemaFile = "../extensions/openchannel/inbound.openapi.yaml"

	arrivalStructName = "arrival"
	partyStructName   = "party"
	arrivalSchemaName = "InboundArrival"
	partySchemaName   = "InboundParty"
)

// openapiDoc is the sliver of the document's shape this gate needs. Unknown
// keys are ignored by yaml.v3's default decode, which is fine here: this gate
// asserts what the two sides must agree on, not the whole document's shape.
type openapiDoc struct {
	Components struct {
		Schemas map[string]openapiSchema `yaml:"schemas"`
	} `yaml:"components"`
}

type openapiSchema struct {
	Required   []string                 `yaml:"required"`
	Properties map[string]openapiSchema `yaml:"properties"`
	Ref        string                   `yaml:"$ref"`
}

func TestTheInboundSchemaMatchesTheArrivalStructInBothDirections(t *testing.T) {
	fromGo := structJSONFields(t, arrivalFile, arrivalStructName)
	fromGoParty := structJSONFields(t, arrivalFile, partyStructName)

	doc := readOpenAPIDoc(t, inboundSchemaFile)
	arrivalSchema, ok := doc.Components.Schemas[arrivalSchemaName]
	if !ok {
		t.Fatalf("%s declares no components.schemas.%s — the gate is comparing nothing", inboundSchemaFile, arrivalSchemaName)
	}
	partySchema, ok := doc.Components.Schemas[partySchemaName]
	if !ok {
		t.Fatalf("%s declares no components.schemas.%s — the gate is comparing nothing", inboundSchemaFile, partySchemaName)
	}

	fromYAML := schemaPropertyNames(arrivalSchema)
	fromYAMLParty := schemaPropertyNames(partySchema)

	if len(fromGo) == 0 {
		t.Fatalf("read no json tags from %s's %s struct — the gate is comparing nothing", arrivalFile, arrivalStructName)
	}
	if len(fromYAML) == 0 {
		t.Fatalf("read no properties from %s's %s schema — the gate is comparing nothing", inboundSchemaFile, arrivalSchemaName)
	}

	compareFieldSets(t, "arrival", fromGo, fromYAML, arrivalFile, inboundSchemaFile)
	compareFieldSets(t, "party", fromGoParty, fromYAMLParty, arrivalFile, inboundSchemaFile)

	// The required set is not "whatever the struct happens to need" — record.go
	// refuses a body for exactly one missing field, message_id, and the YAML's
	// required list is the promise an integrator reads before anything else.
	wantRequired := []string{"message_id"}
	gotRequired := append([]string(nil), arrivalSchema.Required...)
	sort.Strings(gotRequired)
	if strings.Join(gotRequired, ",") != strings.Join(wantRequired, ",") {
		t.Errorf("%s marks %v required on %s, but record.go's recordFor refuses a body for lacking exactly %v — "+
			"the published contract must name the one field that actually blocks acceptance, no more and no less",
			inboundSchemaFile, gotRequired, arrivalSchemaName, wantRequired)
	}
}

// compareFieldSets asserts the two named sets are equal, reporting which file
// would need which fix for whichever direction fails.
func compareFieldSets(t *testing.T, label string, fromGo, fromYAML map[string]bool, goFile, yamlFile string) {
	t.Helper()
	for name := range fromGo {
		if !fromYAML[name] {
			t.Errorf("%s's %s struct carries JSON field %q that %s does not publish — a sender integrating "+
				"against the published contract has no way to learn this field exists", goFile, label, name, yamlFile)
		}
	}
	for name := range fromYAML {
		if !fromGo[name] {
			t.Errorf("%s publishes field %q on its %s schema that no field in %s's %s struct reads — an "+
				"integrator who fills it in would see it silently ignored", yamlFile, name, label, goFile, label)
		}
	}
}

// structJSONFields reads one struct's JSON tag names from the AST, so this
// gate reads the declaration the connector actually unmarshals into rather
// than a copy of it.
func structJSONFields(t *testing.T, path, structName string) map[string]bool {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		t.Fatalf("parsing %s: %v", path, err)
	}
	names := map[string]bool{}
	found := false
	ast.Inspect(file, func(n ast.Node) bool {
		spec, ok := n.(*ast.TypeSpec)
		if !ok || spec.Name.Name != structName {
			return true
		}
		structType, ok := spec.Type.(*ast.StructType)
		if !ok {
			t.Fatalf("%s: %s is not declared as a struct", path, structName)
		}
		found = true
		for _, field := range structType.Fields.List {
			if field.Tag == nil {
				continue
			}
			tag := strings.Trim(field.Tag.Value, "`")
			name := jsonTagName(tag)
			if name != "" && name != "-" {
				names[name] = true
			}
		}
		return false
	})
	if !found {
		t.Fatalf("%s declares no struct named %s — the gate is reading the wrong declaration", path, structName)
	}
	return names
}

// jsonTagName extracts the name half of a `json:"..."` struct tag, dropping
// any comma-separated option (omitempty and friends).
func jsonTagName(tag string) string {
	const key = `json:"`
	start := strings.Index(tag, key)
	if start < 0 {
		return ""
	}
	rest := tag[start+len(key):]
	end := strings.Index(rest, `"`)
	if end < 0 {
		return ""
	}
	value := rest[:end]
	if comma := strings.Index(value, ","); comma >= 0 {
		value = value[:comma]
	}
	return value
}

// readOpenAPIDoc parses the published contract strictly enough to read its
// schemas, and fails loudly on a document this gate cannot read at all.
func readOpenAPIDoc(t *testing.T, path string) openapiDoc {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	var doc openapiDoc
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parsing %s: %v", path, err)
	}
	return doc
}

// schemaPropertyNames is the set of property names a schema declares.
func schemaPropertyNames(schema openapiSchema) map[string]bool {
	names := make(map[string]bool, len(schema.Properties))
	for name := range schema.Properties {
		names[name] = true
	}
	return names
}
