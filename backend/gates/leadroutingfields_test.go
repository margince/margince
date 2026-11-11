// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H2

package gates

import (
	"slices"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/margince/margince/backend/internal/modules/automation"
	"github.com/margince/margince/backend/internal/shared/gatekit"
)

func TestPublicLeadRoutingFieldsMatchTheExistingRouter(t *testing.T) {
	raw, err := gatekit.ReadFile("api/crm.yaml")
	if err != nil {
		t.Fatal(err)
	}
	var api struct {
		Components struct {
			Schemas map[string]struct {
				Properties map[string]struct {
					Enum []string `yaml:"enum"`
				} `yaml:"properties"`
			} `yaml:"schemas"`
		} `yaml:"components"`
	}
	if err := yaml.Unmarshal(raw, &api); err != nil {
		t.Fatal(err)
	}
	fields := api.Components.Schemas["LeadRoutingRule"].Properties["field"].Enum
	if len(fields) == 0 {
		t.Fatal("public routing field enum is missing")
	}
	if len(fields) != len(automation.RoutableLeadFields) {
		t.Fatalf("public routing fields = %v, router fields = %v", fields, automation.RoutableLeadFields)
	}
	for _, field := range fields {
		if !slices.Contains(automation.RoutableLeadFields, field) {
			t.Fatalf("public routing field %s is unsupported", field)
		}
	}
}
