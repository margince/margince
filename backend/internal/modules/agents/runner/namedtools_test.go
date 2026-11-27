// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package runner

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

func TestASentenceNamingAToolThatIsNotOfferedGoesAndTheRestStays(t *testing.T) {
	t.Parallel()
	spec := mcp.ToolSpec{
		Name:        "log_activity",
		Description: "Records what happened, e.g. a call. Unlinked, it is on no timeline; relink_activity links it later. Keep the id.",
		NamedTools:  []string{"relink_activity"},
	}
	got := AsOffered([]mcp.ToolSpec{spec})[0].Description
	want := "Records what happened, e.g. a call. Keep the id."
	if got != want {
		t.Errorf("description = %q, want %q", got, want)
	}
}

func TestADescriptionNamingOnlyOfferedToolsIsUntouched(t *testing.T) {
	t.Parallel()
	spec := mcp.ToolSpec{
		Name:        "a",
		Description: "First.\n\nSecond names b.",
		NamedTools:  []string{"b"},
	}
	offered := []mcp.ToolSpec{spec, {Name: "b"}}
	if got := AsOffered(offered)[0].Description; got != spec.Description {
		t.Errorf("description = %q, want it unchanged", got)
	}
}

func TestASchemaDescriptionNamingAToolThatIsNotOfferedLosesThatSentence(t *testing.T) {
	t.Parallel()
	schema := `{"type":"object","properties":{"filters":{"description":"String operands. describe_list_filters lists the values.","type":"string"},"only":{"description":"Names describe_list_filters.","type":"string"}}}`
	spec := mcp.ToolSpec{Name: "list_records", InputSchema: json.RawMessage(schema), NamedTools: []string{"describe_list_filters"}}
	got := string(AsOffered([]mcp.ToolSpec{spec})[0].InputSchema)
	if strings.Contains(got, "describe_list_filters") {
		t.Errorf("schema still names the tool: %s", got)
	}
	if !strings.Contains(got, `"String operands."`) {
		t.Errorf("schema lost the sentence that names nothing: %s", got)
	}
	if strings.Contains(got, `"only"`+`:{"description"`) {
		t.Errorf("a description left empty should be removed: %s", got)
	}
}

func TestASchemaWithNothingToDropKeepsItsBytes(t *testing.T) {
	t.Parallel()
	schema := `{"properties":{"b":{"description":"Plain.","type":"string"}},"type":"object"}`
	spec := mcp.ToolSpec{Name: "a", InputSchema: json.RawMessage(schema), NamedTools: []string{"gone"}}
	if got := string(AsOffered([]mcp.ToolSpec{spec})[0].InputSchema); got != schema {
		t.Errorf("schema = %s, want it unchanged", got)
	}
}
