// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contactbrief

import "testing"

func TestTheContactBriefKeepsADealsOwnAmountAndDropsAnInventedSum(t *testing.T) {
	in := inputFixture()
	cases := map[string]int{
		"The renewal is worth 180,000 EUR.": 1,
		"The renewal is worth 950,000 EUR.": 0,
		"The renewal is worth USD 180,000.": 0,
	}
	for text, want := range cases {
		reply := `{"sentences":[{"text":"` + text + `","nature":"fact","evidence":[{"entity_type":"deal","entity_id":"` + briefDealID + `"}]}]}`
		kept, err := parseCitedBrief(reply, briefContactID, in)
		if err != nil || len(kept) != want {
			t.Errorf("%q kept %d sentences (%v), want %d", text, len(kept), err, want)
		}
	}
}
