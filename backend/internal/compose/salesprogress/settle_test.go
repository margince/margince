// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package salesprogress

import (
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/modules/reporting"
)

func TestSettleSharesAddsUpToTheTotalExactly(t *testing.T) {
	cases := []struct {
		name    string
		total   int64
		weights []int64
		want    []int64
	}{
		{"even split", 100, []int64{1, 1}, []int64{50, 50}},
		{"a tie goes to the earlier weight", 100, []int64{1, 1, 1}, []int64{34, 33, 33}},
		{"largest remainder first", 10, []int64{1, 2, 4}, []int64{1, 3, 6}},
		{"scaled down to a smaller deal value", 6000000, []int64{4000000, 3000000}, []int64{3428571, 2571429}},
		{"nothing to split over", 500, []int64{0, 0}, []int64{0, 0}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := settleShares(tc.total, tc.weights)
			if !slices.Equal(got, tc.want) {
				t.Fatalf("settleShares(%d, %v) = %v, want %v", tc.total, tc.weights, got, tc.want)
			}
		})
	}
}

func mixPart(group, row string, money int64) reporting.Fact {
	amount := money
	fact := reporting.Fact{GroupKey: group, Money: &amount}
	fact.Row.Key = row
	return fact
}

func partMoney(parts []reporting.Fact) map[string]int64 {
	out := map[string]int64{}
	for _, part := range parts {
		out[part.Row.Key] = *part.Money
	}
	return out
}

func TestSettleDealSplitsTheDealValueOverCategoriesThenLines(t *testing.T) {
	facts := []reporting.Fact{
		mixPart("kind:implementation", "d:svc", 30000),
		mixPart("kind:software", "d:b", 20000),
		mixPart("kind:software", "d:a", 50000),
	}
	total := int64(80000)
	got := partMoney(settleDeal(facts, []int{0, 1, 2}, &total, "invalid"))
	want := map[string]int64{"d:a": 40000, "d:b": 16000, "d:svc": 24000}
	if len(got) != len(want) {
		t.Fatalf("settled parts = %v, want %v", got, want)
	}
	for key, amount := range want {
		if got[key] != amount {
			t.Fatalf("part %s = %d, want %d (all %v)", key, got[key], amount, got)
		}
	}
}

func TestSettleDealShowsAnOverAllocatedDealWhole(t *testing.T) {
	facts := []reporting.Fact{
		mixPart("kind:software", "d:a", 90000),
		mixPart("kind:unallocated", "d:unallocated", -10000),
	}
	total := int64(80000)
	got := settleDeal(facts, []int{0, 1}, &total, "Lines exceed the deal value")
	if len(got) != 1 {
		t.Fatalf("an over-allocated deal settles to %d parts, want one", len(got))
	}
	if got[0].GroupKey != "kind:"+InvalidKind || *got[0].Money != total {
		t.Fatalf("invalid part = %s %d, want kind:%s %d", got[0].GroupKey, *got[0].Money, InvalidKind, total)
	}
	if Category(got[0].GroupKey) != Unresolved {
		t.Fatalf("an invalid deal counts as %s, want unresolved", Category(got[0].GroupKey))
	}
}

func TestSettleDealKeepsTheLinesOfADealWithNoValue(t *testing.T) {
	facts := []reporting.Fact{mixPart("kind:software", "d:a", 5000), mixPart("kind:unallocated", "d:unallocated", -1)}
	got := settleDeal(facts, []int{0, 1}, nil, "invalid")
	if len(got) != 1 || *got[0].Money != 5000 {
		t.Fatalf("a deal with no value keeps %v, want only its positive line", partMoney(got))
	}
}
