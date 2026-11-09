// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package aicert

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// shapeBlindFields are the request fields a replay may ignore, each with why
// it cannot change an answer.
var shapeBlindFields = map[string]string{
	"SecretStripper":        "hygiene over the outbound payload, the same on every run",
	"ContextBytes":          "trace metadata of the context block ContextFingerprint already digests",
	"ContextTokensEstimate": "trace metadata of the context block ContextFingerprint already digests",
}

// Every model.Request field either moves the shape or is named above with a
// reason. The census reads the struct, so a field added later fails here until
// someone decides which it is.
func TestEveryRequestFieldThatCanChangeAnAnswerMovesItsShape(t *testing.T) {
	base := model.Request{System: "Describe the subject.", Messages: []model.Message{{Role: "user", Content: "a widget"}}}
	baseShape := shapeOf(t, base)
	fields := reflect.TypeFor[model.Request]().NumField()
	for i := range fields {
		field := reflect.TypeFor[model.Request]().Field(i)
		if _, blind := shapeBlindFields[field.Name]; blind {
			continue
		}
		changed := base
		setNonZero(t, reflect.ValueOf(&changed).Elem().Field(i), field.Name)
		if shapeOf(t, changed) == baseShape {
			t.Errorf("model.Request.%s changed and the shape did not: a replay would answer a request it was never asked", field.Name)
		}
	}
	for name := range shapeBlindFields {
		if _, ok := reflect.TypeFor[model.Request]().FieldByName(name); !ok {
			t.Errorf("shapeBlindFields names %s, which model.Request no longer has", name)
		}
	}
}

func shapeOf(t *testing.T, req model.Request) string {
	t.Helper()
	shape, err := requestShape(req, &idTable{})
	if err != nil {
		t.Fatalf("requestShape: %v", err)
	}
	return shape
}

// setNonZero gives a request field a value other than the base request's.
func setNonZero(t *testing.T, v reflect.Value, name string) {
	t.Helper()
	switch v.Interface().(type) {
	case string:
		v.SetString("changed")
	case int:
		v.SetInt(7)
	case bool:
		v.SetBool(true)
	case []string:
		v.Set(reflect.ValueOf([]string{"changed"}))
	case []model.Message:
		v.Set(reflect.ValueOf([]model.Message{{Role: "user", Content: "a gadget"}}))
	case []model.ToolDef:
		v.Set(reflect.ValueOf([]model.ToolDef{{Name: "lookup"}}))
	case json.RawMessage:
		v.Set(reflect.ValueOf(json.RawMessage(`{"type":"object"}`)))
	case map[string]json.RawMessage:
		v.Set(reflect.ValueOf(map[string]json.RawMessage{"openai": json.RawMessage(`{"reasoning_effort":"low"}`)}))
	case []model.Attachment:
		v.Set(reflect.ValueOf([]model.Attachment{{MIME: "image/png", Bytes: []byte{1}}}))
	default:
		t.Fatalf("model.Request.%s is a %s this census cannot set; teach setNonZero, then decide whether it moves the shape", name, v.Type())
	}
}

// Two attachments that differ only in their bytes are two requests.
func TestAnAttachmentsBytesMoveTheShape(t *testing.T) {
	one := model.Request{Attachments: []model.Attachment{{MIME: "image/png", Bytes: []byte{1}}}}
	two := model.Request{Attachments: []model.Attachment{{MIME: "image/png", Bytes: []byte{2}}}}
	if shapeOf(t, one) == shapeOf(t, two) {
		t.Fatal("two attachments with different bytes share a shape")
	}
}
