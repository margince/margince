// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

// The margince.yaml editor schema agrees with the config loader on what a file
// may contain. These hold it to the loader.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"gopkg.in/yaml.v3"

	"github.com/margince/margince/backend/internal/platform/deployconfig"
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
	t.Parallel()
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
	t.Parallel()
	raw, err := os.ReadFile(configSchemaPath)
	if err != nil {
		t.Fatalf("read schema: %v", err)
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parse schema: %v", err)
	}
	missing := missingFields(reflect.TypeFor[deployconfig.Config](), doc, "")
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
	for f := range t.Fields() {
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
		if inner.Kind() == reflect.Struct && inner != reflect.TypeFor[deployconfig.Secret]() {
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

// An explicit null on a pointer field is something the LOADER accepts, so the
// schema accepts it too.
//
// yaml.v3 decodes `bootstrap_admin: null` — and a key left bare, which parses
// the same way — into a nil pointer, which is how an operator comments a
// section out without deleting it. A schema that flagged that would be wrong
// about the file in the direction that teaches contacts to ignore the squiggle.
func TestTheSchemaAcceptsAnExplicitNullWhereTheLoaderDoes(t *testing.T) {
	t.Parallel()
	const withNulls = `version: 1
bootstrap_admin: null
uploads:
  attachment_mb: null
`
	// Through deployconfig.Parse, which is what the server actually calls:
	// decoding ALONE is a weaker claim, because Parse also validates, and a
	// schema matched against the decoder would accept a file the loader goes on
	// to reject. If this ever stops being accepted the test fails here, on the
	// premise, rather than quietly asserting a rule nobody holds any more.
	if _, err := deployconfig.Parse([]byte(withNulls)); err != nil {
		t.Fatalf("the loader refuses an explicit null — this test's premise is gone: %v", err)
	}

	var doc any
	if err := yaml.Unmarshal([]byte(withNulls), &doc); err != nil {
		t.Fatalf("probe yaml: %v", err)
	}
	encoded, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("probe will not round-trip: %v", err)
	}
	var value any
	if err := json.Unmarshal(encoded, &value); err != nil {
		t.Fatalf("probe: %v", err)
	}
	if err := compiledConfigSchema(t).Validate(value); err != nil {
		t.Errorf("the schema rejects a null the loader accepts:\n%v", err)
	}
}

// The security.txt block is where the schema says more than the shape: the
// loader refuses a block with no contact or no expiry, so the editor does too,
// and both accept the minimal block RFC 9116 allows.
func TestTheSchemaAndTheLoaderAgreeOnTheSecurityTxtBlock(t *testing.T) {
	t.Parallel()
	schema := compiledConfigSchema(t)
	for name, tc := range map[string]struct {
		block string
		ok    bool
	}{
		"minimal":       {`{ contact: ["mailto:a@example.org"], expires: "2030-01-01T00:00:00Z" }`, true},
		"no contact":    {`{ expires: "2030-01-01T00:00:00Z" }`, false},
		"empty contact": {`{ contact: [], expires: "2030-01-01T00:00:00Z" }`, false},
		"no expires":    {`{ contact: ["mailto:a@example.org"] }`, false},
	} {
		src := "version: 1\nweb:\n  security_txt: " + tc.block + "\n"
		if _, err := deployconfig.Parse([]byte(src)); (err == nil) != tc.ok {
			t.Fatalf("%s: the loader's answer is not %v (%v) — this test's premise is gone", name, tc.ok, err)
		}
		var doc any
		if err := yaml.Unmarshal([]byte(src), &doc); err != nil {
			t.Fatalf("%s: probe yaml: %v", name, err)
		}
		encoded, err := json.Marshal(doc)
		if err != nil {
			t.Fatalf("%s: probe will not round-trip: %v", name, err)
		}
		var value any
		if err := json.Unmarshal(encoded, &value); err != nil {
			t.Fatalf("%s: probe: %v", name, err)
		}
		if err := schema.Validate(value); (err == nil) != tc.ok {
			t.Errorf("%s: the schema's answer differs from the loader's (want ok=%v): %v", name, tc.ok, err)
		}
	}
}
