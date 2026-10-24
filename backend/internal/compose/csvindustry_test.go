// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import "testing"

// The store holds the code, so a file spelling the industry another way is not
// a change, and re-importing an unchanged file reports nothing to update.
func TestAnImportedIndustryIsComparedAsTheCodeItNames(t *testing.T) {
	stored := []byte(`{"industry":"healthcare"}`)
	for _, cell := range []string{"Healthcare", "health care", "Gesundheitswesen"} {
		changed, err := changedFields(stored, map[string]string{fieldIndustry: cell}, nil)
		if err != nil {
			t.Fatal(err)
		}
		if len(changed) != 0 {
			t.Errorf("%q reported as a change to healthcare: %v", cell, changed)
		}
	}
	changed, err := changedFields(stored, map[string]string{fieldIndustry: "Software"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, moved := changed[fieldIndustry]; !moved {
		t.Error("a different industry was not reported as a change")
	}
}
