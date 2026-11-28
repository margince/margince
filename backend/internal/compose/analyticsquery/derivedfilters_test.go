// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package analyticsquery

import (
	"maps"
	"strings"
	"testing"
)

func derivedFilterSchema(names ...string) Schema {
	schema := testSchema()
	entity := schema.Entities["deals"]
	fields := maps.Clone(entity.Fields)
	for _, name := range names {
		fields[name] = Field{Name: name, Expr: "t." + name, Kind: KindFilter, Shape: ShapeNumber}
	}
	entity.Fields = fields
	schema.Entities["deals"] = entity
	return schema
}

// A model reads filter names from the vocabulary document, so a derived filter
// the seat holds is named there with what it means.
func TestTheVocabularyNamesTheDerivedFiltersASeatHolds(t *testing.T) {
	t.Parallel()
	doc := DescribeSchema(derivedFilterSchema(FilterDaysSinceActivity, FilterMeetingAhead))
	for _, want := range []string{FilterDaysSinceActivity, FilterMeetingAhead, "longer than any number"} {
		if !strings.Contains(doc, want) {
			t.Errorf("the vocabulary omits %q:\n%s", want, doc)
		}
	}
	if strings.Contains(doc, FilterCompanyDaysSinceActivity) {
		t.Errorf("the vocabulary names %s, which this seat does not hold:\n%s", FilterCompanyDaysSinceActivity, doc)
	}
}
