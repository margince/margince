// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package roombrief

import "testing"

func TestTheRoomBriefKeepsASumABuyerWroteAndDropsAnInventedOne(t *testing.T) {
	in := room()
	in.Threads[1].Comments[0].Body = "Our budget is 180,000 EUR."
	cases := map[string]int{"The budget is 180,000 EUR.": 1, "The budget is 950,000 EUR.": 0}
	for text, want := range cases {
		reply := cited(in, `{"sentences":[{"text":"`+text+`","nature":"fact","evidence":[{"entity_type":"thread","entity_id":"`+capID+`"}]}]}`)
		kept, err := ParseBrief(reply, in)
		if err != nil || len(kept) != want {
			t.Errorf("%q kept %d sentences (%v), want %d", text, len(kept), err, want)
		}
	}
}
