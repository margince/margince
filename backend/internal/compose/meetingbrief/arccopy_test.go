// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package meetingbrief

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/langcopy/langcopytest"
)

func TestArcCopyKeepsDatesTitlesAndConversationCounts(t *testing.T) {
	langcopytest.Census(t, arcWords)
	langcopytest.Census(t, activityWords)
	for _, language := range []string{"en", "de", "vi"} {
		said := briefCopyFor(language)
		for _, title := range []string{"", "Original title"} {
			for _, count := range []int{1, 3} {
				line := said.arcSummary(ArcMoment{Title: title, From: instantOn(2), To: instantOn(5)}, count)
				if !strings.Contains(line, "2026-08-02–2026-08-05") || (title != "" && !strings.Contains(line, title)) || (count == 3 && !strings.Contains(line, "3")) {
					t.Fatalf("%s lost arc facts: %s", language, line)
				}
				if language != "en" && strings.Contains(line, "conversation") {
					t.Fatalf("%s arc stayed English", language)
				}
			}
		}
		for kind := range activityWords {
			activity := ActIn{Kind: string(kind), Subject: "Original subject"}
			if said.subjectOrKind(activity) != activity.Subject {
				t.Fatal("translated recorded subject")
			}
			activity.Subject = ""
			if said.subjectOrKind(activity) == "" {
				t.Fatal("known kind lacks caption")
			}
		}
		if said.subjectOrKind(ActIn{Kind: "future_kind"}) != "future_kind" {
			t.Fatal("unknown kind changed")
		}
	}
}

func TestUngroundedAdvanceFallsBackInTheBriefLanguage(t *testing.T) {
	for _, tc := range []struct{ language, prefix string }{{"en", "Leave with"}, {"de", "Vereinbare"}, {"vi", "Kết thúc"}} {
		in := fullInput()
		in.Language = tc.language
		got := wireAdvance(Advance{}, in, knownRecords(in))
		if !strings.HasPrefix(got.Minimum.Text, tc.prefix) || len(got.Minimum.Evidence) != 1 || got.Best.Text == "" || got.Fallback.Text == "" {
			t.Fatalf("%s lost grounded localized fallback: %+v", tc.language, got)
		}
	}
}
