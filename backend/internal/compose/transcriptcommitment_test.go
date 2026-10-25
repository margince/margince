// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestTranscriptReadReportsEveryKindOfResult(t *testing.T) {
	detail := transcriptCopyFor("en").transcriptReadDetail(4, transcriptStaging{tasks: 1, watched: 1, proposals: []ids.UUID{ids.NewV7()}, withheld: map[CommitmentOutcome]int{CommitmentOutcome(activities.CommitmentHistorical): 1, CommitmentOutcome(activities.CommitmentSourceUnknown): 1}})
	for _, want := range []string{"1 task", "1 commitment", "1 proposal", "1 commitment: kept as history", "1 commitment: needs a verified source date"} {
		if !strings.Contains(detail, want) {
			t.Errorf("detail %q omits %q", detail, want)
		}
	}
}

func TestTranscriptReviewReasonsNameTheActualMissingEvidence(t *testing.T) {
	for reason, want := range map[activities.CommitmentAdmissionReason]string{
		activities.CommitmentMeetingPending:    "meeting that has not taken place",
		activities.CommitmentMeetingCancelled:  "canceled meeting",
		activities.CommitmentContextUnassessed: "later conversation evidence",
	} {
		if got := transcriptCopyFor("en").transcriptWithheldReason(CommitmentOutcome(reason)); !strings.Contains(got, want) {
			t.Errorf("%s rendered %q, want %q", reason, got, want)
		}
	}
}

func TestTranscriptSemanticReasonsDescribeTheReading(t *testing.T) {
	for state, want := range map[string]string{"completed": "already completed", "conditional": "unmet condition", "optional": "optional offer", "request": "request nobody has accepted", "unclear": "does not establish", "unassessed": "does not establish"} {
		got := transcriptCopyFor("en").transcriptWithheldReason(CommitmentOutcome("semantic_" + state))
		if !strings.Contains(got, want) {
			t.Errorf("%s: %q omits %q", state, got, want)
		}
	}
}
