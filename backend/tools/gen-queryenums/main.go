// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Command gen-queryenums derives, from the contract, which query parameters
// of which route are closed enums, and which integer ones have a minimum. The
// server refuses a value outside the enum, or below the minimum, with one 422
// that names what is allowed. A new parameter of either kind is covered by
// being declared, with no handler code.
package main

import (
	"errors"
	"flag"
	"fmt"
	"go/format"
	"log"
	"os"
	"sort"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
)

// routePrefix is the base path the generated router mounts every operation
// under; the chi pattern a request matches carries it.
const routePrefix = "/v1"

type queryEnum struct {
	Name   string
	Values []string
	// Array says the parameter carries a list; Explode says each member is its
	// own key. Only an array that is not exploded travels as one joined value.
	Array, Explode bool
}

type queryMinimum struct {
	Name    string
	Minimum int64
}

// routeQuery is what the server checks on one route's query string.
type routeQuery struct {
	enums    []queryEnum
	minimums []queryMinimum
}

func main() {
	in := flag.String("in", "", "authoritative crm.yaml")
	out := flag.String("out", "", "generated Go table")
	flag.Parse()
	if *in == "" || *out == "" {
		log.Fatal("gen-queryenums: -in and -out are required")
	}
	doc, err := openapi3.NewLoader().LoadFromFile(*in)
	if err != nil {
		log.Fatalf("gen-queryenums: loading %s: %v", *in, err)
	}
	table, err := deriveTable(doc)
	if err != nil {
		log.Fatalf("gen-queryenums: %v", err)
	}
	formatted, err := format.Source([]byte(render(table)))
	if err != nil {
		log.Fatalf("gen-queryenums: formatting the generated table: %v", err)
	}
	if err := os.WriteFile(*out, formatted, 0o600); err != nil {
		log.Fatalf("gen-queryenums: %v", err)
	}
	fmt.Printf("%d routes with checked query parameters generated\n", len(table))
}

// deriveTable maps "METHOD /v1/path" to the checked query parameters of that
// operation, whatever its method: a DELETE takes a closed scope too.
// An operation-level parameter replaces a path-level one of the same name.
func deriveTable(doc *openapi3.T) (map[string]routeQuery, error) {
	table := map[string]routeQuery{}
	for path, item := range doc.Paths.Map() {
		for method, op := range item.Operations() {
			byName := queryParams(item.Parameters, op.Parameters)
			enums, err := enumsOf(byName)
			if err == nil {
				var minimums []queryMinimum
				minimums, err = minimumsOf(byName)
				if len(enums)+len(minimums) > 0 {
					table[method+" "+routePrefix+path] = routeQuery{enums: enums, minimums: minimums}
				}
			}
			if err != nil {
				return nil, fmt.Errorf("%s %s: %w", method, path, err)
			}
		}
	}
	return table, nil
}

func queryParams(pathLevel, opLevel openapi3.Parameters) map[string]*openapi3.Parameter {
	byName := map[string]*openapi3.Parameter{}
	for _, params := range []openapi3.Parameters{pathLevel, opLevel} {
		for _, ref := range params {
			if ref != nil && ref.Value != nil && ref.Value.In == openapi3.ParameterInQuery {
				byName[ref.Value.Name] = ref.Value
			}
		}
	}
	return byName
}

