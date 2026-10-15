// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"
)

// Enum admits only the listed values.
func Enum[E ~string](values ...E) Type[E] {
	names := make([]string, len(values))
	for i, v := range values {
		names[i] = string(v)
	}
	name := "enum(" + strings.Join(names, "|") + ")"
	return textType[E]{
		name: name,
		parse: func(s string) (E, error) {
			if slices.Contains(values, E(s)) {
				return E(s), nil
			}
			return "", fmt.Errorf("want one of %s", strings.Join(names, ", "))
		},
		format: func(v E) string { return string(v) },
	}
}

type listType[T any] struct{ elem Type[T] }

// List is a sequence of elem values.
func List[T any](elem Type[T]) Type[[]T] { return listType[T]{elem: elem} }

func (l listType[T]) Name() string { return "list<" + l.elem.Name() + ">" }

func (l listType[T]) Parse(s string) ([]T, error) {
	if s == "" {
		return []T{}, nil
	}
	parts := splitEscaped(s, ',')
	out := make([]T, 0, len(parts))
	for i, p := range parts {
		v, err := l.elem.Parse(p)
		if err != nil {
			return nil, fmt.Errorf("item %d: %w", i+1, err)
		}
		out = append(out, v)
	}
	return out, nil
}

func (l listType[T]) DecodeYAML(n *yaml.Node) ([]T, error) {
	if n.Kind != yaml.SequenceNode {
		return nil, fmt.Errorf("want a list, got a %s", nodeKind(n))
	}
	out := make([]T, 0, len(n.Content))
	for i, item := range n.Content {
		v, err := l.elem.DecodeYAML(item)
		if err != nil {
			return nil, fmt.Errorf("item %d: %w", i+1, err)
		}
		out = append(out, v)
	}
	return out, nil
}

func (l listType[T]) DecodeJSON(raw json.RawMessage) ([]T, error) {
	var items []json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, errors.New("want a JSON array")
	}
	out := make([]T, 0, len(items))
	for i, item := range items {
		v, err := l.elem.DecodeJSON(item)
		if err != nil {
			return nil, fmt.Errorf("item %d: %w", i+1, err)
		}
		out = append(out, v)
	}
	return out, nil
}

func (l listType[T]) Format(v []T) string {
	parts := make([]string, len(v))
	for i, item := range v {
		parts[i] = escape(l.elem.Format(item), ",")
	}
	return strings.Join(parts, ",")
}

func (l listType[T]) Less(_, _ []T) (bool, bool) { return false, false }

type mapType[T any] struct{ elem Type[T] }

// Map is a string-keyed mapping of elem values.
func Map[T any](elem Type[T]) Type[map[string]T] { return mapType[T]{elem: elem} }

func (m mapType[T]) Name() string { return "map<string," + m.elem.Name() + ">" }

