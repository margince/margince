// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"fmt"
	"strings"
)

// contractEnums are the closed vocabularies a hand-written tool schema would
// otherwise spell again. Each is emitted as a JSON array to splice in, because
// both hand-written copies had already drifted from the contract.
var contractEnums = []struct{ name, schema, path string }{
	{"activityKindEnum", "CreateActivityRequest", "kind"},
	{"activityLinkEntityTypeEnum", "CreateActivityRequest", "links.items.entity_type"},
	{"activityMeetingStatusEnum", "CreateActivityRequest", "meeting_status"},
}

// contractVocabularies are closed vocabularies Go code reads as values: the
// sidecar keys record_fields lists and change_record_parts admits.
var contractVocabularies = []struct{ name, schema, path string }{
	{"companyProfileFieldKeys", "CompanyProfileField", "field"},
	{"companyFactCategories", "CompanyFact", "category"},
	{"companyFactFields", "CompanyFact", "field"},
	{"contactProfileFieldKeys", "ContactProfileField", "field"},
}

func renderEnums(b *strings.Builder, all schemas) error {
	b.WriteString("\n// The contract's own closed vocabularies, as JSON arrays a tool's InputSchema\n")
	b.WriteString("// splices in place of a hand-written enum literal.\n")
	for _, want := range contractEnums {
		values, err := contractEnumValues(all, want.schema, want.path)
		if err != nil {
			return err
		}
		b.WriteString("\nconst " + want.name + " = `[" + strings.Join(values, ",") + "]`\n")
	}
	b.WriteString("\n// The sidecar vocabularies, as values.\n")
	for _, want := range contractVocabularies {
		values, err := contractEnumValues(all, want.schema, want.path)
		if err != nil {
			return err
		}
		b.WriteString("\nvar " + want.name + " = []string{" + strings.Join(values, ", ") + "}\n")
	}
	return nil
}

// contractEnumValues reads one property's enum, quoted as Go and JSON spell it.
func contractEnumValues(all schemas, schemaName, path string) ([]string, error) {
	schema, ok := all[schemaName]
	if !ok {
		return nil, fmt.Errorf("crm.yaml declares no %s", schemaName)
	}
	target, err := walk(schema, path, all)
	if err != nil {
		return nil, fmt.Errorf("%s.%s: %w", schemaName, path, err)
	}
	values := enumValues(target)
	if len(values) == 0 {
		return nil, fmt.Errorf("%s.%s declares no enum", schemaName, path)
	}
	return values, nil
}
