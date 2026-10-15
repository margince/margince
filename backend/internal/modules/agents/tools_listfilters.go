// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"context"
	"encoding/json"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

// describeListFilters publishes the per-type filter vocabulary list_records
// takes, as a document of its own. Recited inside list_records, it rode in
// every prompt and grew with every type a list serves.
type describeListFilters struct {
	filters map[string][]listFilter
}

// ListFilterEntry is one filter a record list may be narrowed by.
type ListFilterEntry struct {
	Name    string   `json:"name"`
	Operand string   `json:"operand"`
	Values  []string `json:"values,omitempty"`
}

// DescribeListFiltersResult is what describe_list_filters answers.
type DescribeListFiltersResult struct {
	RecordTypes map[string][]ListFilterEntry `json:"record_types"`
}

func (t describeListFilters) Spec() mcp.ToolSpec {
	return mcp.ToolSpec{
		Name: "describe_list_filters", Title: "Describe what each record list may be narrowed by",
		Version:       toolVersionV1,
		Description:   describeListFiltersCopy.render(),
		RequiredScope: principal.ScopeRead, Tier: mcp.TierAutoExecute, Unlisted: true,
		OpenAPIOp:    "describeListFilters",
		InputSchema:  schema(`{"type":"object","properties":{},"additionalProperties":false}`),
		OutputSchema: schemaFor[DescribeListFiltersResult](),
	}
}

func (t describeListFilters) Handle(_ context.Context, in json.RawMessage) (json.RawMessage, error) {
	if err := decodeNoArguments(in); err != nil {
		return nil, err
	}
	return json.Marshal(listFiltersDocument(t.filters))
}

// ListFiltersDocument is the document describe_list_filters answers over
// these stores: the contract's list parameters each one binds.
func ListFiltersDocument(vocabulary FilterVocabulary) DescribeListFiltersResult {
	return listFiltersDocument(bindableFilters(vocabulary))
}

func listFiltersDocument(filters map[string][]listFilter) DescribeListFiltersResult {
	out := DescribeListFiltersResult{RecordTypes: make(map[string][]ListFilterEntry, len(filters))}
	for _, recordType := range listRecordTypes {
		entries := []ListFilterEntry{}
		for _, filter := range filters[recordType] {
			operand := filter.Type
			if operand == "" {
				operand = schemaString
			}
			entries = append(entries, ListFilterEntry{Name: filter.Name, Operand: operand, Values: filter.Enum})
		}
		out.RecordTypes[recordType] = entries
	}
	return out
}
