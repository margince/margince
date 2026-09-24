// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package schema_test

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/schema"
)

func TestValidateJSONAcceptsAConformingObject(t *testing.T) {
	sch := schema.Must(schema.Object(
		map[string]schema.Node{
			"name":   schema.String(),
			"status": schema.Enum("new", "won", "lost"),
		},
		"name", "status",
	))
	if err := schema.ValidateJSON(sch, `{"name":"Acme","status":"won"}`); err != nil {
		t.Fatalf("ValidateJSON: %v", err)
	}
}

func TestValidateJSONRejectsAMissingRequiredProperty(t *testing.T) {
	sch := schema.Must(schema.Object(
		map[string]schema.Node{"name": schema.String()}, "name",
	))
	if err := schema.ValidateJSON(sch, `{}`); err == nil {
		t.Fatal("want an error for a missing required property, got nil")
	}
}

func TestValidateJSONRejectsAnUnknownPropertyOnAClosedObject(t *testing.T) {
	sch := schema.Must(schema.Object(
		map[string]schema.Node{"name": schema.String()}, "name",
	))
	if err := schema.ValidateJSON(sch, `{"name":"Acme","extra":true}`); err == nil {
		t.Fatal("want an error for an additional property on a closed object, got nil")
	}
}

func TestValidateJSONRejectsAnEnumValueOutsideItsSet(t *testing.T) {
	sch := schema.Must(schema.Enum("new", "won", "lost"))
	if err := schema.ValidateJSON(sch, `"deleted"`); err == nil {
		t.Fatal("want an error for an out-of-set enum value, got nil")
	}
}

func TestValidateJSONWalksArrayItems(t *testing.T) {
	sch := schema.Must(schema.Array(schema.Object(
		map[string]schema.Node{"value": schema.String()}, "value",
	)))
	if err := schema.ValidateJSON(sch, `[{"value":"a"},{"value":"b"}]`); err != nil {
		t.Fatalf("ValidateJSON: %v", err)
	}
	if err := schema.ValidateJSON(sch, `[{"value":"a"},{}]`); err == nil {
		t.Fatal("want an error for the second item missing its required property, got nil")
	}
}

func TestValidateJSONRejectsATypeMismatch(t *testing.T) {
	sch := schema.Must(schema.Number())
	if err := schema.ValidateJSON(sch, `"not a number"`); err == nil {
		t.Fatal("want an error for a string where a number is required, got nil")
	}
}

func TestValidateJSONRejectsMalformedValueJSON(t *testing.T) {
	sch := schema.Must(schema.Object(map[string]schema.Node{"name": schema.String()}, "name"))
	if err := schema.ValidateJSON(sch, `{not json`); err == nil {
		t.Fatal("want an error for output that is not valid JSON, got nil")
	}
}

func TestValidateJSONHoldsAnIntegerToAWholeNumber(t *testing.T) {
	sch := schema.Must(schema.Array(schema.Integer()))
	if err := schema.ValidateJSON(sch, `[1, 2, 3.0]`); err != nil {
		t.Fatalf("whole numbers refused: %v", err)
	}
	for _, value := range []string{`[1, 2.5]`, `["3"]`} {
		if err := schema.ValidateJSON(sch, value); err == nil {
			t.Errorf("%s accepted as a list of integers", value)
		}
	}
}

func TestValidateJSONAdmitsAnOptionalAsItsValueOrNull(t *testing.T) {
	sch := schema.Must(schema.Record(
		schema.Field("reply", schema.Optional(schema.Enum("positive", "negative"))),
	))
	for _, value := range []string{`{"reply":"positive"}`, `{"reply":null}`} {
		if err := schema.ValidateJSON(sch, value); err != nil {
			t.Errorf("%s refused: %v", value, err)
		}
	}
	for _, value := range []string{`{"reply":"maybe"}`, `{"reply":3}`, `{}`} {
		if err := schema.ValidateJSON(sch, value); err == nil {
			t.Errorf("%s accepted", value)
		}
	}
}

func TestValidateJSONRefusesANodeThatDescribesNothing(t *testing.T) {
	if err := schema.ValidateJSON([]byte(`{}`), `"anything"`); err == nil {
		t.Fatal("a schema node with neither a type nor anyOf admitted a value")
	}
}
