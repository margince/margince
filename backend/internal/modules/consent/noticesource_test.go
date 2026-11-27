// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"encoding/json"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
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

// The contract lets a body that selects evidence leave out the note. A note is
// needed only when nothing is selected.
func TestAnAbsentNoteIsNeededOnlyWithoutSelectedEvidence(t *testing.T) {
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	for _, tc := range []struct {
		name, body string
		valid      bool
	}{
		{"evidence selected", `{"kind":"subject_initiated","selected_evidence":{"kind":"activity","id":"019ff000-0000-7000-8000-000000000001"}}`, true},
		{"nothing selected", `{"kind":"subject_initiated"}`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var req crmcontracts.AssessNoticeSource
			if err := json.Unmarshal([]byte(tc.body), &req); err != nil {
				t.Fatal(err)
			}
			_, _, err := sourceAssessmentOf(req, now).validate()
			if (err == nil) != tc.valid {
				t.Fatalf("err=%v, want valid=%t", err, tc.valid)
			}
		})
	}
}

func TestSourcePreviewRefusesLegacyCasesWhoseNoticeIsAlreadyQueuedOrFinished(t *testing.T) {
	for _, state := range []NoticeState{NoticeQueued, NoticeCompleted, NoticeSourceReview} {
		current := NoticeCase{State: state, Acquisition: &NoticeAcquisition{Kind: "unknown_legacy"}}
		_, err := projectSourceAssessment(current, SourceAssessment{Kind: "subject_initiated", Note: "Original correspondence"})
		if (err == nil) != (state == NoticeSourceReview) {
			t.Fatalf("state %s: %v", state, err)
		}
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
