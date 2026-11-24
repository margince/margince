// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package modelcontract_test

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/modules/agents"
)

func servedSurface() *agents.Registry { return compose.NewRegistry(nil, compose.SendPath{}) }

// selectionFields are the closed fields of a report selection, by name.
type selectionFields struct {
	Properties struct {
		Selection struct {
			Properties map[string]struct {
				Enum        []string `json:"enum"`
				Description string   `json:"description"`
			} `json:"properties"`
		} `json:"selection"`
	} `json:"properties"`
}

// servedSelections are the selections a model reads for tool. That is the
// listed one, or each one the action catalog gives where the listing has none.
func servedSelections(t *testing.T, tool string) []selectionFields {
	t.Helper()
	registry := servedSurface()
	spec, ok := registry.Spec(tool)
	if !ok {
		t.Fatalf("%s is not served", tool)
	}
	var listed selectionFields
	if err := json.Unmarshal(spec.InputSchema, &listed); err != nil {
		t.Fatalf("%s: %v", tool, err)
	}
	if listed.Properties.Selection.Properties != nil {
		return []selectionFields{listed}
	}
	raw, err := registry.ActionCatalog(tool)
	if err != nil {
		t.Fatalf("%s lists no selection and has no action catalog: %v", tool, err)
	}
	var catalog struct {
		Actions []struct {
			Body selectionFields `json:"body"`
		} `json:"actions"`
	}
	if err := json.Unmarshal(raw, &catalog); err != nil {
		t.Fatalf("%s's action catalog: %v", tool, err)
	}
	var out []selectionFields
	for _, action := range catalog.Actions {
		if action.Body.Properties.Selection.Properties != nil {
			out = append(out, action.Body)
		}
	}
	return out
}

// A model sent value_basis "amount" and close_window "all_open" and got another
// metric than it asked for, because the routers served both as bare strings.
// Each router a model fills a selection through lists the contract's values
// and a sentence for both, in its listing or in its action catalog.
func TestTheReportRoutersServeTheSelectionsClosedFieldsWithTheirValues(t *testing.T) {
	want := map[string][]string{
		"close_window": {"fiscal_quarter", "all_open", "custom"},
		"value_basis":  {"all", "software", "implementation"},
	}
	for _, tool := range []string{"read_reports", "change_reports"} {
		selections := servedSelections(t, tool)
		if len(selections) == 0 {
			t.Errorf("%s serves no selection in its listing or its action catalog", tool)
		}
		for _, selection := range selections {
			for name, values := range want {
				served := selection.Properties.Selection.Properties[name]
				if !slices.Equal(served.Enum, values) || served.Description == "" {
					t.Errorf("%s serves selection.%s with values %v and description %q, want %v and a sentence",
						tool, name, served.Enum, served.Description, values)
				}
			}
		}
	}
}

// The census reads every input derived from a Go type once the surface is built.
// An input registered anywhere is held to values and a sentence.
func TestEveryReflectedToolInputServesItsContractEnumsWithValuesAndASentence(t *testing.T) {
	servedSurface()
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
