// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package meetingbrief

import (
	"reflect"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/langcopy/langcopytest"
)

func TestChangedCopyKeepsBaselineSourcesAndQuotes(t *testing.T) {
	langcopytest.Census(t, changedWords)
	for _, language := range []string{"en", "de", "vi"} {
		in := fullInput()
		in.LastSpokeAt = new(instantOn(5))
		in.Commitments = []ClaimIn{{ContactName: "Original Name", Kind: kindObjection, Body: "Original objection", SourceID: activityID, OccurredAt: new(instantOn(7))}}
		in.Recent = []ActIn{{ID: activityID, Subject: "Original subject", At: at(8)}}
		english := whatChangedSection(in, rankClaims(in))
		in.Language = language
		got := whatChangedSection(in, rankClaims(in))
		if len(got) != len(english) {
			t.Fatal("language changed coverage")
		}
		for i, line := range got {
			if !reflect.DeepEqual(line.Evidence, english[i].Evidence) || line.Nature != english[i].Nature {
				t.Fatal("language changed evidence")
			}
			if language != "en" && line.Text == english[i].Text {
				t.Fatalf("%s left prose unchanged: %s", language, line.Text)
			}
		}
		if !strings.Contains(got[1].Text, "Original objection") || !strings.Contains(got[2].Text, "Original subject") {
			t.Fatal("language changed source words")
		}
		in.Commitments, in.Recent = nil, nil
		quiet := whatChangedSection(in, rankClaims(in))
		in.LastSpokeAt = nil
		first := whatChangedSection(in, rankClaims(in))
		if len(quiet) != 1 || len(first) != 1 || quiet[0].Text == first[0].Text || first[0].Nature != natureAssessment {
			t.Fatal("quiet spell became first contact")
		}
		for _, count := range []int{1, 3} {
			line := briefCopyFor(language).recentConversations(count, "Original subject")
			if !strings.Contains(line, "Original subject") || (count == 3 && !strings.Contains(line, "3")) {
				t.Fatalf("count or subject lost: %s", line)
			}
		}
	}
}
