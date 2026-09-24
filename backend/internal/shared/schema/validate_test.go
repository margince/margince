// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package schema_test

import (
	"encoding/json"
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

// An integer passes exactly when the field's reader — encoding/json into an
// int — would take it, so a validated answer is never one its reader refuses.
func TestValidateJSONHoldsAnIntegerToWhatAnIntReaderTakes(t *testing.T) {
	sch := schema.Must(schema.Array(schema.Integer()))
	for _, value := range []string{`[1, -2, 0]`, `[9223372036854775807]`, `[]`} {
		if err := schema.ValidateJSON(sch, value); err != nil {
			t.Errorf("%s refused: %v", value, err)
		}
		var read []int
		if err := json.Unmarshal([]byte(value), &read); err != nil {
			t.Errorf("fixture %s is not one an int reader takes: %v", value, err)
		}
	}
	for _, value := range []string{`[1, 2.5]`, `["3"]`, `[3.0]`, `[1e2]`, `[1e300]`, `[9223372036854775808]`} {
		if err := schema.ValidateJSON(sch, value); err == nil {
			t.Errorf("%s accepted as a list of integers", value)
		}
		var read []int
		if err := json.Unmarshal([]byte(value), &read); err == nil {
			t.Errorf("fixture %s is one an int reader takes, so refusing it proves nothing", value)
		}
	}
}

// A number field takes any JSON number, integral or not.
func TestValidateJSONAdmitsAnyNumberForANumberField(t *testing.T) {
	sch := schema.Must(schema.Array(schema.Number()))
	if err := schema.ValidateJSON(sch, `[1, 2.5, 3.0, 1e300, -0]`); err != nil {
		t.Fatalf("numbers refused: %v", err)
	}
}

func TestValidateJSONRefusesTrailingValues(t *testing.T) {
	sch := schema.Must(schema.Number())
	for _, value := range []string{`1 2`, `1 ]`, `1 }`, `1 x`} {
		if err := schema.ValidateJSON(sch, value); err == nil {
			t.Errorf("%q accepted as one JSON value", value)
		}
	}
	if err := schema.ValidateJSON(sch, " 1 \n"); err != nil {
		t.Errorf("whitespace around a value refused: %v", err)
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
