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
	seat := ids.NewV7()
	for _, language := range []string{"de", "vi"} {
		said := transcriptCopyFor(language)
		for _, count := range []int{1, 2} {
			staged := newTranscriptStaging()
			staged.tasks[seat] = count
			staged.watched = count
			staged.proposals = make([]activities.TranscriptProposal, count)
			staged.withheld[CommitmentOutcome(activities.CommitmentHistorical)] = count
			facts := *staged.facts(count)
			got := said.outcomeSentence(facts, transcriptAudience{})
			if strings.Count(got, fmt.Sprint(count)) != 4 || got == transcriptCopyFor("en").outcomeSentence(facts, transcriptAudience{}) {
				t.Fatalf("%s changed the outcome counts or kept English: %s", language, got)
			}
		}
		tooLong := fmt.Errorf("private context: %w", &activities.TranscriptTooLongError{Lines: 777, Chars: 88888})
		failure, terminal := unreadableTranscript(tooLong)
		if !terminal {
			t.Fatalf("a transcript too long to read was not a terminal refusal")
		}
		detail := said.outcomeSentence(activities.TranscriptReadFacts{Failure: failure}, transcriptAudience{})
		if !strings.Contains(detail, "777") || !strings.Contains(detail, "88888") || strings.Contains(detail, "private context") {
			t.Fatalf("%s lost measured counts or exposed the wrapper: %s", language, detail)
		}
	}
}
