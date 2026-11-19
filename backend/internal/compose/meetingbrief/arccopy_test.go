// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package meetingbrief

import (
	"fmt"
	"strings"
	"testing"
	"time"

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
				if !strings.Contains(line, augustDay(language, 2)+"–"+augustDay(language, 5)) || (title != "" && !strings.Contains(line, title)) || (count == 3 && !strings.Contains(line, "3")) {
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

// augustDay is how each shipped language's locale notation writes a day of
// August 2026.
func augustDay(language string, day int) string {
	if language == "de" {
		return fmt.Sprintf("%02d.08.2026", day)
	}
	return fmt.Sprintf("%02d/08/2026", day)
}

// A day entry is stored at noon UTC. East of +12 that instant is the next day,
// and the arc must still name the day it was logged.
func TestAnArcEndpointLoggedAsADayKeepsItsDateEastOfTwelve(t *testing.T) {
	kiritimati, err := time.LoadLocation("Pacific/Kiritimati")
	if err != nil {
		t.Fatalf("zone: %v", err)
	}
	said := briefCopyFor("en")
	said.dates.Zone = kiritimati
	logged := Occurred{At: time.Date(2026, time.August, 9, 12, 0, 0, 0, time.UTC), DateOnly: true}
	if line := said.arcSummary(ArcMoment{From: logged, To: logged}, 1); !strings.Contains(line, augustDay("en", 9)) {
		t.Fatalf("arc line %q, want the logged day %s", line, augustDay("en", 9))
	}
}
