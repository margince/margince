// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"testing"
)

func TestASearchOfOnlySpacesFiltersNothing(t *testing.T) {
	for query, wantClauses := range map[string]int{"": 0, "   ": 0, " \t ": 0, "  acme  ": 1} {
		var bound []any
		filters := listFilters{IncludeArchived: true, Query: &query, entity: entityCompany, nameColumn: "display_name"}
		where, err := filters.clauses(context.Background(), nil, nil, func(v any) int {
			bound = append(bound, v)
			return len(bound)
		})
		if err != nil {
			t.Fatalf("%q: %v", query, err)
		}
		if len(where) != wantClauses {
			t.Errorf("%q gave %d clauses, want %d", query, len(where), wantClauses)
		}
		if wantClauses == 1 && (len(bound) != 1 || bound[0] != "acme") {
			t.Errorf("%q bound %v, want the trimmed term", query, bound)
		}
	}
}
