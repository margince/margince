// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package stateddeadline

import (
	"testing"
	"time"
)

var sentThursday = Day{2026, time.October, 22}

func TestADeadlineWordFollowedByOneDayStatesThatDay(t *testing.T) {
	for text, want := range map[string]string{
		"Request for materials | Deadline: 2026-10-26":      "2026-10-26",
		"Please send the logo | Deadline: 27 October":       "2026-10-27",
		"Bitte bis 30.10.2026 zurückschicken":               "2026-10-30",
		"Could you send it by Friday, October 30?":          "2026-10-30",
		"Unterlagen fällig am 3. November 2026":             "2026-11-03",
		"Deadline: 5 Jan":                                   "2027-01-05",
		"We need the signed copy no later than 2 November.": "2026-11-02",
	} {
		got, ok := In(text, sentThursday)
		if !ok || got.String() != want {
			t.Errorf("In(%q) = %s, %v; want %s", text, got, ok, want)
		}
	}
}

func TestNoDeadlineWordOrNoSingleDayStatesNothing(t *testing.T) {
	for _, text := range []string{
		"Meeting notes from 27 October",
		"Please send it before 30 October",
		"Deadline: 10/11/2026",
		"Deadline: 31 April 2026",
		"Please reply by email",
		"My hobby 2026-10-26 is painting",
		"Overdue 30.10.2026",
		"Deadline: next week",
		"",
	} {
		if got, ok := In(text, sentThursday); ok {
			t.Errorf("In(%q) = %s; want no day", text, got)
		}
	}
}

func TestThisWeekIsTheFridayOfTheWeekItWasWrittenIn(t *testing.T) {
	for sent, want := range map[Day]string{
		{2026, time.October, 19}: "2026-10-23",
		sentThursday:             "2026-10-23",
		{2026, time.October, 23}: "2026-10-23",
		{2026, time.October, 25}: "2026-10-23",
	} {
		got, ok := ThisWeek("I will send it to you this week to sign.", sent, nil)
		if !ok || got.String() != want {
			t.Errorf("ThisWeek sent %s = %s, %v; want %s", sent, got, ok, want)
		}
	}
	for _, words := range []string{"next week", "diese Woche", "tuần này", "by the end of the week"} {
		_, ok := ThisWeek(words, sentThursday, nil)
		if ok != (words != "next week") {
			t.Errorf("ThisWeek(%q) = %v", words, ok)
		}
	}
}

func TestThisWeekEndsOnTheSendersLastWorkingDay(t *testing.T) {
	got, ok := ThisWeek("this week", sentThursday, []int{1, 2, 3, 4})
	if !ok || got.String() != "2026-10-22" {
		t.Fatalf("a Monday-to-Thursday week ends %s, %v; want 2026-10-22", got, ok)
	}
	got, ok = ThisWeek("this week", sentThursday, []int{1, 2, 3, 4, 5, 6})
	if !ok || got.String() != "2026-10-24" {
		t.Fatalf("a six-day week ends %s, %v; want 2026-10-24", got, ok)
	}
}
