// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/datatask"
)

func TestUpdateHealthDistinguishesOperatorWorkFromAutomaticProgress(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	later, earlier := now.Add(time.Minute), now.Add(-time.Minute)
	cases := []struct {
		name      string
		progress  datatask.Progress
		code      string
		condition crmcontracts.HealthSourceCondition
		included  bool
	}{
		{name: "completed", progress: datatask.Progress{Status: "done"}},
		{name: "unrequested manual task", progress: datatask.Progress{BlockedReason: "review_required"}},
		{name: "failed", progress: datatask.Progress{Status: "failed", NextAttemptAt: &later}, code: "updates_failed", condition: healthTechnical, included: true},
		{name: "paused", progress: datatask.Progress{Status: "paused"}, code: "updates_review", condition: healthWaitingHuman, included: true},
		{name: "review requested", progress: datatask.Progress{Version: 1, BlockedReason: "review_required"}, code: "updates_review", condition: healthWaitingHuman, included: true},
		{name: "missing prerequisite", progress: datatask.Progress{BlockedReason: "missing_prerequisite"}, code: "updates_review", condition: healthWaitingHuman, included: true},
		{name: "newer release needed", progress: datatask.Progress{BlockedReason: "newer_release_required"}, code: "updates_review", condition: healthWaitingHuman, included: true},
		{name: "scheduled retry", progress: datatask.Progress{Status: "pending", NextAttemptAt: &later}, code: "updates_retrying", condition: healthRetrying, included: true},
		{name: "retry due", progress: datatask.Progress{Status: "pending", NextAttemptAt: &earlier}, code: "updates_processing", condition: healthProcessing, included: true},
		{name: "running", progress: datatask.Progress{Status: "running"}, code: "updates_processing", condition: healthProcessing, included: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			code, condition, included := dataTaskHealth(tc.progress, now)
			if code != tc.code || condition != tc.condition || included != tc.included {
				t.Fatalf("got %q/%q included=%v; want %q/%q included=%v", code, condition, included, tc.code, tc.condition, tc.included)
			}
		})
	}
}
