// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package draftfloor

import "testing"

func TestAGreetingUsesTheGivenNameInEitherNameOrder(t *testing.T) {
	for display, want := range map[string]string{
		"Phan Phương Chi": "Chi",
		"Phan Phuong Chi": "Chi",
		"Nguyễn Văn An":   "An",
		"ĐẶNG Thu Hà":     "Hà",
		"Sofia Meier":     "Sofia",
		"Nguyen":          "Nguyen",
		"Anna Le":         "Anna",
		"":                "",
	} {
		if got := GreetingName("", display); got != want {
			t.Errorf("GreetingName(%q) = %q, want %q", display, got, want)
		}
	}
	if got := GreetingName("Bé Chi", "Phan Phương Chi"); got != "Bé Chi" {
		t.Errorf("a chosen greeting name lost to the display name: %q", got)
	}
}
