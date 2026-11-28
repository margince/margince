// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package extdelivery

import (
	"context"
	"slices"
	"testing"
)

func heldNone(context.Context, map[string]any) ([]string, error) { return nil, nil }

func TestOnlyAValueThatDiffersIsWrittenAndAUsersValueIsNamed(t *testing.T) {
	current := map[string]any{"cf_a": "same", "cf_b": "old", "cf_c": "typed"}
	offered := map[string]any{"cf_a": "same", "cf_b": "new", "cf_c": "offered", "cf_d": []string{"x"}}
	userOwns := func(_ context.Context, patch map[string]any) ([]string, error) {
		if _, unchanged := patch["cf_a"]; unchanged {
			t.Error("an unchanged value was put to the ownership check")
		}
		return []string{"cf_c"}, nil
	}
	write, withheld, err := ValuesToWrite(context.Background(), current, offered, userOwns)
	if err != nil {
		t.Fatal(err)
	}
	if len(write) != 2 || write["cf_b"] != "new" || write["cf_d"] == nil {
		t.Errorf("write = %v, want cf_b and cf_d only", write)
	}
	if !slices.Equal(withheld, []string{"cf_c"}) {
		t.Errorf("withheld = %v, want cf_c", withheld)
	}
}

func TestARepeatOfTheStoredValuesWritesNothing(t *testing.T) {
	stored := map[string]any{"cf_tags": []any{"a", "b"}}
	write, withheld, err := ValuesToWrite(context.Background(), stored, map[string]any{"cf_tags": []string{"a", "b"}}, heldNone)
	if err != nil || len(write) != 0 || len(withheld) != 0 {
		t.Errorf("write = %v, withheld = %v, err = %v, want nothing for a repeat", write, withheld, err)
	}
}
