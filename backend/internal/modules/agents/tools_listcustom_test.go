// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// customFieldVocabulary is probeVocabulary with an activity store that binds
// cf_ keys, the way the activities provider names the prefix.
type customFieldVocabulary struct{ probeVocabulary }

func (v customFieldVocabulary) ListFilters(t datasource.EntityType) []string {
	names := v.probeVocabulary.ListFilters(t)
	if t == datasource.EntityActivity {
		names = append(names, datasource.CustomFieldPrefix)
	}
	return names
}

func TestAnActivityListTakesACustomFieldKeyToTheSeam(t *testing.T) {
	seam := &listProbeProvider{}
	tool := listRecords{p: seam, filters: bindableFilters(customFieldVocabulary{})}
	if _, err := tool.Handle(t.Context(), json.RawMessage(
		`{"record_type":"activity","filters":{"cf_visit_outcome":"Won","kind":"meeting"}}`)); err != nil {
		t.Fatalf("listing activities by a custom field: %v", err)
	}
	if len(seam.queries) != 1 || seam.queries[0].Filters["cf_visit_outcome"] != "Won" {
		t.Fatalf("the custom-field filter did not reach the seam intact: %+v", seam.queries)
	}
}

// A type whose store binds no custom field refuses the key by name, before the
// seam, where it would otherwise run the list unnarrowed.
func TestADealListRefusesACustomFieldKeyByName(t *testing.T) {
	seam := &listProbeProvider{}
	tool := listRecords{p: seam, filters: bindableFilters(customFieldVocabulary{})}
	_, err := tool.Handle(t.Context(), json.RawMessage(`{"record_type":"deal","filters":{"cf_region":"North"}}`))
	if err == nil || !strings.Contains(err.Error(), "cf_region") {
		t.Fatalf("a deal was listed by a custom field, or the refusal does not name it: %v", err)
	}
	if seam.queries != nil {
		t.Error("the refused custom-field key reached the seam")
	}
}

func TestTheFiltersObjectDeclaresTheCustomFieldKeyPattern(t *testing.T) {
	tool := listRecords{filters: bindableFilters(customFieldVocabulary{})}
	var declared struct {
		Properties struct {
			Filters struct {
				Properties        map[string]json.RawMessage `json:"properties"`
				PatternProperties map[string]json.RawMessage `json:"patternProperties"` //nolint:tagliatelle // JSON Schema's own key spelling
			} `json:"filters"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(tool.Spec().InputSchema, &declared); err != nil {
		t.Fatalf("list_records' input schema does not parse: %v", err)
	}
	if _, ok := declared.Properties.Filters.PatternProperties[customFieldKeyPattern]; !ok {
		t.Errorf("filters declares no %s pattern: %+v", customFieldKeyPattern, declared.Properties.Filters.PatternProperties)
	}
	if _, ok := declared.Properties.Filters.Properties[datasource.CustomFieldPrefix]; ok {
		t.Error("filters declares the bare cf_ prefix as a key, which names no field")
	}
}

func TestTheListFilterDocumentNamesTheCustomFieldsOfAnActivityOnly(t *testing.T) {
	answer, err := describeListFilters{filters: bindableFilters(customFieldVocabulary{})}.Handle(context.Background(), nil)
	if err != nil {
		t.Fatalf("describe_list_filters refused: %v", err)
	}
	var published DescribeListFiltersResult
	if err := json.Unmarshal(answer, &published); err != nil {
		t.Fatalf("the answer is not the declared result: %v", err)
	}
	takesCustom := func(recordType string) bool {
		for _, filter := range published.RecordTypes[recordType] {
			if filter.Pattern == customFieldKeyPattern {
				return true
			}
		}
		return false
	}
	if !takesCustom("activity") || takesCustom("deal") {
		t.Errorf("custom fields listed on activity: %v, on deal: %v; want only activity", takesCustom("activity"), takesCustom("deal"))
	}
}