// The entry split keeps escapes so the key/value split sees `\:` and `\,`
// in a key, and only then removes them.
func (m mapType[T]) Parse(s string) (map[string]T, error) {
	out := map[string]T{}
	if s == "" {
		return out, nil
	}
	for _, entry := range splitKeep(s, ',') {
		kv := splitEscaped(entry, ':')
		if len(kv) != 2 {
			return nil, errors.New("want key:value entries")
		}
		if err := m.put(out, kv[0], func() (T, error) { return m.elem.Parse(kv[1]) }); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func (m mapType[T]) DecodeYAML(n *yaml.Node) (map[string]T, error) {
	if n.Kind != yaml.MappingNode {
		return nil, fmt.Errorf("want a mapping, got a %s", nodeKind(n))
	}
	out := map[string]T{}
	for i := 0; i+1 < len(n.Content); i += 2 {
		valueNode := n.Content[i+1]
		if err := m.put(out, n.Content[i].Value, func() (T, error) { return m.elem.DecodeYAML(valueNode) }); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func (m mapType[T]) DecodeJSON(raw json.RawMessage) (map[string]T, error) {
	var entries map[string]json.RawMessage
	if err := json.Unmarshal(raw, &entries); err != nil {
		return nil, errors.New("want a JSON object")
	}
	out := map[string]T{}
	for k, v := range entries {
		if err := m.put(out, k, func() (T, error) { return m.elem.DecodeJSON(v) }); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// put names no key in its errors: a key is input too, and the error text
// reaches logs and the operator's terminal.
func (m mapType[T]) put(out map[string]T, key string, decode func() (T, error)) error {
	if _, dup := out[key]; dup {
		return errors.New("a key is given twice")
	}
	v, err := decode()
	if err != nil {
		return fmt.Errorf("a value: %w", err)
	}
	out[key] = v
	return nil
}

func (m mapType[T]) Format(v map[string]T) string {
	keys := slices.Sorted(maps.Keys(v))
	parts := make([]string, len(keys))
	for i, k := range keys {
		parts[i] = escape(k, ",:") + ":" + escape(m.elem.Format(v[k]), ",:")
	}
	return strings.Join(parts, ",")
}

func (m mapType[T]) Less(_, _ map[string]T) (bool, bool) { return false, false }

type structType[T any] struct{}

// Struct is a value replaced whole by a higher source, never merged field by
// field. Its fields are checked together, so only the whole is valid.
func Struct[T any]() Type[T] { return structType[T]{} }

func (structType[T]) Name() string { return "object" }

func (s structType[T]) Parse(text string) (T, error) { return s.DecodeJSON(json.RawMessage(text)) }

// KnownFields makes YAML refuse an unknown field as JSON does.
func (structType[T]) DecodeYAML(n *yaml.Node) (T, error) {
	var v T
	if n.Kind != yaml.MappingNode {
		return v, fmt.Errorf("want a mapping, got a %s", nodeKind(n))
	}
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	if err := enc.Encode(n); err != nil {
		return v, fmt.Errorf("re-encoding: %w", err)
	}
	if err := enc.Close(); err != nil {
		return v, fmt.Errorf("re-encoding: %w", err)
	}
	dec := yaml.NewDecoder(&buf)
	dec.KnownFields(true)
	if err := dec.Decode(&v); err != nil {
		return v, errStructFields
	}
	return v, nil
}

func (structType[T]) DecodeJSON(raw json.RawMessage) (T, error) {
	var v T
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&v); err != nil {
		return v, errStructFields
	}
	if _, err := dec.Token(); err != io.EOF {
		return v, errStructFields
	}
	return v, nil
}

// errStructFields hides the decoders' messages, which quote the offending
// input, and a struct field can hold a secret.
var errStructFields = errors.New("want an object of this setting's fields")

func (structType[T]) Format(v T) string {
	raw, err := json.Marshal(v)
	if err != nil {
		return "<unencodable>"
	}
	return string(raw)
}

func (structType[T]) Less(_, _ T) (bool, bool) { return false, false }

// splitEscaped splits on sep where it is not preceded by a backslash, and
// removes the escaping backslash of `\,`, `\:` and `\\`.
func splitEscaped(s string, sep rune) []string {
	var parts []string
	var cur strings.Builder
	escaped := false
	for _, r := range s {
		switch {
		case escaped:
			cur.WriteRune(r)
			escaped = false
		case r == '\\':
			escaped = true
		case r == sep:
			parts = append(parts, cur.String())
			cur.Reset()
		default:
			cur.WriteRune(r)
		}
	}
	return append(parts, cur.String())
}

// splitKeep splits like splitEscaped but leaves every escape in place, so a
// later split on another separator still sees them.
func splitKeep(s string, sep rune) []string {
	var parts []string
	var cur strings.Builder
	escaped := false
	for _, r := range s {
		switch {
		case escaped:
			cur.WriteRune('\\')
			cur.WriteRune(r)
			escaped = false
		case r == '\\':
			escaped = true
		case r == sep:
			parts = append(parts, cur.String())
			cur.Reset()
		default:
			cur.WriteRune(r)
		}
	}
	if escaped {
		cur.WriteRune('\\')
	}
	return append(parts, cur.String())
}

func escape(s, specials string) string {
	var b strings.Builder
	for _, r := range s {
		if r == '\\' || strings.ContainsRune(specials, r) {
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
}
