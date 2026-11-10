// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"slices"
	"testing"
)

// boundArgs collects what a predicate binds, the way listActivitiesFilter does.
func boundArgs() (*[]any, func(any) int) {
	args := &[]any{}
	return args, func(v any) int { *args = append(*args, v); return len(*args) }
}

func TestKnownKindsAreSpelledAsLiteralsSoThePartialIndexesStayUsable(t *testing.T) {
	for _, tc := range []struct {
		kinds []string
		want  string
	}{
		{[]string{"email"}, "a.kind = 'email'"},
		{[]string{"email", "message"}, "a.kind IN ('email', 'message')"},
	} {
		args, arg := boundArgs()

		got := kindPredicate(tc.kinds, arg)

		if got != tc.want {
			t.Errorf("kindPredicate(%v) = %q, want %q", tc.kinds, got, tc.want)
		}
		if len(*args) != 0 {
			t.Errorf("kindPredicate(%v) bound %v, want nothing bound", tc.kinds, *args)
		}
	}
}

// One value outside the vocabulary binds the whole set: a literal is never
// spelled from a value the vocabulary does not hold.
func TestASetWithAnUnknownKindIsBoundWhole(t *testing.T) {
	args, arg := boundArgs()
	arg("an earlier argument")
	kinds := []string{"email", "fax'); DROP TABLE activity; --"}

	got := kindPredicate(kinds, arg)

	if want := "a.kind = ANY($2)"; got != want {
		t.Errorf("kindPredicate(%v) = %q, want %q", kinds, got, want)
	}
	if len(*args) != 2 {
		t.Fatalf("bound %d arguments, want the earlier one and the kind set", len(*args))
	}
	bound, ok := (*args)[1].([]string)
	if !ok || !slices.Equal(bound, kinds) {
		t.Errorf("bound %v, want the whole kind set %v", (*args)[1], kinds)
	}
}
