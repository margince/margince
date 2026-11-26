// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

// Reading an operation's SCHEMAS out of the merged contract: where a method's
// arguments live, and how a declared schema becomes the standalone JSON literal
// the composed program carries.
//
// Split from extverbs.go, which reads an operation's IDENTITY and GOVERNANCE.
// The two answer different questions and fail in different ways — a bad route is
// a namespace violation, an unresolvable $ref is an argument shape no client can
// use — and together they were over the file-length cap.

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"gopkg.in/yaml.v3"

	"github.com/margince/margince/backend/pkg/extension"
)

// argumentSchema reads the operation's argument shape from wherever the method
// says it lives, and refuses it in the other place. The seam reads a body or a
// query, never both. Arguments on the other side would be published and then
// dropped on every call. Path arguments join either, as plain properties.
func argumentSchema(method, route string, body, params *yaml.Node) (json.RawMessage, error) {
	declared, err := readParameters(params)
	if err != nil {
		return nil, err
	}
	if err := checkPathParameters(route, declared); err != nil {
		return nil, err
	}
	if extension.CarriesBody(method) {
		if declared.query > 0 {
			return nil, fmt.Errorf("the operation declares %s and also declares query parameters — a %s carries its arguments in the body, so the parameters would be published and never read. Move them into the requestBody schema, or declare the operation GET", method, method)
		}
		schema, err := requestSchema(body)
		if err != nil || len(declared.path) == 0 {
			return schema, err
		}
		return withPathProperties(schema, declared)
	}
	if !body.IsZero() {
		return nil, fmt.Errorf("the operation declares %s and also declares a requestBody — a %s carries no body, so the schema would be published and never read. Declare the arguments as query parameters, or declare the operation POST", method, method)
	}
	return querySchema(declared)
}

// parameters are an operation's declared path and query parameters, as the
// properties of the tool's input schema.
type parameters struct {
	properties map[string]json.RawMessage
	required   []string
	path       []string
	query      int
}

// readParameters reads every declared parameter. `header` and `cookie` are
// refused, because a tool's arguments are its arguments: transport metadata is
// not something a model may be handed as an input field.
func readParameters(params *yaml.Node) (parameters, error) {
	declared := parameters{properties: map[string]json.RawMessage{}}
	if params.IsZero() {
		return declared, nil
	}
	if params.Kind != yaml.SequenceNode {
		return parameters{}, errors.New("the operation's parameters is not a list")
	}
	for _, param := range params.Content {
		var decl struct {
			Name     string `yaml:"name"`
			In       string `yaml:"in"`
			Required bool   `yaml:"required"`
		}
		if err := param.Decode(&decl); err != nil {
			return parameters{}, fmt.Errorf("the operation declares a parameter this reader cannot decode: %w", err)
		}
		switch {
		case decl.In == "path" && !decl.Required:
			return parameters{}, fmt.Errorf("path parameter %q is not required — a path segment is always present, so declare it required: true", decl.Name)
		case decl.In == "path":
			declared.path = append(declared.path, decl.Name)
		case decl.In == "query":
			declared.query++
		default:
			return parameters{}, fmt.Errorf("parameter %q is declared in %q — an extension operation takes its arguments from the path, the query string or the body", decl.Name, decl.In)
		}
		if decl.Name == "" {
			return parameters{}, errors.New("the operation declares a parameter with no name")
		}
		if _, dup := declared.properties[decl.Name]; dup {
			// json.Marshal would silently keep one of the two, and the published
			// schema would describe an argument set no reader of the contract wrote.
			return parameters{}, fmt.Errorf("the operation declares the parameter %q twice", decl.Name)
		}
		node := yamlChild(param, "schema")
		if node == nil {
			return parameters{}, fmt.Errorf("parameter %q declares no schema — the seam coerces a path or query value against its declared type, so an untyped one could not be decoded", decl.Name)
		}
		// Through jsonSchema for the $ref refusal, which matters here for the same
		// reason it matters for a body: the emitted literal is a standalone schema
		// and this generator resolves nothing.
		encoded, err := jsonSchema("parameter "+decl.Name, node)
		if err != nil {
			return parameters{}, err
		}
		declared.properties[decl.Name] = encoded
		if decl.Required {
			declared.required = append(declared.required, decl.Name)
		}
	}
	// Sorted, because `required` is a LIST and YAML declaration order would
	// otherwise reach the emitted literal and the manifest digest — making a
	// reordering of the contract look like a changed argument contract.
	slices.Sort(declared.required)
	return declared, nil
}

