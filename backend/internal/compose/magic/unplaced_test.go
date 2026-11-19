// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package magic

import (
	"maps"
	"regexp"
	"slices"
	"strconv"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/auditentity"
)

func TestLinkChangeSQLReferencesExactlyTheArgumentsItAdds(t *testing.T) {
	args := []any{"since", "actors"}
	arg := func(v any) int { args = append(args, v); return len(args) }
	action, linked := linkChangeSQL(arg)

	referenced := map[int]bool{}
	for _, match := range regexp.MustCompile(`\$(\d+)`).FindAllStringSubmatch(action+linked, -1) {
		n, err := strconv.Atoi(match[1])
		if err != nil {
			t.Fatal(err)
		}
		referenced[n] = true
	}
	want := make([]int, 0, len(args)-2)
	for n := 3; n <= len(args); n++ {
		want = append(want, n)
	}
	if got := slices.Sorted(maps.Keys(referenced)); !slices.Equal(got, want) {
		t.Fatalf("placeholders %v, want %v for the arguments it added", got, want)
	}
	if len(args)-2 != 2*len(auditentity.LinkChanges()) {
		t.Fatalf("added %d arguments for %d link changes", len(args)-2, len(auditentity.LinkChanges()))
	}
}
