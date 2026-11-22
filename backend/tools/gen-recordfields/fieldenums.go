// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// renderFieldEnums emits the values and the contract's sentence for every enum-valued
// property of a component schema. An array of enum items counts. A tool schema
// derived from a Go type sees neither, because Go keeps enum values in constants.
// The key is the schema's name and the property's wire name, as the Go type and json tag spell them.
func renderFieldEnums(b *strings.Builder, all schemas) error {
	b.WriteString("\n// contractFieldEnums is every enum-valued property of a crm.yaml schema, keyed\n")
	b.WriteString("// by schema name and wire name, with the contract's first paragraph for it.\n")
	b.WriteString("var contractFieldEnums = map[string]contractFieldEnum{\n")
	names := make([]string, 0, len(all))
	for name := range all {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		schema := all[name]
		if schema == nil {
			continue
		}
		fields := make([]string, 0, len(schema.Properties))
		for field := range schema.Properties {
			fields = append(fields, field)
		}
		sort.Strings(fields)
		for _, field := range fields {
			values, note, err := fieldEnum(schema.Properties[field], all)
			if err != nil {
				return fmt.Errorf("%s.%s: %w", name, field, err)
			}
			if len(values) == 0 {
				continue
			}
			fmt.Fprintf(b, "\t%s: {Values: []string{%s}, Description: %s},\n",
				strconv.Quote(name+"."+field), strings.Join(values, ", "), strconv.Quote(note))
		}
	}
	b.WriteString("}\n")
	return nil
}

// fieldEnum reads one property: its enum values and the sentence that says what
// they change. The property's own description wins over the enum schema's.
func fieldEnum(prop *node, all schemas) (values []string, note string, err error) {
	own := firstParagraph(prop.Description)
	for wrapped := prop; own == "" && wrapped != nil && len(wrapped.AllOf) == 1; wrapped = wrapped.AllOf[0] {
		own = firstParagraph(wrapped.AllOf[0].Description)
	}
	resolved, err := resolve(prop, all)
	if err != nil {
		return nil, "", err
	}
	if typeNames(resolved)["array"] && resolved.Items != nil {
		if own == "" {
			own = firstParagraph(resolved.Description)
		}
		if resolved, err = resolve(resolved.Items, all); err != nil {
			return nil, "", err
		}
	}
	if !typeNames(resolved)["string"] {
		return nil, "", nil
	}
	if own == "" {
		own = firstParagraph(resolved.Description)
	}
	return enumValues(resolved), own, nil
}

// maxNoteBytes bounds a field note: a served schema pays for every word on
// every step, so a note keeps the whole sentences that fit.
const maxNoteBytes = 240

// firstParagraph is a description's first paragraph on one line, cut after the
// last whole sentence within maxNoteBytes. The rest is for a reader of the
// contract. A first sentence longer than the bound stays whole.
func firstParagraph(text string) string {
	paragraph, _, _ := strings.Cut(strings.TrimSpace(text), "\n\n")
	flat := strings.Join(strings.Fields(paragraph), " ")
	if len(flat) <= maxNoteBytes {
		return flat
	}
	kept := ""
	for _, sentence := range strings.SplitAfter(flat, ". ") {
		if kept != "" && len(kept)+len(sentence) > maxNoteBytes {
			break
		}
		kept += sentence
	}
	return strings.TrimSpace(kept)
}
