// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package assurance

import (
	"reflect"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func finding(kind, subjectKind string, subject ids.UUID, amount int64, currency string) Exception {
	e := Exception{ID: ids.NewV7(), Type: kind, SubjectKind: subjectKind, SubjectID: subject, Currency: currency}
	if currency != "" {
		e.AffectedMinor = &amount
	}
	return e
}

func TestGroupsCountDealsApartFromFindingsAndMoneyOncePerDeal(t *testing.T) {
	harbour, quay, ashgrove := ids.NewV7(), ids.NewV7(), ids.NewV7()
	call := ids.NewV7()
	found := []Exception{
		finding("close_past", subjectKindDeal, harbour, 5_000_000, "EUR"),
		finding("no_next_step", subjectKindDeal, harbour, 4_800_000, "EUR"),
		finding("close_past", subjectKindDeal, quay, 3_600_000, "USD"),
		// The same deal twice in one group, at two amounts: the larger counts.
		finding("close_past", subjectKindDeal, harbour, 4_800_000, "EUR"),
		finding("no_next_step", subjectKindDeal, ashgrove, 0, ""),
		finding("call_moved", SubjectKindForecastCall, call, 0, ""),
	}
	answerable := map[ids.UUID]bool{found[2].ID: true}

	groups, totals := GroupFindings(found, answerable)

	want := []FindingGroup{
		{Type: "close_past", Findings: 3, Deals: 2, Exposure: []Exposure{
			{Currency: "EUR", AmountMinor: 5_000_000}, {Currency: "USD", AmountMinor: 3_600_000},
		}, Answerable: true},
		{Type: "no_next_step", Findings: 2, Deals: 2, Exposure: []Exposure{{Currency: "EUR", AmountMinor: 4_800_000}}},
		{Type: "call_moved", Findings: 1, OtherFindings: 1, Exposure: []Exposure{}},
	}
	if !reflect.DeepEqual(groups, want) {
		t.Errorf("groups are\n%+v\nwant\n%+v", groups, want)
	}
	if totals != (FindingTotals{Findings: 6, Deals: 3, OtherFindings: 1}) {
		t.Errorf("totals are %+v, want 6 findings on 3 deals and 1 other", totals)
	}
}

func TestAnEmptyListHasNoGroupsAndZeroTotals(t *testing.T) {
	groups, totals := GroupFindings(nil, nil)
	if len(groups) != 0 || totals != (FindingTotals{}) {
		t.Errorf("an empty list grouped to %+v, %+v", groups, totals)
	}
}
