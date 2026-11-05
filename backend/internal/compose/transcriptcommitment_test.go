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
	staged := newTranscriptStaging()
	staged.tasks[ids.NewV7()] = 1
	staged.watched = 1
	staged.proposals = []activities.TranscriptProposal{{ApprovalID: ids.NewV7(), SeatID: ids.NewV7()}}
	staged.withheld[CommitmentOutcome(activities.CommitmentHistorical)] = 1
	staged.withheld[CommitmentOutcome(activities.CommitmentSourceUnknown)] = 1
	detail := transcriptCopyFor("en").outcomeSentence(*staged.facts(4), transcriptAudience{})
	for _, want := range []string{"added 1 task", "1 commitment", "staged 1 suggestion", "1 commitment: kept as history", "1 commitment: needs a verified source date"} {
		if !strings.Contains(detail, want) {
			t.Errorf("detail %q omits %q", detail, want)
		}
	}
	if strings.Contains(detail, "colleague") || strings.Contains(detail, "you") {
		t.Errorf("the stored sentence %q speaks to someone; it is read by everyone who sees the record", detail)
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
