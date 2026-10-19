// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package langcopytest

import (
	"reflect"
	"slices"
	"strings"

	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// Sentences reads the older per-language structs. Exclusions name metadata or
// nested vocabularies whose own tests cover them; an unknown exclusion fails.
//
//craft:ignore naked-any legacy tables have distinct private struct types
func Sentences(t reporter, record any, excluded ...string) map[string]string {
	t.Helper()
	value := reflect.ValueOf(record)
	if value.Kind() != reflect.Struct {
		t.Fatalf("copy is not a struct")
		return nil
	}
	sentences := map[string]string{}
	omitted := map[string]bool{}
	for i := range value.NumField() {
		name := value.Type().Field(i).Name
		if slices.Contains(excluded, name) {
			omitted[name] = true
			continue
		}
		field := value.Field(i)
		if field.Type() != reflect.TypeFor[string]() {
			t.Errorf("%s is not a sentence or an explicitly excluded field", name)
			continue
		}
		sentences[name] = field.String()
	}
	for _, name := range excluded {
		if !omitted[name] {
			t.Errorf("excluded field %s does not exist", name)
		}
	}
	if len(sentences) == 0 {
		t.Fatalf("copy has no sentences")
	}
	return sentences
}

// CensusByLanguage holds legacy language-keyed tables to their call sites'
// placeholders, while also requiring every translation to differ from English.
func CensusByLanguage[T any](t reporter, table map[textlang.Lang]T, holes map[string][]string, excluded ...string) {
	t.Helper()
	english, ok := table[textlang.English]
	if !ok {
		t.Fatalf("copy has no English entry")
		return
	}
	baseline := Sentences(t, english, excluded...)
	for name := range baseline {
		if _, ok := holes[name]; !ok {
			t.Errorf("%s has no call-site placeholder contract", name)
		}
	}
	for name := range holes {
		if _, ok := baseline[name]; !ok {
			t.Errorf("placeholder contract %s has no sentence", name)
		}
	}
	for _, lang := range textlang.Shipped {
		record, ok := table[lang]
		if !ok {
			t.Errorf("copy has no %s entry", lang)
			continue
		}
		for name, sentence := range Sentences(t, record, excluded...) {
			if strings.TrimSpace(sentence) == "" {
				t.Errorf("%s.%s is empty", lang, name)
			}
			if !slices.Equal(Placeholders(sentence), holes[name]) {
				t.Errorf("%s.%s has placeholders %v, expected %v", lang, name, Placeholders(sentence), holes[name])
			}
			if lang != textlang.English && sentence == baseline[name] {
				t.Errorf("%s.%s repeats English", lang, name)
			}
		}
	}
}
