// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package analyticsquery

import (
	"slices"
	"strings"
	"testing"
)

// A source listing is narrowed three ways, and they must not blur: no group
// filter lists every group, the shown groups list theirs, and a grouped answer
// that shows no row lists nothing.
func TestASourceListingReadsOnlyTheGroupsItIsGiven(t *testing.T) {
	t.Parallel()
	grouped := Query{Entity: "deals", GroupBy: []string{"stage", "owner"}, Measures: []Measure{{Fn: CountAll}}}
	shown := [][]any{{"s-1", nil}, {"s-2", "u-1"}}
	none := [][]any{}
	for name, tc := range map[string]struct {
		filter SourceFilter
		want   string
		args   []any
	}{
		"every group": {SourceFilter{}, "WHERE t.archived_at IS NULL ORDER BY t.id LIMIT $1", []any{11}},
		"the shown groups": {
			SourceFilter{Groups: &shown},
			"((t.stage_id IS NOT DISTINCT FROM $1 AND t.owner_id IS NOT DISTINCT FROM $2) OR " +
				"(t.stage_id IS NOT DISTINCT FROM $3 AND t.owner_id IS NOT DISTINCT FROM $4)) ORDER BY t.id LIMIT $5",
			[]any{"s-1", nil, "s-2", "u-1", 11},
		},
		"no shown group": {SourceFilter{Groups: &none}, "AND FALSE ORDER BY t.id LIMIT $1", []any{11}},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			plan, err := CompileSources(grouped, testSchema(), noScope, tc.filter, 11)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.HasSuffix(plan.SQL, tc.want) {
				t.Errorf("got %s\nwant it to end %s", plan.SQL, tc.want)
			}
			if !slices.Equal(plan.Args, tc.args) {
				t.Errorf("args %v, want %v", plan.Args, tc.args)
			}
		})
	}
}
