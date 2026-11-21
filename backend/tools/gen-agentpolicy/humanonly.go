// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// humanOnlyMarker flags a body member a human alone may set. The store refuses
// any other principal; the agent is not offered the member at all.
const humanOnlyMarker = "x-human-only"

// withoutHumanOnly drops each top-level member that carries humanOnlyMarker and
// names them, sorted. The agent gate refuses an agent body that sets one, and
// it reads top-level members only, so a marker deeper down is refused here. So
// is a required one, since no agent call could then be complete.
//
//craft:ignore naked-any a JSON schema is walked as decoded YAML, whatever its shape
func withoutHumanOnly(root any) ([]string, error) {
	var names []string
	if top, ok := root.(map[string]any); ok {
		props, _ := top["properties"].(map[string]any)
		required, _ := top["required"].([]any)
		for name, member := range props {
			if fields, ok := member.(map[string]any); ok && fields[humanOnlyMarker] == true {
				if slices.Contains(required, any(name)) {
					return nil, fmt.Errorf("body member %s is human-only and required, so no agent call could be complete", name)
				}
				delete(props, name)
				names = append(names, name)
			}
		}
	}
	if carriesBelow(root, humanOnlyMarker) {
		return nil, errors.New("x-human-only is read on a top-level body member only")
	}
	sort.Strings(names)
	return names, nil
}

// humanOnlyOffTheDoor names each operation that marks a body member
// x-human-only without being served_by: route. The agent gate reads the marker
// from the route-verb table, so on any other operation it would hold for nobody.
func humanOnlyOffTheDoor(paths map[string]map[string]yaml.Node, rawSchemas map[string]any, policies []policy) []string {
	var defects []string
	for _, p := range policies {
		if p.ServedBy == routeServed {
			continue
		}
		method, route, _ := strings.Cut(p.Route, " ")
		var op struct {
			RequestBody yaml.Node `yaml:"requestBody"`
		}
		node := paths[strings.TrimPrefix(route, "/v1")][strings.ToLower(method)]
		if err := node.Decode(&op); err != nil {
			defects = append(defects, fmt.Sprintf("%s: %v", p.Route, err))
			continue
		}
		if _, marks, err := bodySchema(&op.RequestBody, rawSchemas); err != nil || len(marks.humanOnly) > 0 || len(marks.agentValues) > 0 {
			defects = append(defects, fmt.Sprintf("%s marks %v x-human-only and %v x-agent-values (%v); only a served_by: route operation enforces them",
				p.Route, marks.humanOnly, marks.agentValues, err))
		}
	}
	return defects
}
