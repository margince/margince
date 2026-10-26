// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package meetingbrief

import (
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/langcopy/langcopytest"
)

func TestMeetingHistoryWordsKeepNamesAndRelativeDays(t *testing.T) {
	langcopytest.Census(t, touchWords)
	for _, language := range []string{"en", "de", "vi"} {
		said := briefCopyFor(language)
		for _, days := range []int{0, 1, 3} {
			touched := instantOn(10 - days)
			attendee := AttendeeIn{FullName: "Original Name", Title: "Original Title", DealRole: "economic_buyer", LastTouch: &touched}
			line := said.attendeeLine(attendee, at(10), time.UTC)
			if !strings.Contains(line, attendee.FullName) || !strings.Contains(line, attendee.Title) {
				t.Fatalf("%s lost attendee facts: %s", language, line)
			}
			if language != "en" && strings.Contains(line, "economic buyer") {
				t.Fatalf("%s did not reuse localized role: %s", language, line)
			}
			prior := said.priorMeetingLine(PriorMeetingIn{Subject: "Original Subject", StartsAt: touched}, at(10), time.UTC)
			touch := lastTouchLine(Input{Language: language, LastTouchAt: &touched, Now: at(10), Zone: time.UTC})
			if !strings.Contains(prior, "Original Subject") {
				t.Fatalf("%s lost subject: %s", language, prior)
			}
			if days == 3 && (!strings.Contains(prior, "3") || !strings.Contains(touch, "3") || !strings.Contains(line, "3")) {
				t.Fatalf("%s lost elapsed days", language)
			}
			if days == 1 && (strings.Contains(line, "1 days") || strings.Contains(line, "1 Tagen")) {
				t.Fatalf("singular: %s", line)
			}
		}
		first := said.attendeeLine(AttendeeIn{FullName: "Original Name"}, at(10), time.UTC)
		if first == "" || lastTouchLine(Input{Language: language}) == "" {
			t.Fatalf("%s lost first contact", language)
		}
	}
}
