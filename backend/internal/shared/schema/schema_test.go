// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package schema_test

import (
	"encoding/json"
	"testing"

	"github.com/margince/margince/backend/internal/shared/schema"
)

// decode renders a node and unmarshals it to a generic map so assertions read
// keys directly — no tagged struct, so the JSON Schema camelCase keywords stay
// literal here rather than fighting the snake_case tag linter.
func decode(t *testing.T, n schema.Node) map[string]any {
	t.Helper()
	raw := schema.Must(n)
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("Must produced invalid JSON: %v (%s)", err, raw)
	}
	return m
}

func TestObjectIsClosedWithItsRequiredAndProperties(t *testing.T) {
	m := decode(t, schema.Object(
		map[string]schema.Node{"name": schema.String(), "age": schema.Number()},
		"name",
	))

	if m["type"] != "object" {
		t.Fatalf("type = %v, want object", m["type"])
	}
	// A closed object is the whole point: additionalProperties must be present
	// and explicitly false, not omitted.
	ap, ok := m["additionalProperties"]
	if !ok || ap != false {
		t.Fatalf("additionalProperties = %v (present=%v), want explicit false", ap, ok)
	}
	props, _ := m["properties"].(map[string]any)
	if _, ok := props["name"]; !ok {
		t.Fatalf("missing property name: %v", props)
	}
	req, _ := m["required"].([]any)
	if len(req) != 1 || req[0] != "name" {
		t.Fatalf("required = %v, want [name]", req)
	}
}

func TestScalarLeavesMarshalToTheirType(t *testing.T) {
	cases := map[string]schema.Node{
		"string": schema.String(),
		"number": schema.Number(),
	}
	for want, node := range cases {
		m := decode(t, node)
		if m["type"] != want {
			t.Errorf("%s leaf marshalled type = %v", want, m["type"])
		}
		if _, extra := m["additionalProperties"]; extra {
			t.Errorf("%s leaf carried additionalProperties", want)
		}
	}
}

func TestEnumIsAStringLeafConstrainedToItsValues(t *testing.T) {
	m := decode(t, schema.Enum("new", "working", "won"))
	if m["type"] != "string" {
		t.Fatalf("enum type = %v, want string", m["type"])
	}
	vals, ok := m["enum"].([]any)
	if !ok || len(vals) != 3 || vals[0] != "new" || vals[2] != "won" {
		t.Fatalf("enum values wrong: %v", m["enum"])
	}
	// A non-enum leaf must omit the key.
	if _, present := decode(t, schema.String())["enum"]; present {
		t.Fatal("plain string leaf emitted an enum key")
	}
}

func TestDescribeAttachesTheDescriptionAnnotation(t *testing.T) {
	m := decode(t, schema.String().Describe("the customer's full legal name"))
	if m["description"] != "the customer's full legal name" {
		t.Fatalf("description = %v", m["description"])
	}
	// A node without Describe must omit the key entirely.
	if _, present := decode(t, schema.String())["description"]; present {
		t.Fatal("undescribed node emitted a description key")
	}
}

func TestArrayOfClosedObjectKeepsTheItemObjectClosed(t *testing.T) {
	// The shipped shape: an array whose items are closed objects. Pin that the
	// nested object still emits additionalProperties:false end-to-end.
	m := decode(t, schema.Array(schema.Object(
		map[string]schema.Node{"k": schema.String()}, "k",
	)))
	items, ok := m["items"].(map[string]any)
	if !ok {
		t.Fatalf("items missing: %v", m)
	}
	if ap, ok := items["additionalProperties"]; !ok || ap != false {
		t.Fatalf("nested object item not closed: %v", items)
	}
}

func TestArrayCarriesItemSchemaAndScalarsHaveNoExtraKeys(t *testing.T) {
	m := decode(t, schema.Array(schema.Number()))

	if m["type"] != "array" {
		t.Fatalf("type = %v, want array", m["type"])
	}
	items, ok := m["items"].(map[string]any)
	if !ok || items["type"] != "number" {
		t.Fatalf("items wrong: %v", m["items"])
	}
	// A scalar leaf must not carry additionalProperties — that keyword is only
	// meaningful (and only accepted by strict providers) on objects.
	if _, present := items["additionalProperties"]; present {
		t.Fatalf("scalar leaf carried additionalProperties: %v", items)
	}
}

func TestIntegerIsItsOwnTypeNotANumber(t *testing.T) {
	if got := decode(t, schema.Integer())["type"]; got != "integer" {
		t.Fatalf("integer leaf type = %v, want integer", got)
	}
}

// Optional keeps the field required and spells absence as null, because the
// strict profile refuses an object with a property missing from required.
func TestOptionalIsTheValueOrNull(t *testing.T) {
	m := decode(t, schema.Optional(schema.Enum("positive", "negative")))
	if _, typed := m["type"]; typed {
		t.Fatalf("an optional node carries its own type beside anyOf: %v", m)
	}
	branches, ok := m["anyOf"].([]any)
	if !ok || len(branches) != 2 {
		t.Fatalf("anyOf = %v, want the value and null", m["anyOf"])
	}
	value, _ := branches[0].(map[string]any)
	null, _ := branches[1].(map[string]any)
	if value["type"] != "string" || null["type"] != "null" {
		t.Fatalf("branches = %v, want the enum then null", branches)
	}
}

// A Record renders its properties in the order declared, and requires every
// one of them in that same order.
func TestRecordRendersItsFieldsInDeclaredOrder(t *testing.T) {
	raw := string(schema.Must(schema.Record(
		schema.Field("subject", schema.String()),
		schema.Field("body", schema.String()),
		schema.Field("reasoning", schema.Array(schema.Integer())),
	)))
	want := `{"type":"object","additionalProperties":false,"properties":{` +
		`"subject":{"type":"string"},"body":{"type":"string"},` +
		`"reasoning":{"type":"array","items":{"type":"integer"}}},` +
		`"required":["subject","body","reasoning"]}`
	if raw != want {
		t.Fatalf("record rendered\n %s\nwant\n %s", raw, want)
	}
}

// An Object renders exactly as encoding/json renders the map it holds: a
// prompt version is stamped from these bytes, so an Object's rendering is part
// of what a certification record describes.
func TestObjectRendersItsPropertiesSortedByName(t *testing.T) {
	raw := string(schema.Must(schema.Object(map[string]schema.Node{
		"zeta": schema.String(), "alpha": schema.Number(), "mid<&>": schema.String().Describe("a<b & c>d"),
	}, "zeta", "alpha")))
	want := `{"type":"object","additionalProperties":false,"properties":{` +
		`"alpha":{"type":"number"},"mid\u003c\u0026\u003e":{"type":"string","description":"a\u003cb \u0026 c\u003ed"},` +
		`"zeta":{"type":"string"}},"required":["zeta","alpha"]}`
	if raw != want {
		t.Fatalf("object rendered\n %s\nwant\n %s", raw, want)
	}
}

func TestRecordRefusesAFieldDeclaredTwice(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("a record naming one field twice rendered instead of panicking")
		}
	}()
	schema.Record(schema.Field("id", schema.String()), schema.Field("id", schema.Integer()))
}

type verdict string

func TestNamesSpellsANamedStringTypeAsPlainStrings(t *testing.T) {
	got := schema.Names(verdict("agreed"), verdict("proposed"))
	if len(got) != 2 || got[0] != "agreed" || got[1] != "proposed" {
		t.Fatalf("Names = %v", got)
	}
}
