// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package weeklyplan

import (
	"encoding/json"
	"testing"
)

func TestACommitmentDueDateOutsideTheStorableRangeIsRefused(t *testing.T) {
	for _, day := range []string{"0000-01-01", "0001-01-01", "9999-12-31", "2026-02-30"} {
		if _, _, err := parseEditDate(json.RawMessage(`"` + day + `"`)); err == nil {
			t.Errorf("parseEditDate accepted %q", day)
		}
	}
	got, ok, err := parseEditDate(json.RawMessage(`"2026-10-10"`))
	if err != nil || !ok || got == nil {
		t.Fatalf("parseEditDate refused a real day: %v", err)
	}
	if _, ok, err := parseEditDate(json.RawMessage(`null`)); err != nil || !ok {
		t.Fatalf("a null must clear the date: ok=%v err=%v", ok, err)
	}
}
