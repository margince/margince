// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"maps"
	"slices"
	"strconv"
	"strings"
)

// createWithheld are body fields the REST route takes and create_record does
// not. The shape omits them and the tool refuses them by name, both from here.
var createWithheld = []struct {
	recordType string
	fields     []string
}{
	// A contact create that also creates or links a company would land a company
	// as a side effect a human approving the contact never sees.
	{"contact", []string{"company_id", "company_name"}},
}

// withoutWithheld is a create body with the record type's withheld fields
// dropped from its properties and its required list.
func withoutWithheld(schema *node, all schemas, recordType string) (*node, error) {
	resolved, err := resolve(schema, all)
	if err != nil {
		return nil, err
	}
	for _, entry := range createWithheld {
		if entry.recordType != recordType {
			continue
		}
		trimmed := *resolved
		trimmed.Properties = maps.Clone(resolved.Properties)
		for _, field := range entry.fields {
			delete(trimmed.Properties, field)
		}
		trimmed.Required = slices.DeleteFunc(slices.Clone(resolved.Required), func(name string) bool {
			return slices.Contains(entry.fields, name)
		})
		return &trimmed, nil
	}
	return resolved, nil
}

func renderWithheld(b *strings.Builder) {
	b.WriteString("\n// createWithheldFields are body fields create_record refuses although the\n")
	b.WriteString("// REST route takes them.\n")
	b.WriteString("var createWithheldFields = map[string][]string{\n")
	for _, entry := range createWithheld {
		quoted := make([]string, 0, len(entry.fields))
		for _, field := range entry.fields {
			quoted = append(quoted, strconv.Quote(field))
		}
		b.WriteString("\t" + strconv.Quote(entry.recordType) + ": {" + strings.Join(quoted, ", ") + "},\n")
	}
	b.WriteString("}\n")
}
