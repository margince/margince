// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

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
		if _, marks, err := bodySchema(&op.RequestBody, rawSchemas); err != nil || len(marks.HumanOnly) > 0 || len(marks.AgentValues) > 0 {
			defects = append(defects, fmt.Sprintf("%s marks %v x-human-only and %v x-agent-values (%v); only a served_by: route operation enforces them",
				p.Route, marks.HumanOnly, marks.AgentValues, err))
		}
	}
	return defects
}
