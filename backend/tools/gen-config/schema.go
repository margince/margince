// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/platform/deployconfig"
)

const routingRef = "#/$defs/aiRouting"

// routingPaths are the two places the model binding is written. Its shape is
// the hand-written $defs, never a reflection of the Go type. The seed is a raw
// node, and the setting's type decodes through its own parser.
var routingPaths = map[string]bool{"seeds.ai_routing": true, "ai.routing": true}

// member is one property of a JSON object, which keeps its members in the
// order they were added so the schema reads in key order.
type member struct {
	name  string
	value json.RawMessage
}

type object []member

func (o object) with(name string, value json.RawMessage) object {
	return append(o, member{name, value})
}

func (o object) raw() json.RawMessage {
	var b bytes.Buffer
	b.WriteByte('{')
	for i, m := range o {
		if i > 0 {
			b.WriteByte(',')
		}
		b.Write(lit(m.name))
		b.WriteByte(':')
		b.Write(m.value)
	}
	b.WriteByte('}')
	return b.Bytes()
}

// lit encodes a string, a list of them or a bool. encoding/json fails on none
// of these, so a failure is a broken toolchain and stops the generator.
func lit[T string | []string | bool](v T) json.RawMessage {
	raw, err := json.Marshal(v)
	if err != nil {
		panic(fmt.Sprintf("gen-config: encoding %v: %v", v, err))
	}
	return raw
}

// number is a bound written as a JSON number.
func number(path, text string) (json.RawMessage, error) {
	if !json.Valid([]byte(text)) {
		return nil, fmt.Errorf("gen-config: %s: the bound %q is not a JSON number", path, text)
	}
	return json.RawMessage(text), nil
}

// schemaJSON is the whole margince.yaml schema: the bootstrap roots, then every
// setting under its section, then the routing $defs.
func schemaJSON(settings []setting, routing json.RawMessage) ([]byte, error) {
	props, err := fieldsOf(reflect.TypeFor[deployconfig.Bootstrap](), "", false)
	if err != nil {
		return nil, err
	}
	props, err = withSettings(props, settings)
	if err != nil {
		return nil, err
	}
	root := object{}.
		with("$schema", lit("https://json-schema.org/draft/2020-12/schema")).
		with("$id", lit("https://margince.dev/schema/margince.json")).
		with("title", lit("Margince installation config (margince.yaml)")).
		with("$comment", lit("Written by tools/gen-config from the settings registry; do not edit.")).
		with("type", lit("object")).
		with("additionalProperties", lit(false)).
		with("properties", props.raw()).
		with("$defs", routing).raw()
	var out bytes.Buffer
	if err := json.Indent(&out, root, "", "  "); err != nil {
		return nil, fmt.Errorf("gen-config: indenting the schema: %w", err)
	}
	out.WriteByte('\n')
	return out.Bytes(), nil
}

// sectionNode is a branch of the setting tree: a YAML mapping of further keys.
type sectionNode struct {
	order    []string
	children map[string]*sectionNode
	leaf     *setting
}

// withSettings appends each top-level section, and each top-level key, to the
// bootstrap roots.
func withSettings(props object, settings []setting) (object, error) {
	tree := &sectionNode{children: map[string]*sectionNode{}}
	for i := range settings {
		if err := tree.insert(strings.Split(settings[i].Key(), "."), &settings[i]); err != nil {
			return nil, err
		}
	}
	for _, name := range tree.order {
		raw, err := tree.children[name].schema(name)
		if err != nil {
			return nil, err
		}
		props = props.with(name, raw)
	}
	return props, nil
}

func (n *sectionNode) insert(parts []string, s *setting) error {
	child, ok := n.children[parts[0]]
	if !ok {
		child = &sectionNode{children: map[string]*sectionNode{}}
		n.children[parts[0]] = child
		n.order = append(n.order, parts[0])
	}
	if len(parts) == 1 {
		if len(child.order) > 0 {
			return fmt.Errorf("gen-config: %s is a setting and a section of others", s.Key())
		}
		child.leaf = s
		return nil
	}
	if child.leaf != nil {
		return fmt.Errorf("gen-config: %s is a setting and a section of others", child.leaf.Key())
	}
	return child.insert(parts[1:], s)
}

