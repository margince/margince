// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"maps"
	"slices"
	"strconv"
	"strings"
)

// withheldFields are the create body's properties marked `x-agent-withheld`:
// fields the REST route takes and create_record refuses, sorted.
func withheldFields(schema *node, all schemas) ([]string, error) {
	resolved, err := resolve(schema, all)
	if err != nil {
		return nil, err
	}
	var out []string
	for name, prop := range resolved.Properties {
		if prop != nil && prop.AgentWithheld {
			out = append(out, name)
		}
	}
	slices.Sort(out)
	return out, nil
}

// withoutWithheld is a create body with its withheld fields dropped from its
// properties and its required list.
func withoutWithheld(schema *node, all schemas) (*node, error) {
	resolved, err := resolve(schema, all)
	if err != nil {
		return nil, err
	}
	withheld, err := withheldFields(resolved, all)
	if err != nil || len(withheld) == 0 {
		return resolved, err
	}
	trimmed := *resolved
	trimmed.Properties = maps.Clone(resolved.Properties)
	for _, field := range withheld {
		delete(trimmed.Properties, field)
	}
	trimmed.Required = slices.DeleteFunc(slices.Clone(resolved.Required), func(name string) bool {
		return slices.Contains(withheld, name)
	})
	return &trimmed, nil
}

func renderWithheld(b *strings.Builder, all schemas) error {
	b.WriteString("\n// createWithheldFields are body fields create_record refuses although the\n")
	b.WriteString("// REST route takes them: the contract marks each `x-agent-withheld`.\n")
	b.WriteString("var createWithheldFields = map[string][]string{\n")
	for _, entry := range recordBodies {
		withheld, err := withheldFields(all[entry.create], all)
		if err != nil {
			return err
		}
		if len(withheld) == 0 {
			continue
		}
		quoted := make([]string, 0, len(withheld))
		for _, field := range withheld {
			quoted = append(quoted, strconv.Quote(field))
		}
		b.WriteString("\t" + strconv.Quote(entry.recordType) + ": {" + strings.Join(quoted, ", ") + "},\n")
	}
	b.WriteString("}\n")
	return nil
}
