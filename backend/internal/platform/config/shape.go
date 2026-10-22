// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package config

import (
	"encoding/json"
	"fmt"
	"reflect"
)

// Shape is what a generated reference needs about a setting's value beyond its
// type name.
type Shape struct {
	// Go is the value's type, which a list, map or object is described from.
	Go reflect.Type
	// Choices are an enum's values; nil for any other type.
	Choices []string
	// Min and Max are the bounds as an operator writes them, or "".
	Min, Max  string
	AllowNone bool
	// DefaultJSON is a list, map or object default as JSON, which YAML reads
	// too. A scalar's DefaultText is already the value, so it is "" there.
	DefaultJSON string
}

type choiceLister interface{ choices() []string }

// Shape describes the value. It fails only on a default JSON cannot encode.
func (s *Setting[T]) Shape() (Shape, error) {
	out := Shape{Go: reflect.TypeFor[T](), AllowNone: s.spec.AllowNone}
	if c, ok := s.spec.Type.(choiceLister); ok {
		out.Choices = c.choices()
	}
	if limit, ok := s.spec.Min.Get(); ok {
		out.Min = s.spec.Type.Format(limit)
	}
	if limit, ok := s.spec.Max.Get(); ok {
		out.Max = s.spec.Type.Format(limit)
	}
	def, ok := s.spec.Default.Get()
	if !ok || !compound(out.Go) {
		return out, nil
	}
	raw, err := json.Marshal(def)
	if err != nil {
		return Shape{}, fmt.Errorf("config: %s: encoding the default as JSON: %w", s.spec.Key, err)
	}
	out.DefaultJSON = string(raw)
	return out, nil
}

// compound is a value YAML writes as a sequence or a mapping.
func compound(t reflect.Type) bool {
	switch t.Kind() {
	case reflect.Slice, reflect.Array, reflect.Map, reflect.Struct, reflect.Pointer:
		return true
	default:
		return false
	}
}
