// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"testing"
	"time"
)

func TestSourceAssessmentRequiresEvidenceAndRejectsFutureDates(t *testing.T) {
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	yesterday, tomorrow := now.AddDate(0, 0, -1), now.AddDate(0, 0, 1)
	for _, tc := range []struct {
		name, kind, note string
		obtained         *time.Time
		valid, owed      bool
	}{
		{"unknown", "unknown_legacy", "Evidence", &yesterday, false, false},
		{"unrecognized", "new_source", "Evidence", &yesterday, false, false},
		{"no evidence", "subject_initiated", " ", nil, false, false},
		{"direct", "subject_initiated", "Original incoming message", nil, true, false},
		{"referral date missing", "referral", "Referral evidence", nil, true, true},
		{"future date", "referral", "Referral evidence", &tomorrow, false, false},
		{"referral", "referral", "Referral evidence", &yesterday, true, true},
		{"form date missing", "event_or_form", "Registration", nil, true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, owed, err := (SourceAssessment{Kind: tc.kind, ObtainedAt: tc.obtained, Note: tc.note, Now: now}).validate()
			if (err == nil) != tc.valid || (err == nil && owed != tc.owed) {
				t.Fatalf("owed=%t err=%v", owed, err)
			}
		})
	}
}

func TestConfirmedIndirectSourcesKeepTheirNoticeDuty(t *testing.T) {
	for _, kind := range []string{"referral", "purchased_or_imported", "public_or_business_source", "event_or_form"} {
		duty, owed := DutyFor(kind)
		if !owed || duty.State != NoticeOpen || len(duty.Routes) == 0 {
			t.Errorf("%s: %#v, owed=%t", kind, duty, owed)
		}
	}
}

func TestAssessedDeadlinesUseTheAcquisitionCalendarAcrossMonthEndsAndDST(t *testing.T) {
	for _, tc := range []struct{ zone, acquired, expected string }{
		{"Asia/Ho_Chi_Minh", "2026-01-30T17:00:00Z", "2026-02-27T17:00:00Z"},
		{"Europe/Berlin", "2026-03-15T23:00:00Z", "2026-04-15T22:00:00Z"},
		{"Europe/Berlin", "2026-09-30T22:00:00Z", "2026-10-31T23:00:00Z"},
	} {
		acquired, err := time.Parse(time.RFC3339, tc.acquired)
		if err != nil {
			t.Fatal(err)
		}
		due, _, err := (SourceAssessment{ObtainedAt: &acquired, Timezone: tc.zone}).deadline(strictNotice(), true, time.Time{})
		if err != nil || due == nil || due.UTC().Format(time.RFC3339) != tc.expected {
			t.Errorf("%s acquired %s: due=%v err=%v", tc.zone, tc.acquired, due, err)
		}
	}
}
