// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package values

import "testing"

func TestCatalogKeyCollapsesEveryNonKeyRunToOneUnderscore(t *testing.T) {
	cases := map[string]string{
		"Trade show":        "trade_show",
		"  Win review (Q4)": "win_review_q4",
		"Đánh giá":          "nh_gi",
		"文字":                "",
	}
	for label, want := range cases {
		if got := CatalogKey(label); got != want {
			t.Errorf("CatalogKey(%q) = %q, want %q", label, got, want)
		}
	}
}