// minimumsOf reads the minimum of each integer query parameter. A bound the
// table cannot hold is refused: an exclusive one, one on array items, or one
// under allOf, oneOf or anyOf. Skipping it would leave it unchecked.
func minimumsOf(byName map[string]*openapi3.Parameter) ([]queryMinimum, error) {
	var out []queryMinimum
	for name, param := range byName {
		if param.Schema == nil || param.Schema.Value == nil {
			continue
		}
		schema := param.Schema.Value
		if schema.Items != nil && schema.Items.Value != nil && schema.Items.Value.Min != nil {
			return nil, fmt.Errorf("query parameter %q: a minimum on array items is not checked; declare the parameter without one", name)
		}
		if hidesABound(schema) {
			return nil, fmt.Errorf("query parameter %q: a minimum reached through allOf, oneOf or anyOf; declare it on the schema itself", name)
		}
		if schema.Min == nil || !schema.Type.Is(openapi3.TypeInteger) {
			continue
		}
		if schema.ExclusiveMin.IsSet() {
			return nil, fmt.Errorf("query parameter %q: an exclusive minimum is not checked; declare an inclusive one", name)
		}
		out = append(out, queryMinimum{Name: name, Minimum: int64(*schema.Min)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// hidesABound reports a minimum under a composition keyword.
func hidesABound(schema *openapi3.Schema) bool {
	for _, group := range []openapi3.SchemaRefs{schema.AllOf, schema.OneOf, schema.AnyOf} {
		for _, member := range group {
			if member != nil && member.Value != nil && (member.Value.Min != nil || hidesABound(member.Value)) {
				return true
			}
		}
	}
	return false
}

func enumsOf(byName map[string]*openapi3.Parameter) ([]queryEnum, error) {
	var out []queryEnum
	for name, param := range byName {
		values, array, err := enumValues(param.Schema)
		if err != nil {
			return nil, fmt.Errorf("query parameter %q: %w", name, err)
		}
		if len(values) > 0 {
			// A form-style query parameter explodes unless it says otherwise.
			explode := param.Explode == nil || *param.Explode
			out = append(out, queryEnum{Name: name, Values: values, Array: array, Explode: explode})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// enumValues reads the closed string set off a parameter's schema, or its
// items when the parameter is an array. A set with a non-string member is not
// a vocabulary this table can compare against. An enum under allOf, oneOf or
// anyOf cannot be read, so it is refused. Skipping it would leave the
// parameter unchecked, silently.
func enumValues(ref *openapi3.SchemaRef) (values []string, array bool, err error) {
	if ref == nil || ref.Value == nil {
		return nil, false, nil
	}
	schema := ref.Value
	if schema.Items != nil && schema.Items.Value != nil {
		schema, array = schema.Items.Value, true
	}
	if hidesAnEnum(schema) {
		return nil, false, errors.New("an enum reached through allOf, oneOf or anyOf; declare it on the schema itself")
	}
	values = make([]string, 0, len(schema.Enum))
	for _, raw := range schema.Enum {
		s, ok := raw.(string)
		if !ok {
			return nil, false, nil
		}
		values = append(values, s)
	}
	return values, array, nil
}

// hidesAnEnum reports an enum that sits under a composition keyword rather
// than on the schema itself.
func hidesAnEnum(schema *openapi3.Schema) bool {
	if len(schema.Enum) > 0 {
		return false
	}
	for _, group := range []openapi3.SchemaRefs{schema.AllOf, schema.OneOf, schema.AnyOf} {
		for _, member := range group {
			if member != nil && member.Value != nil && (len(member.Value.Enum) > 0 || hidesAnEnum(member.Value)) {
				return true
			}
		}
	}
	return false
}

func render(table map[string]routeQuery) string {
	routes := make([]string, 0, len(table))
	for route := range table {
		routes = append(routes, route)
	}
	sort.Strings(routes)

	var b strings.Builder
	b.WriteString("// Code generated by tools/gen-queryenums from api/crm.yaml. DO NOT EDIT.\n\n")
	b.WriteString("package compose\n\n")
	b.WriteString("// queryEnumTable maps \"METHOD /v1/path\" to the query parameters the contract\n")
	b.WriteString("// declares as closed enums, with the values each admits.\n")
	b.WriteString("var queryEnumTable = map[string][]queryEnum{\n")
	for _, route := range routes {
		if len(table[route].enums) == 0 {
			continue
		}
		fmt.Fprintf(&b, "\t%q: {\n", route)
		for _, e := range table[route].enums {
			quoted := make([]string, len(e.Values))
			for i, v := range e.Values {
				quoted[i] = fmt.Sprintf("%q", v)
			}
			fmt.Fprintf(&b, "\t\t{Name: %q, Values: []string{%s}, Array: %t, Explode: %t},\n",
				e.Name, strings.Join(quoted, ", "), e.Array, e.Explode)
		}
		b.WriteString("\t},\n")
	}
	b.WriteString("}\n\n")
	b.WriteString("// queryMinimumTable maps \"METHOD /v1/path\" to the integer query parameters\n")
	b.WriteString("// the contract gives a minimum, with that minimum.\n")
	b.WriteString("var queryMinimumTable = map[string][]queryMinimum{\n")
	for _, route := range routes {
		if len(table[route].minimums) == 0 {
			continue
		}
		fmt.Fprintf(&b, "\t%q: {\n", route)
		for _, m := range table[route].minimums {
			fmt.Fprintf(&b, "\t\t{Name: %q, Minimum: %d},\n", m.Name, m.Minimum)
		}
		b.WriteString("\t},\n")
	}
	b.WriteString("}\n")
	return b.String()
}
