// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"fmt"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/langcopy/langcopytest"
)

func TestTranscriptExplanationLanguageKeepsEveryOutcomeAndCount(t *testing.T) {
	langcopytest.Census(t, transcriptWords)
	for _, language := range []string{"de", "vi"} {
		said := transcriptCopyFor(language)
		for _, count := range []int{1, 2} {
			staged := transcriptStaging{taskHolders: []holderCount{{"Anna Example", count}}, watched: count, proposals: make([]ids.UUID, count), proposalHolders: []holderCount{{"Anna Example", count}}, withheld: map[CommitmentOutcome]int{CommitmentOutcome(activities.CommitmentHistorical): count}}
			got := said.transcriptReadDetail(count, staged)
			if strings.Count(got, fmt.Sprint(count)) != 4 || got == transcriptCopyFor("en").transcriptReadDetail(count, staged) {
				t.Fatalf("%s changed the outcome counts or kept English: %s", language, got)
			}
		}
		tooLong := fmt.Errorf("private context: %w", &activities.TranscriptTooLongError{Lines: 777, Chars: 88888})
		detail, terminal := said.unreadableTranscript(tooLong)
		if !terminal || !strings.Contains(detail, "777") || !strings.Contains(detail, "88888") || strings.Contains(detail, "private context") {
			t.Fatalf("%s lost measured counts or exposed the wrapper: %s", language, detail)
		}
	}
}
