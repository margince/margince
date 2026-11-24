// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/agents"
)

// A model sent value_basis "amount" and close_window "all_open" and got another
// metric than it asked for, because the routers served both as bare strings.
// Each router a model fills a selection through lists the
// contract's values and a sentence for both.
func TestTheReportRoutersServeTheSelectionsClosedFieldsWithTheirValues(t *testing.T) {
	type field struct {
		Enum        []string `json:"enum"`
		Description string   `json:"description"`
	}
	want := map[string][]string{
		"close_window": {"fiscal_quarter", "all_open", "custom"},
		"value_basis":  {"all", "software", "implementation"},
	}
	for _, tool := range []string{"read_reports", "change_reports"} {
		spec, ok := servedSurface(t).Spec(tool)
		if !ok {
			t.Fatalf("%s is not served", tool)
		}
		var input struct {
			Properties struct {
				Selection struct {
					Properties map[string]field `json:"properties"`
				} `json:"selection"`
			} `json:"properties"`
		}
		if err := json.Unmarshal(spec.InputSchema, &input); err != nil {
			t.Fatalf("%s: %v", tool, err)
		}
		for name, values := range want {
			served := input.Properties.Selection.Properties[name]
			if !slices.Equal(served.Enum, values) || served.Description == "" {
				t.Errorf("%s serves selection.%s with values %v and description %q, want %v and a sentence",
					tool, name, served.Enum, served.Description, values)
			}
		}
	}
}

// The census reads every input derived from a Go type once the surface is built.
// An input registered anywhere is held to values and a sentence.
func TestEveryReflectedToolInputServesItsContractEnumsWithValuesAndASentence(t *testing.T) {
	servedSurface(t)
	gaps, checked := agents.ContractEnumGaps()
	for _, want := range []string{"MeetingInvitationRequest", "ReportScope", "ReportingRead"} {
		if !slices.ContainsFunc(checked, func(name string) bool { return strings.HasSuffix(name, "."+want) }) {
			t.Errorf("the census never met %s: %v", want, checked)
		}
	}
	for _, gap := range gaps {
		t.Error(gap)
	}
}
