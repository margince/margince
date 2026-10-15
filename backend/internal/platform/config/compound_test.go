// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package config_test

import (
	"reflect"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/margince/margince/backend/internal/platform/config"
)

type level string

func TestEnumRefusesAValueItDoesNotList(t *testing.T) {
	typ := config.Enum[level]("debug", "info")
	if _, err := typ.Parse("trace"); err == nil {
		t.Fatal("trace is not listed")
	}
	if typ.Name() != "enum(debug|info)" {
		t.Fatalf("name %q", typ.Name())
	}
}

func TestListTextSplitsOnUnescapedCommas(t *testing.T) {
	got, err := config.List(config.String).Parse(`a,b\,c,`)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"a", "b,c", ""}; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %q want %q", got, want)
	}
	empty, err := config.List(config.String).Parse("")
	if err != nil || len(empty) != 0 {
		t.Fatalf("empty text is the empty list, got %q %v", empty, err)
	}
}

func TestMapTextEscapesBothSeparators(t *testing.T) {
	got, err := config.Map(config.Int).Parse(`a:1,b\:c:2`)
	if err != nil {
		t.Fatal(err)
	}
	if want := map[string]int{"a": 1, "b:c": 2}; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v", got)
	}
	if _, err := config.Map(config.Int).Parse("a:1,a:2"); err == nil {
		t.Fatal("a repeated key is a mistake, not a silent overwrite")
	}
	if _, err := config.Map(config.Int).Parse("novalue"); err == nil {
		t.Fatal("an entry without a colon must fail")
	}
}

func TestMapTextEscapesACommaInAKey(t *testing.T) {
	got, err := config.Map(config.Int).Parse(`a\,b:1`)
	if err != nil {
		t.Fatal(err)
	}
	if want := map[string]int{"a,b": 1}; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

type routing struct {
	Model string `json:"model" yaml:"model"`
}

func TestStructIsJSONInTextAndNativeInYAML(t *testing.T) {
	typ := config.Struct[routing]()
	got, err := typ.Parse(`{"model":"m1"}`)
	if err != nil || got.Model != "m1" {
		t.Fatalf("text: %v %v", got, err)
	}
	if _, err := typ.Parse(`{"modle":"m1"}`); err == nil {
		t.Fatal("an unknown field in JSON must fail, as in YAML")
	}
	var n yaml.Node
	if err := yaml.Unmarshal([]byte("model: m2"), &n); err != nil {
		t.Fatal(err)
	}
	got, err = typ.DecodeYAML(n.Content[0])
	if err != nil || got.Model != "m2" {
		t.Fatalf("yaml: %v %v", got, err)
	}
}

type sized struct {
	Count int `json:"count" yaml:"count"`
}

func TestStructErrorsNeverEchoTheInput(t *testing.T) {
	typ := config.Struct[sized]()
	if _, err := typ.Parse(`{"count":"hunter2-secret"}`); err == nil || strings.Contains(err.Error(), "hunter2") {
		t.Fatalf("JSON error must not echo the value: %v", err)
	}
	if _, err := typ.Parse(`hunter2-secret`); err == nil || strings.Contains(err.Error(), "hunter2") {
		t.Fatalf("JSON syntax error must not echo the value: %v", err)
	}
	var n yaml.Node
	if err := yaml.Unmarshal([]byte("count: hunter2-secret"), &n); err != nil {
		t.Fatal(err)
	}
	if _, err := typ.DecodeYAML(n.Content[0]); err == nil || strings.Contains(err.Error(), "hunter2") {
		t.Fatalf("YAML error must not echo the value: %v", err)
	}
}

func TestStructRefusesTrailingData(t *testing.T) {
	typ := config.Struct[routing]()
	for _, text := range []string{`{"model":"m1"} junk`, `{"model":"m1"}{"x":1}`} {
		if _, err := typ.Parse(text); err == nil {
			t.Fatalf("trailing data must fail: %q", text)
		}
	}
}
