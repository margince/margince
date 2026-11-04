// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"fmt"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// readViewVerb is the tool every read with no verb of its own is served by.
const readViewVerb = "read_view"

// queryParameter is one parameter of an operation, decoded for its name, where
// it sits and the JSON type its schema gives it.
type queryParameter struct {
	Ref    string `yaml:"$ref"`
	Name   string `yaml:"name"`
	In     string `yaml:"in"`
	Schema struct {
		Ref  string `yaml:"$ref"`
		Type any    `yaml:"type"`
	} `yaml:"schema"`
}

// viewQuery is one query parameter of a view's route and its JSON type.
type viewQuery struct{ name, jsonType string }

// routeShape is what a route-backed verb passes on to its route. It holds the
// query parameters, whether the route takes If-Match and a JSON body, and the
// summary that describes the verb.
type routeShape struct {
	queries []viewQuery
	ifMatch bool
	body    bool
	summary string
	// bodySchema is the route's JSON body schema, $refs resolved;
	// answerSchema is its first 2xx answer's.
	bodySchema   string
	answerSchema string
}

// readViewQueries names the query parameters of each route read_view serves,
// keyed as agentPolicies is, so the tool can pass on what the route takes.
func readViewQueries(paths map[string]map[string]yaml.Node, shared map[string]queryParameter,
	schemas map[string]schemaNode, policies []policy,
) (map[string][]viewQuery, error) {
	shapes, err := routeShapes(paths, shared, schemas, nil, policies, func(p policy) bool { return p.Tool == readViewVerb })
	out := map[string][]viewQuery{}
	for route, shape := range shapes {
		out[route] = shape.queries
	}
	return out, err
}

// routeShapes reads the shape of every route a selected policy names.
func routeShapes(paths map[string]map[string]yaml.Node, shared map[string]queryParameter,
	schemas map[string]schemaNode, rawSchemas map[string]any, policies []policy, selected func(policy) bool,
) (map[string]routeShape, error) {
	out := map[string]routeShape{}
	for _, p := range policies {
		if !selected(p) {
			continue
		}
		method, route, _ := strings.Cut(p.Route, " ")
		path := strings.TrimPrefix(route, "/v1")
		var op struct {
			Summary     string           `yaml:"summary"`
			Parameters  []queryParameter `yaml:"parameters"`
			RequestBody yaml.Node        `yaml:"requestBody"`
			Responses   yaml.Node        `yaml:"responses"`
		}
		node := paths[path][strings.ToLower(method)]
		if err := node.Decode(&op); err != nil {
			return nil, fmt.Errorf("%s: %w", p.Route, err)
		}
		// Parameters declared on the path item hold for every operation on it.
		var onPath []queryParameter
		if item, declared := paths[path]["parameters"]; declared {
			if err := item.Decode(&onPath); err != nil {
				return nil, fmt.Errorf("%s: path parameters: %w", p.Route, err)
			}
		}
		op.Parameters = append(onPath, op.Parameters...)
		shape := routeShape{queries: []viewQuery{}, body: op.RequestBody.Kind != 0, summary: strings.TrimSpace(op.Summary)}
		if rawSchemas != nil {
			body, err := bodySchema(&op.RequestBody, rawSchemas)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", p.Route, err)
			}
			shape.bodySchema = body
			answer, err := answerSchema(op.Responses, rawSchemas)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", p.Route, err)
			}
			shape.answerSchema = answer
		}
		for _, param := range op.Parameters {
			if param.Ref != "" {
				resolved, ok := shared[strings.TrimPrefix(param.Ref, "#/components/parameters/")]
				if !ok {
					return nil, fmt.Errorf("%s names parameter %s, which components.parameters does not declare", p.Route, param.Ref)
				}
				param = resolved
			}
			if param.In == "header" && param.Name == "If-Match" {
				shape.ifMatch = true
			}
			if param.In != "query" {
				continue
			}
			jsonType, err := parameterType(param, schemas)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", p.Route, err)
			}
			shape.queries = append(shape.queries, viewQuery{name: param.Name, jsonType: jsonType})
		}
		sort.Slice(shape.queries, func(i, j int) bool { return shape.queries[i].name < shape.queries[j].name })
		out[p.Route] = shape
	}
	return out, nil
}

// parameterType reads a parameter's JSON type, through a component $ref, with
// a nullable type list read as its one non-null member.
func parameterType(param queryParameter, schemas map[string]schemaNode) (string, error) {
	declared := param.Schema.Type
	if param.Schema.Ref != "" {
		component, ok := schemas[strings.TrimPrefix(param.Schema.Ref, "#/components/schemas/")]
		if !ok {
			return "", fmt.Errorf("parameter %s names schema %s, which components.schemas does not declare", param.Name, param.Schema.Ref)
		}
		declared = component.Type
	}
	switch t := declared.(type) {
	case string:
		return t, nil
	case []any:
		for _, member := range t {
			if name, ok := member.(string); ok && name != "null" {
				return name, nil
			}
		}
	case nil:
		return "string", nil
	}
	return "", fmt.Errorf("parameter %s declares type %v, which names no JSON type", param.Name, declared)
}

func renderViews(views map[string][]viewQuery) string {
	var b strings.Builder
	b.WriteString(`
// readViewQueries are the query parameters of each route read_view serves, so
// the tool passes on every one the route takes, typed as the route types it.
var readViewQueries = map[string][]viewQuery{
`)
	routes := make([]string, 0, len(views))
	for route := range views {
		routes = append(routes, route)
	}
	sort.Strings(routes)
	for _, route := range routes {
		fmt.Fprintf(&b, "\t%q: {", route)
		for i, q := range views[route] {
			if i > 0 {
				b.WriteString(", ")
			}
			fmt.Fprintf(&b, "{Name: %q, Type: %q}", q.name, q.jsonType)
		}
		b.WriteString("},\n")
	}
	b.WriteString("}\n")
	return b.String()
}

func renderRouteShapes(shapes map[string]routeShape) string {
	var b strings.Builder
	b.WriteString(`
// routeVerbShapes is what each route-backed verb passes on to its route: the
// query parameters typed as the route types them, If-Match, a JSON body, and
// the summary that describes the verb.
var routeVerbShapes = map[string]routeVerbShape{
`)
	routes := make([]string, 0, len(shapes))
	for route := range shapes {
		routes = append(routes, route)
	}
	sort.Strings(routes)
	for _, route := range routes {
		shape := shapes[route]
		fmt.Fprintf(&b, "\t%q: {Summary: %q, IfMatch: %t, Body: %t, BodySchema: %q, AnswerSchema: %q, Query: []viewQuery{",
			route, shape.summary, shape.ifMatch, shape.body, shape.bodySchema, shape.answerSchema)
		for i, q := range shape.queries {
			if i > 0 {
				b.WriteString(", ")
			}
			fmt.Fprintf(&b, "{Name: %q, Type: %q}", q.name, q.jsonType)
		}
		b.WriteString("}},\n")
	}
	b.WriteString("}\n")
	return b.String()
}
