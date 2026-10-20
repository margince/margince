// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deployconfig

import (
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/margince/margince/backend/internal/platform/config"
)

// fault is one problem with one key. near is the key an unknown one likely
// meant; problem is empty for an unknown key.
type fault struct {
	key, problem, near string
	line               int
}

var (
	rawNodeType     = reflect.TypeFor[yaml.Node]()
	unmarshalerType = reflect.TypeFor[yaml.Unmarshaler]()
)

// shapeFaults walks one root against the Go type it decodes into. It names
// each key that type has no field for and each value of the wrong kind. The
// strict decoder stops at the first; this finds them all, in file order.
func shapeFaults(key string, n *yaml.Node, t reflect.Type) []fault {
	if n.Kind == yaml.AliasNode && n.Alias != nil {
		n = n.Alias
	}
	if n.Kind == yaml.ScalarNode && n.Tag == "!!null" || t == rawNodeType {
		return nil
	}
	if reflect.PointerTo(t).Implements(unmarshalerType) {
		if err := n.Decode(reflect.New(t).Interface()); err != nil {
			return []fault{{key: key, line: n.Line, problem: strings.TrimPrefix(err.Error(), "deployconfig: ")}}
		}
		return nil
	}
	switch t.Kind() {
	case reflect.Pointer:
		return shapeFaults(key, n, t.Elem())
	case reflect.Struct:
		return structFaults(key, n, t)
	case reflect.Slice:
		return sliceFaults(key, n, t)
	default:
		if n.Kind != yaml.ScalarNode || n.Decode(reflect.New(t).Interface()) != nil {
			return []fault{{key: key, line: n.Line, problem: scalarWant(t)}}
		}
		return nil
	}
}

// sliceFaults names each item by its index, so a fault points at one entry.
func sliceFaults(key string, n *yaml.Node, t reflect.Type) []fault {
	if n.Kind != yaml.SequenceNode {
		return []fault{{key: key, line: n.Line, problem: "want a list"}}
	}
	var out []fault
	for i, item := range n.Content {
		out = append(out, shapeFaults(fmt.Sprintf("%s[%d]", key, i), item, t.Elem())...)
	}
	return out
}

// structFaults reads a `<<:` merge as the keys it brings, as the decoder does.
func structFaults(key string, n *yaml.Node, t reflect.Type) []fault {
	if n.Kind != yaml.MappingNode {
		return []fault{{key: key, line: n.Line, problem: "want a mapping of keys"}}
	}
	fields := yamlFields(t)
	var out []fault
	for i := 0; i+1 < len(n.Content); i += 2 {
		name, value := n.Content[i], n.Content[i+1]
		if name.Value == "<<" {
			out = append(out, mergedFaults(key, value, t)...)
			continue
		}
		here := key + "." + name.Value
		field, ok := fields[name.Value]
		if !ok {
			out = append(out, fault{key: here, line: name.Line, near: nearField(key, name.Value, fields)})
			continue
		}
		out = append(out, shapeFaults(here, value, field)...)
	}
	return out
}

func mergedFaults(key string, merged *yaml.Node, t reflect.Type) []fault {
	if merged.Kind == yaml.AliasNode && merged.Alias != nil {
		merged = merged.Alias
	}
	if merged.Kind != yaml.SequenceNode {
		return structFaults(key, merged, t)
	}
	var out []fault
	for _, each := range merged.Content {
		out = append(out, mergedFaults(key, each, t)...)
	}
	return out
}

// yamlFields maps each key the decoder reads into t to the field's type.
func yamlFields(t reflect.Type) map[string]reflect.Type {
	out := map[string]reflect.Type{}
	for field := range t.Fields() {
		if !field.IsExported() {
			continue
		}
		name := yamlName(field)
		if field.Tag.Get("yaml") == "" {
			name = strings.ToLower(field.Name)
		}
		if name != "" {
			out[name] = field.Type
		}
	}
	return out
}

func scalarWant(t reflect.Type) string {
	switch t.Kind() {
	case reflect.Bool:
		return "want true or false"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return "want a whole number"
	default:
		return "want a single value"
	}
}

// nearField is the key a misspelt field of section likely meant, in full.
func nearField(section, name string, fields map[string]reflect.Type) string {
	if near := config.Suggest(name, slices.Collect(maps.Keys(fields))); near != "" {
		return section + "." + near
	}
	return ""
}
