// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package config

import (
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"gopkg.in/yaml.v3"
)

// Type parses, decodes, formats and orders the values of one Go type.
type Type[T any] interface {
	Name() string
	Parse(text string) (T, error)
	DecodeYAML(n *yaml.Node) (T, error)
	DecodeJSON(raw json.RawMessage) (T, error)
	Format(v T) string
	Less(a, b T) (less, ordered bool)
}

// textType backs the scalar types. YAML and JSON carry the same text a flag does, so
// "90s" or "0x10" means one thing in every source.
type textType[T any] struct {
	name    string
	parse   func(string) (T, error)
	format  func(T) string
	compare func(a, b T) int
	// enumValues are the only values an Enum admits; nil for every other type.
	enumValues []string
}

func (t textType[T]) Name() string              { return t.name }
func (t textType[T]) Parse(s string) (T, error) { return t.parse(s) }
func (t textType[T]) Format(v T) string         { return t.format(v) }
func (t textType[T]) choices() []string         { return t.enumValues }

func (t textType[T]) DecodeYAML(n *yaml.Node) (T, error) {
	var zero T
	if n.Kind != yaml.ScalarNode {
		return zero, fmt.Errorf("want a single %s value, got a %s", t.name, nodeKind(n))
	}
	return t.parse(n.Value)
}

// A JSON string is unquoted; any other JSON token (a number, a bool) is taken
// as the text it was written as.
func (t textType[T]) DecodeJSON(raw json.RawMessage) (T, error) {
	var text string
	if err := json.Unmarshal(raw, &text); err == nil {
		return t.parse(text)
	}
	return t.parse(string(raw))
}

func (t textType[T]) Less(a, b T) (bool, bool) {
	if t.compare == nil {
		return false, false
	}
	return t.compare(a, b) < 0, true
}

func signed[T ~int | ~int8 | ~int16 | ~int32 | ~int64](name string, bits int) Type[T] {
	return textType[T]{
		name: name, compare: cmp.Compare[T],
		parse: func(s string) (T, error) {
			n, err := strconv.ParseInt(s, 0, bits)
			return T(n), numErr(name, err)
		},
		format: func(v T) string { return strconv.FormatInt(int64(v), 10) },
	}
}

func unsigned[T ~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64](name string, bits int) Type[T] {
	return textType[T]{
		name: name, compare: cmp.Compare[T],
		parse: func(s string) (T, error) {
			n, err := strconv.ParseUint(s, 0, bits)
			return T(n), numErr(name, err)
		},
		format: func(v T) string { return strconv.FormatUint(uint64(v), 10) },
	}
}

func float[T ~float32 | ~float64](name string, bits int) Type[T] {
	return textType[T]{
		name: name, compare: cmp.Compare[T],
		parse: func(s string) (T, error) {
			n, err := strconv.ParseFloat(s, bits)
			return T(n), numErr(name, err)
		},
		format: func(v T) string { return strconv.FormatFloat(float64(v), 'g', -1, bits) },
	}
}

// numErr never includes the text it failed on, since a setting can hold a
// secret. The type's name is what an operator needs to act on.
func numErr(name string, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, strconv.ErrRange) {
		return fmt.Errorf("out of range for %s", name)
	}
	return fmt.Errorf("not a valid %s", name)
}

// The scalar types. Each parses the same text from a flag, env, YAML or JSON.
var (
	Int   = signed[int]("int", strconv.IntSize)
	Int8  = signed[int8]("int8", 8)
	Int16 = signed[int16]("int16", 16)
	Int32 = signed[int32]("int32", 32)
	Int64 = signed[int64]("int64", 64)

	Uint   = unsigned[uint]("uint", strconv.IntSize)
	Uint8  = unsigned[uint8]("uint8", 8)
	Uint16 = unsigned[uint16]("uint16", 16)
	Uint32 = unsigned[uint32]("uint32", 32)
	Uint64 = unsigned[uint64]("uint64", 64)

	Float32 = float[float32]("float32", 32)
	Float64 = float[float64]("float64", 64)

	Bool = textType[bool]{
		name: "bool",
		parse: func(s string) (bool, error) {
			b, err := strconv.ParseBool(s)
			if err != nil {
				return false, errors.New("want true or false")
			}
			return b, nil
		},
		format: strconv.FormatBool,
	}

	String = textType[string]{
		name:   "string",
		parse:  func(s string) (string, error) { return s, nil },
		format: func(s string) string { return s },
	}

	NonEmpty = textType[string]{
		name: "non-empty string",
		parse: func(s string) (string, error) {
			if s == "" {
				return "", errors.New("must not be empty")
			}
			return s, nil
		},
		format: func(s string) string { return s },
	}

	Duration = textType[time.Duration]{
		name: "duration", compare: cmp.Compare[time.Duration],
		parse: func(s string) (time.Duration, error) {
			d, err := time.ParseDuration(s)
			if err != nil {
				return 0, errors.New(`want a duration such as "90s" or "15m"`)
			}
			return d, nil
		},
		format: time.Duration.String,
	}
)

func nodeKind(n *yaml.Node) string {
	switch n.Kind {
	case yaml.SequenceNode:
		return "list"
	case yaml.MappingNode:
		return "mapping"
	default:
		return "value"
	}
}