// checkPathParameters refuses a path parameter the route does not carry. A
// `{name}` the parameters do not declare is Verb.Validate's to refuse, since
// the input schema then lacks the argument.
func checkPathParameters(route string, declared parameters) error {
	routed := extension.Verb{Route: route}.PathArguments()
	for _, name := range declared.path {
		if !slices.Contains(routed, name) {
			return fmt.Errorf("path parameter %q is not a {%s} segment of %s", name, name, route)
		}
	}
	return nil
}

// querySchema assembles a bodyless operation's input schema from its declared
// parameters. `additionalProperties` is false because the serving seam refuses
// a query key nothing declared. With no parameters it is the empty object, not
// nil, so a model is told the tool takes no arguments.
func querySchema(declared parameters) (json.RawMessage, error) {
	schema := struct {
		Type                 string                     `json:"type"`
		Properties           map[string]json.RawMessage `json:"properties,omitempty"`
		Required             []string                   `json:"required,omitempty"`
		AdditionalProperties bool                       `json:"additionalProperties"`
	}{Type: "object", Properties: declared.properties, Required: declared.required}
	return json.Marshal(schema)
}

// withPathProperties adds a body operation's path arguments to its request
// schema, so the tool takes them as properties of one arguments object. A body
// property of the same name is refused: the served route could not tell which
// of the two values the caller meant.
func withPathProperties(schema json.RawMessage, declared parameters) (json.RawMessage, error) {
	var root map[string]json.RawMessage
	if err := json.Unmarshal(schema, &root); err != nil {
		return nil, fmt.Errorf("the requestBody schema cannot take path arguments: %w", err)
	}
	properties := map[string]json.RawMessage{}
	if raw, ok := root["properties"]; ok {
		if err := json.Unmarshal(raw, &properties); err != nil {
			return nil, fmt.Errorf("the requestBody schema's properties cannot be read: %w", err)
		}
	}
	var required []string
	if raw, ok := root["required"]; ok {
		if err := json.Unmarshal(raw, &required); err != nil {
			return nil, fmt.Errorf("the requestBody schema's required cannot be read: %w", err)
		}
	}
	for _, name := range declared.path {
		if _, shadowed := properties[name]; shadowed {
			return nil, fmt.Errorf("the requestBody declares %q, which is also a path parameter — rename one of the two", name)
		}
		properties[name] = declared.properties[name]
		required = append(required, name)
	}
	slices.Sort(required)
	var err error
	if root["properties"], err = json.Marshal(properties); err != nil {
		return nil, err
	}
	if root["required"], err = json.Marshal(required); err != nil {
		return nil, err
	}
	return json.Marshal(root)
}

// requestSchema reads the operation's inline JSON request schema. A $ref is
// refused BY NAME rather than resolved: this generator does not walk
// references, and a silently unresolved one would advertise `{"$ref": …}` to a
// model as the tool's argument shape.
func requestSchema(body *yaml.Node) (json.RawMessage, error) {
	if body.IsZero() {
		return nil, errors.New("the operation declares no requestBody — a body-carrying extension operation is a tool invocation and its arguments are the body")
	}
	schema := yamlChild(yamlChild(yamlChild(body, "content"), "application/json"), "schema")
	if schema == nil {
		return nil, errors.New("the operation's requestBody declares no application/json schema")
	}
	return jsonSchema("requestBody", schema)
}

// responseSchema reads the 200 response's inline JSON schema. Absent is
// allowed — a tool may return nothing describable — but a $ref is refused for
// the same reason as the request's.
func responseSchema(responses *yaml.Node) (json.RawMessage, error) {
	schema := yamlChild(yamlChild(yamlChild(yamlChild(responses, "200"), "content"), "application/json"), "schema")
	if schema == nil {
		return nil, nil
	}
	return jsonSchema("response 200", schema)
}

