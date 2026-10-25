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
	detail := transcriptCopyFor("en").transcriptReadDetail(4, transcriptStaging{taskHolders: []holderCount{{"Lars Example", 1}}, watched: 1, proposals: []ids.UUID{ids.NewV7()}, proposalHolders: []holderCount{{"Anna Example", 1}}, withheld: map[CommitmentOutcome]int{CommitmentOutcome(activities.CommitmentHistorical): 1, CommitmentOutcome(activities.CommitmentSourceUnknown): 1}})
	for _, want := range []string{"added 1 task for Lars Example", "1 commitment", "sent 1 suggestion to Anna Example to confirm", "1 commitment: kept as history", "1 commitment: needs a verified source date"} {
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

func TestTranscriptReadNamesWhoEachTaskAndSuggestionWentTo(t *testing.T) {
	staged := transcriptStaging{
		taskHolders:     []holderCount{{"Lars Example", 1}, {"Anna Example", 2}},
		proposalHolders: []holderCount{{"", 1}},
	}
	got := transcriptCopyFor("en").transcriptReadDetail(4, staged)
	want := "added 1 task for Lars Example; added 2 tasks for Anna Example; sent 1 suggestion to a colleague to confirm"
	if got != want {
		t.Fatalf("detail = %q, want %q", got, want)
	}
}
