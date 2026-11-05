// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package meetingbrief

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/langcopy/langcopytest"
)

func TestOmissionsExplainTheSameLimitsInEveryLanguage(t *testing.T) {
	langcopytest.Census(t, omissionWords)
	for _, language := range []string{"en", "de", "vi"} {
		for _, count := range []int{1, 2} {
			in := Input{Language: language, RoomHidden: true}
			for range count {
				in.History = append(in.History, HistoryIn{Withheld: true})
			}
			got := omissions(in)
			if got == nil || len(*got) != 2 || (*got)[0].Source != "deal_room" || (*got)[1].Source != "activity_history" {
				t.Fatalf("%s: %+v", language, got)
			}
			for _, omission := range *got {
				if omission.Reason == "" || (language != "en" && strings.Contains(omission.Reason, "this brief")) {
					t.Fatalf("%s: %s", language, omission.Reason)
				}
			}
			if count == 2 && !strings.Contains((*got)[1].Reason, "2") {
				t.Fatal("hidden conversation count lost")
			}
		}
	}
	if got := omissions(Input{Language: "de"}); got != nil {
		t.Fatal("full access reported an omission")
	}
}
