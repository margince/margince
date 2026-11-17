// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

func TestAStatedSourceIsCheckedBeforeTheContactIsWritten(t *testing.T) {
	text := func(s string) *string { return &s }
	source := func(s crmcontracts.ContactStatedSource) *crmcontracts.ContactStatedSource { return &s }
	cases := []struct {
		name    string
		source  *crmcontracts.ContactStatedSource
		note    *string
		want    Acquisition
		refused bool
	}{
		{name: "nothing stated", want: Acquisition{}},
		{name: "a card", source: source(crmcontracts.ContactStatedSourceBusinessCard), want: Acquisition{StatedSource: "business_card"}},
		{name: "other with words", source: source(crmcontracts.ContactStatedSourceOther), note: text(" A podcast "), want: Acquisition{StatedSource: "other", Note: "A podcast"}},
		{name: "other alone", source: source(crmcontracts.ContactStatedSourceOther), refused: true},
		{name: "a word outside the list", source: source("cold_list"), refused: true},
	}
	for _, tc := range cases {
		got, err := statedAcquisition(tc.source, tc.note)
		if tc.refused != (err != nil) || (!tc.refused && got != tc.want) {
			t.Errorf("%s: got %+v, %v", tc.name, got, err)
		}
	}
}
