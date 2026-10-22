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

// readViewQueries names the query parameters of each route read_view serves,
// keyed as agentPolicies is, so the tool can pass on what the route takes.
func readViewQueries(paths map[string]map[string]yaml.Node, shared map[string]queryParameter,
	schemas map[string]schemaNode, policies []policy,
) (map[string][]viewQuery, error) {
	out := map[string][]viewQuery{}
	for _, p := range policies {
		if p.Tool != readViewVerb {
			continue
		}
		path := strings.TrimPrefix(strings.SplitN(p.Route, " ", 2)[1], "/v1")
		var op struct {
			Parameters []queryParameter `yaml:"parameters"`
		}
		node := paths[path]["get"]
		if err := node.Decode(&op); err != nil {
			return nil, fmt.Errorf("%s: %w", p.Route, err)
		}
		queries := []viewQuery{}
		for _, param := range op.Parameters {
			if param.Ref != "" {
				resolved, ok := shared[strings.TrimPrefix(param.Ref, "#/components/parameters/")]
				if !ok {
					return nil, fmt.Errorf("%s names parameter %s, which components.parameters does not declare", p.Route, param.Ref)
				}
				param = resolved
			}
			if param.In != "query" {
				continue
			}
			jsonType, err := parameterType(param, schemas)
			if err != nil {
				return nil, fmt.Errorf("%s: %w", p.Route, err)
			}
			queries = append(queries, viewQuery{name: param.Name, jsonType: jsonType})
		}
		sort.Slice(queries, func(i, j int) bool { return queries[i].name < queries[j].name })
		out[p.Route] = queries
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