// schema is a section, a mapping a file may also leave null, or a setting.
func (n *sectionNode) schema(path string) (json.RawMessage, error) {
	if n.leaf != nil {
		return settingSchema(*n.leaf)
	}
	var props object
	for _, name := range n.order {
		raw, err := n.children[name].schema(path + "." + name)
		if err != nil {
			return nil, err
		}
		props = props.with(name, raw)
	}
	return object{}.
		with("type", lit([]string{"object", "null"})).
		with("additionalProperties", lit(false)).
		with("properties", props.raw()).raw(), nil
}

// settingSchema describes one setting. Every setting accepts null, which the
// loader reads as the key left unwritten.
func settingSchema(s setting) (json.RawMessage, error) {
	head := object{}.with("description", lit(s.Doc()))
	if s.Secret() {
		return secretSchema(head, s.shape.AllowNone).raw(), nil
	}
	body, err := valueSchema(s)
	if err != nil {
		return nil, err
	}
	body = nullable(body)
	def, ok, err := schemaDefault(s)
	if err != nil {
		return nil, err
	}
	if ok {
		body = body.with("default", def)
	}
	return append(head, body...).raw(), nil
}

// secretSchema takes a reference and never a literal, as the loader does.
func secretSchema(head object, allowNone bool) object {
	pattern := deployconfig.SecretRefPattern
	examples := []string{"${env:VARIABLE_NAME}", "${file:/run/secrets/name}"}
	if allowNone {
		pattern += `|^\s*\$\{none\}\s*$`
		examples = append(examples, "${none}")
	}
	return head.
		with("type", lit([]string{"string", "null"})).
		with("pattern", lit(pattern)).
		with("examples", lit(examples))
}

func valueSchema(s setting) (object, error) {
	switch {
	case routingPaths[s.Key()]:
		return object{}.with("$ref", lit(routingRef)), nil
	case s.shape.Choices != nil:
		return object{}.with("type", lit("string")).with("enum", lit(s.shape.Choices)), nil
	case s.TypeName() == config.Duration.Name():
		return object{}.with("type", lit("string")), nil
	case s.TypeName() == config.NonEmpty.Name():
		return object{}.with("type", lit("string")).with("minLength", json.RawMessage("1")), nil
	}
	body, err := typeSchema(s.shape.Go, s.Key(), false)
	if err != nil || !numeric(s.shape.Go) {
		return body, err
	}
	for _, bound := range []struct{ keyword, text string }{{"minimum", s.shape.Min}, {"maximum", s.shape.Max}} {
		if bound.text == "" {
			continue
		}
		raw, err := number(s.Key(), bound.text)
		if err != nil {
			return nil, err
		}
		body = body.with(bound.keyword, raw)
	}
	return body, nil
}

// schemaDefault is the default as the JSON value a file would write.
func schemaDefault(s setting) (json.RawMessage, bool, error) {
	text, ok := s.DefaultText()
	switch {
	case !ok:
		return nil, false, nil
	case s.shape.DefaultJSON != "":
		return json.RawMessage(s.shape.DefaultJSON), true, nil
	case textual(s):
		return lit(text), true, nil
	case !json.Valid([]byte(text)):
		return nil, false, fmt.Errorf("gen-config: %s: the default %q is not a JSON value", s.Key(), text)
	default:
		return json.RawMessage(text), true, nil
	}
}

// nullable lets a value be null too. A single type takes "null" beside it;
// anything else, an enum or a $ref among them, becomes one of two branches.
func nullable(body object) object {
	at := slices.IndexFunc(body, func(m member) bool { return m.name == "type" })
	enum := slices.ContainsFunc(body, func(m member) bool { return m.name == "enum" })
	var single string
	if at < 0 || enum || json.Unmarshal(body[at].value, &single) != nil {
		return object{}.with("anyOf", json.RawMessage(`[`+string(body.raw())+`,{"type":"null"}]`))
	}
	body[at].value = lit([]string{single, "null"})
	return body
}

func numeric(t reflect.Type) bool {
	switch t.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return true
	default:
		return false
	}
}

