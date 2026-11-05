// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package draftcore

import (
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

func TestDatedNoteSilenceKeepsItsStatedDay(t *testing.T) {
	zone, err := time.LoadLocation("Pacific/Kiritimati")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 6, 12, 9, 0, 0, 0, zone)
	notes := []crmcontracts.Activity{{
		Kind:              crmcontracts.ActivityKindNote,
		OccurredAt:        time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC),
		OccurredPrecision: "day",
	}}
	state := ClassifyWithLogged(now, zone, time.Time{}, time.Time{}, notes)
	if state.SilenceDays != 2 {
		t.Fatalf("silence = %d, want 2 from the stated date", state.SilenceDays)
	}
}

func TestLoggedTouchOrderFollowsTheRecordCalendar(t *testing.T) {
	zone, err := time.LoadLocation("Pacific/Kiritimati")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 6, 12, 9, 0, 0, 0, zone)
	notes := []crmcontracts.Activity{
		{Kind: crmcontracts.ActivityKindNote, OccurredAt: time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC), OccurredPrecision: "day"},
		{Kind: crmcontracts.ActivityKindNote, OccurredAt: time.Date(2026, 6, 10, 11, 0, 0, 0, time.UTC), OccurredPrecision: "instant"},
	}
	state := ClassifyWithLogged(now, zone, time.Time{}, time.Time{}, notes)
	if state.SilenceDays != 1 {
		t.Fatalf("silence = %d, want 1 from the latest local activity day", state.SilenceDays)
	}
}
