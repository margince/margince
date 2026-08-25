// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package backendarch

// The margince.yaml schema is editor tooling, and editor tooling that lies is
// worse than none: an operator trusts the squiggle. These hold it to the loader.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"gopkg.in/yaml.v3"

	"github.com/gradionhq/margince/backend/internal/platform/deployconfig"
)

const configSchemaPath = "../config/margince.schema.json"

func compiledConfigSchema(t *testing.T) *jsonschema.Schema {
	t.Helper()
	raw, err := os.Open(configSchemaPath)
	if err != nil {
		t.Fatalf("open schema: %v", err)
	}
	defer func() {
		if cerr := raw.Close(); cerr != nil {
			t.Errorf("close schema: %v", cerr)
		}
	}()
	doc, err := jsonschema.UnmarshalJSON(raw)
	if err != nil {
		t.Fatalf("parse schema: %v", err)
	}
	c := jsonschema.NewCompiler()
	if err := c.AddResource("margince.json", doc); err != nil {
		t.Fatalf("add schema: %v", err)
	}
	sch, err := c.Compile("margince.json")
	if err != nil {
		t.Fatalf("compile schema: %v", err)
	}
	return sch
}

// Every config this repo ships must validate against the schema this repo
// ships. They are the files an operator copies, so a schema that rejects one is
// telling them their starting point is wrong.
func TestEveryShippedConfigValidatesAgainstTheSchema(t *testing.T) {
	schema := compiledConfigSchema(t)
	paths, err := filepath.Glob("../config/margince*.yaml")
	if err != nil {
		t.Fatalf("globbing the shipped configs: %v", err)
	}
	// NOT a tolerated zero: the tree ships these, so an empty glob means the
	// path moved and this gate would validate nothing while reporting PASS.
	if len(paths) == 0 {
		t.Fatal("no config/margince*.yaml found — the corpus moved and this gate is checking nothing")
	}
	for _, path := range paths {
		t.Run(filepath.Base(path), func(t *testing.T) {
			if err := schema.Validate(asJSONValue(t, path)); err != nil {
				t.Errorf("%s does not validate against the schema an editor will check it with:\n%v", path, err)
			}
		})
	}
}

// asJSONValue reads a YAML file as the plain value a JSON Schema validator
// walks.
//
// but only after a JSON round trip, since it rejects the numeric types yaml
// hands back for an integer.
//
//craft:ignore naked-any jsonschema.Validate takes any — this is the library's seam, not a shape of ours YAML gives map[string]any for a mapping, which the validator wants —
func asJSONValue(t *testing.T, path string) any {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var doc any
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("%s is not parseable yaml: %v", path, err)
	}
	encoded, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("%s will not round-trip to json: %v", path, err)
	}
	var value any
	if err := json.Unmarshal(encoded, &value); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	return value
}

// Every key the LOADER accepts is a key the schema accepts.
//
// The schema says additionalProperties:false, mirroring the loader's
// KnownFields(true) — which makes an omission an active lie rather than a
// silence: a section missing here is reported to the operator as an unknown key
// while the server reads it happily. Derived from the struct rather than a list
// somebody remembers to extend: this walk IS what keeps the generated file and
// the struct together, so it derives its list from the struct rather than
// restating one.
func TestTheSchemaAcceptsEveryFieldTheConfigDeclares(t *testing.T) {
	raw, err := os.ReadFile(configSchemaPath)
	if err != nil {
		t.Fatalf("read schema: %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parse schema: %v", err)
	}
	missing := missingFields(reflect.TypeOf(deployconfig.Config{}), doc, "")
	for _, m := range missing {
		t.Errorf("margince.yaml accepts %s and the schema does not — an editor would flag a key the server reads", m)
	}
}

// missingFields walks the struct beside the schema and names what the schema
// lacks. A $ref is followed no further: the routing subtree has its own gate
// (TestRoutingSchemaEnumsMatchCode) and is not a Go struct on this side.
func missingFields(t reflect.Type, node map[string]any, path string) []string {
	props, _ := node["properties"].(map[string]any)
	var missing []string
	for i := range t.NumField() {
		f := t.Field(i)
		name, ok := schemaFieldName(f)
		if !ok {
			continue
		}
		here := strings.TrimPrefix(path+"."+name, ".")
		child, present := props[name].(map[string]any)
		if !present {
			missing = append(missing, here)
			continue
		}
		if _, isRef := child["$ref"]; isRef {
			continue
		}
		inner := f.Type
		if inner.Kind() == reflect.Pointer {
			inner = inner.Elem()
		}
		if inner.Kind() == reflect.Struct && inner != reflect.TypeOf(deployconfig.Secret{}) {
			missing = append(missing, missingFields(inner, child, here)...)
		}
	}
	return missing
}

func schemaFieldName(f reflect.StructField) (string, bool) {
	tag, ok := f.Tag.Lookup("yaml")
	if !ok {
		return "", false
	}
	name, _, _ := strings.Cut(tag, ",")
	return name, name != "" && name != "-"
}