// typeSchema describes a Go value the way the loader decodes it. A struct that
// decodes itself from JSON is read from YAML through that decoder. Its fields
// are then named by their json tags, from there down.
func typeSchema(t reflect.Type, path string, jsonNames bool) (object, error) {
	switch {
	case routingPaths[path]:
		return object{}.with("$ref", lit(routingRef)), nil
	case t == reflect.TypeFor[deployconfig.Secret]():
		return secretSchema(object{}, false), nil
	case t == reflect.TypeFor[yaml.Node]():
		return nil, fmt.Errorf("gen-config: %s is a raw YAML node with no declared shape", path)
	case t == reflect.TypeFor[ai.RoutingConfig]():
		return nil, fmt.Errorf("gen-config: %s holds a model binding outside the paths the schema knows", path)
	}
	switch t.Kind() {
	case reflect.Pointer:
		inner, err := typeSchema(t.Elem(), path, jsonNames)
		if err != nil {
			return nil, err
		}
		return nullable(inner), nil
	case reflect.Bool:
		return object{}.with("type", lit("boolean")), nil
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return object{}.with("type", lit("integer")), nil
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return object{}.with("type", lit("integer")).with("minimum", json.RawMessage("0")), nil
	case reflect.Float32, reflect.Float64:
		return object{}.with("type", lit("number")), nil
	case reflect.String:
		return object{}.with("type", lit("string")), nil
	case reflect.Slice, reflect.Array:
		return containerSchema("array", "items", t.Elem(), path+"[]", jsonNames)
	case reflect.Map:
		if t.Key().Kind() != reflect.String {
			return nil, fmt.Errorf("gen-config: %s is a map keyed by %s, which YAML cannot name", path, t.Key())
		}
		return containerSchema("object", "additionalProperties", t.Elem(), path+".*", jsonNames)
	case reflect.Struct:
		return structSchema(t, path, jsonNames)
	default:
		return nil, fmt.Errorf("gen-config: %s is a %s, which no source can write", path, t.Kind())
	}
}

func containerSchema(kind, keyword string, elem reflect.Type, path string, jsonNames bool) (object, error) {
	inner, err := typeSchema(elem, path, jsonNames)
	if err != nil {
		return nil, err
	}
	return object{}.with("type", lit(kind)).with(keyword, inner.raw()), nil
}

func structSchema(t reflect.Type, path string, jsonNames bool) (object, error) {
	if reflect.PointerTo(t).Implements(reflect.TypeFor[json.Unmarshaler]()) {
		jsonNames = true
	}
	props, err := fieldsOf(t, path, jsonNames)
	if err != nil {
		return nil, err
	}
	return object{}.
		with("type", lit("object")).
		with("additionalProperties", lit(false)).
		with("properties", props.raw()), nil
}

// fieldsOf names each exported field as the decoder that reads it does.
func fieldsOf(t reflect.Type, path string, jsonNames bool) (object, error) {
	var props object
	for f := range t.Fields() {
		if !f.IsExported() {
			continue
		}
		name, err := fieldName(f, jsonNames)
		if err != nil {
			return nil, fmt.Errorf("gen-config: %s.%s: %w", path, f.Name, err)
		}
		if name == "" {
			continue
		}
		here := strings.TrimPrefix(path+"."+name, ".")
		body, err := typeSchema(f.Type, here, jsonNames)
		if err != nil {
			return nil, err
		}
		props = props.with(name, body.raw())
	}
	return props, nil
}

var errInline = errors.New("an inline field has no name of its own; describe it before generating")

// fieldName is the key a decoder reads the field from. That is the tag's name,
// else the Go name as encoding/json spells it, or lowercased as yaml.v3 does.
func fieldName(f reflect.StructField, jsonNames bool) (string, error) {
	tagKey, fallback := "yaml", strings.ToLower(f.Name)
	if jsonNames {
		tagKey, fallback = "json", f.Name
	}
	name, opts, _ := strings.Cut(f.Tag.Get(tagKey), ",")
	switch {
	case name == "-":
		return "", nil
	case strings.Contains(","+opts+",", ",inline,"):
		return "", errInline
	case name == "":
		return fallback, nil
	default:
		return name, nil
	}
}

// routingGoFile embeds the routing $defs in the ai module, which serves them
// at GET /ai/routing/schema.
func routingGoFile(routing json.RawMessage) ([]byte, error) {
	var compact bytes.Buffer
	if err := json.Compact(&compact, routing); err != nil {
		return nil, fmt.Errorf("gen-config: compacting the routing $defs: %w", err)
	}
	return fmt.Appendf(nil, "// Code generated by gen-config; DO NOT EDIT.\n\npackage ai\n\n"+
		"// routingSchemaJSON is the routing $defs of config/margince.schema.json.\n"+
		"const routingSchemaJSON = %q\n", compact.String()), nil
}
