// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package moneynotation

import (
	"math"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

func TestGroupedWritesTheReadersDigitMarks(t *testing.T) {
	cases := []struct {
		name     string
		minor    int64
		currency string
		lang     textlang.Lang
		want     string
	}{
		{"english groups with commas", 1234500, "EUR", textlang.English, "12,345.00"},
		{"german groups with dots", 1234500, "EUR", textlang.German, "12.345,00"},
		{"vietnamese groups with dots", 1234500, "EUR", textlang.Vietnamese, "12.345,00"},
		{"an unset language reads as english", 1234500, "EUR", textlang.Unknown, "12,345.00"},
		{"a zero-decimal currency has no fraction", 18000000, "JPY", textlang.German, "18.000.000"},
		{"three digits take no mark", 99900, "USD", textlang.English, "999.00"},
		{"a negative amount keeps its sign in front", -123456789, "EUR", textlang.English, "-1,234,567.89"},
		{"the smallest int64 still groups", math.MinInt64, "JPY", textlang.English, "-9,223,372,036,854,775,808"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Grouped(tc.minor, tc.currency, tc.lang); got != tc.want {
				t.Fatalf("Grouped(%d, %s, %q) = %q, want %q", tc.minor, tc.currency, tc.lang, got, tc.want)
			}
		})
	}
}