func jsonSchema(where string, node *yaml.Node) (json.RawMessage, error) {
	// Recursive, not a check on the root. The emitted literal IS the standalone
	// schema an MCP client hands a model, so a `$ref` anywhere inside it — one
	// property, one array's items, one branch of a oneOf — arrives as an
	// argument shape nothing can resolve: the client has no document to resolve
	// it against, and this generator does not walk references to inline it. The
	// root-only check refused the obvious spelling and passed every nested one,
	// which is the shape a real fragment is more likely to have.
	if path, found := findRef(node, ""); found {
		return nil, fmt.Errorf("%s schema declares a $ref at %s — declare an extension operation's schema inline; this generator does not resolve references, and an unresolved one would be advertised to a model as the argument shape", where, path)
	}
	var doc any
	if err := node.Decode(&doc); err != nil {
		return nil, err
	}
	// Marshalled with sorted keys by encoding/json's map handling, so the
	// emitted literal and the hashed bytes are stable across YAML key order.
	encoded, err := json.Marshal(doc)
	if err != nil {
		return nil, fmt.Errorf("%s schema is not expressible as JSON: %w", where, err)
	}
	return encoded, nil
}

// schemaDataKeywords hold INSTANCE data, not subschemas. A `$ref` under any of
// them is a property of the example or the default value being described — a
// perfectly ordinary object member that happens to be spelled `$ref` — and
// refusing it would refuse a schema that references nothing.
var schemaDataKeywords = map[string]bool{
	"example": true, "examples": true, "default": true, "const": true, "enum": true,
}

// namedSubschemaKeywords hold subschemas keyed by an AUTHOR-CHOSEN name. The
// level below them is a set of names, so `properties.$ref` is a property called
// `$ref` and not a reference; the level below THAT is a schema again.
//
// `dependentSchemas` belongs here for exactly the reason `properties` does: in
// 2020-12 (the dialect an openapi 3.1 contract carries) it maps PROPERTY NAMES
// to schemas, so a schema conditioned on a property literally named `$ref` was
// being refused as an unresolved reference — a correct fragment rejected for
// the name one of its properties happens to have.
var namedSubschemaKeywords = map[string]bool{
	"properties": true, "patternProperties": true, "$defs": true, "definitions": true,
	"dependentSchemas": true,
}

// findRef walks a SCHEMA node for a `$ref` at any depth, returning the path to
// the first one so the refusal names a position rather than a document. The
// path is written the way a reader would say it out loud —
// `.properties.deal.$ref`, `.allOf[0].$ref` — because the point of naming it is
// that the author can go to the line.
//
// It is a schema walk rather than a document walk, and the distinction is what
// keeps it from refusing correct fragments: a bare recursive search for the key
// `$ref` also finds a PROPERTY named `$ref` and a `$ref` member inside an
// `example`, neither of which is a reference to anything. The two keyword sets
// above are what tell those apart.
func findRef(node *yaml.Node, path string) (string, bool) {
	if node == nil {
		return "", false
	}
	switch node.Kind {
	case yaml.MappingNode:
		for i := 0; i+1 < len(node.Content); i += 2 {
			key, value := node.Content[i].Value, node.Content[i+1]
			switch {
			case key == "$ref":
				return path + ".$ref", true
			case schemaDataKeywords[key]:
				continue
			case namedSubschemaKeywords[key] && value.Kind == yaml.MappingNode:
				for j := 0; j+1 < len(value.Content); j += 2 {
					if found, ok := findRef(value.Content[j+1], path+"."+key+"."+value.Content[j].Value); ok {
						return found, true
					}
				}
			default:
				if found, ok := findRef(value, path+"."+key); ok {
					return found, true
				}
			}
		}
	case yaml.SequenceNode:
		for i, child := range node.Content {
			if found, ok := findRef(child, fmt.Sprintf("%s[%d]", path, i)); ok {
				return found, true
			}
		}
	}
	return "", false
}
