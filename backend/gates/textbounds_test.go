// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build !integration

package gates

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/margince/margince/backend/internal/platform/database/textbounds"
	"github.com/margince/margince/backend/internal/shared/gatekit"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

func TestEveryTextColumnHasAnExplicitLengthPolicy(t *testing.T) {
	raw, err := gatekit.ReadFile(vocabularyCatalog)
	if err != nil {
		t.Fatal(err)
	}
	columns, err := textbounds.Columns(strings.NewReader(string(raw)))
	if err != nil {
		t.Fatal(err)
	}
	policies, err := textbounds.Policies()
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, column := range columns {
		seen[column.Key] = true
		policy, exists := policies[column.Key]
		if !exists {
			t.Errorf("%s has no text length policy", column.Key)
			continue
		}
		if column.Generated && policy.Exception == "" {
			t.Errorf("%s is generated and cannot have its own write guard", column.Key)
		}
	}
	for key := range policies {
		if !seen[key] {
			t.Errorf("text policy names missing column %s", key)
		}
	}
	for _, line := range strings.Split(string(raw), "\n") {
		if match := catalogTextColumn.FindStringSubmatch(line); match != nil {
			key := "public." + match[1] + "." + match[2]
			if !seen[key] {
				t.Errorf("text census missed %s", key)
			}
		}
	}
}

func TestFrontendTextLimitsComeFromTheSharedValues(t *testing.T) {
	raw, err := gatekit.ReadFile("../frontend/src/lib/textlimits_gen.ts")
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != textbounds.Frontend() {
		t.Fatal("frontend text limits differ from shared values; run make gen")
	}
}

func TestCompanyAndDealDescriptionsShareTheDocumentLimit(t *testing.T) {
	raw, err := gatekit.ReadFile("api/crm.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var contract map[string]any
	if err := yaml.Unmarshal(raw, &contract); err != nil {
		t.Fatal(err)
	}
	for _, schema := range []string{"CreateCompanyRequest", "UpdateCompanyRequest", "CreateDealRequest", "UpdateDealRequest"} {
		cap := publishedMaxLength(t, contract, schemaPath("components", "schemas", schema, "properties", "description"))
		if cap != values.TextDocumentCharacters {
			t.Errorf("%s.description has limit %d, want %d", schema, cap, values.TextDocumentCharacters)
		}
	}
}
