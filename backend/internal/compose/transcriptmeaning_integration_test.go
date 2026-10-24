// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/activities"
)

func TestTranscriptReadingDistinguishesNonActionsFromInvalidMeaning(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		meaning    *commitmentMeaning
		proposals  int
		status     string
	}{
		{"open range", "Mira: I will send the scope next week.", &commitmentMeaning{State: "open", DeadlinePrecision: "range", DeadlineQuote: "next week"}, 1, activities.TranscriptReadDone},
		{"completed", "Mira: I sent the scope this morning.", &commitmentMeaning{State: "completed", DeadlinePrecision: "none"}, 0, activities.TranscriptReadDone},
		{"conditional", "Mira: I will send the scope if the buyer agrees.", &commitmentMeaning{State: "conditional", Condition: "if the buyer agrees", DeadlinePrecision: "none"}, 0, activities.TranscriptReadDone},
		{"missing meaning", "Mira: I will send the scope.", nil, 0, activities.TranscriptReadFailed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := setupTranscript(t)
			at := time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC)
			if _, err := e.Activities.UpdateActivity(e.ctx, e.activity, activities.UpdateActivityInput{Body: &tc.body, OccurredAt: &at}); err != nil {
				t.Fatal(err)
			}
			reply, err := json.Marshal(map[string]any{"proposals": []proposedStep{{Summary: "Send the scope", Owner: "Mira", PromiserRef: "seat:" + e.Rep1.String(), SourceLines: []int{1}, Confidence: 1, Meaning: tc.meaning}}})
			if err != nil {
				t.Fatal(err)
			}
			done := e.readAt(t, cannedBrain{reply: string(reply)}, func() time.Time { return at })
			if done.Status != tc.status || len(done.ProposalIDs) != tc.proposals {
				t.Fatalf("read = %+v, want status %s and %d proposals", done, tc.status, tc.proposals)
			}
			if tc.meaning != nil && tc.meaning.withheldOutcome() != "" {
				want := transcriptCopyFor("en").transcriptWithheldReason(tc.meaning.withheldOutcome())
				if done.StatusDetail == nil || !strings.Contains(*done.StatusDetail, want) {
					t.Fatalf("read detail = %v, want %q", done.StatusDetail, want)
				}
			}
			if e.taskCount(t) != 0 {
				t.Fatal("an unverified actor or non-action created a task")
			}
		})
	}
}
