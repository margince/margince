// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A reading that gave the logger a task and put a colleague's promise to that
// colleague says so to each of them: "you" for their own, a name for the other.
func TestTheReadingSaysWhoEachTaskAndProposalWentTo(t *testing.T) {
	logger, colleague := ids.NewV7(), ids.NewV7()
	facts := activities.TranscriptReadFacts{
		Found:     2,
		Tasks:     []activities.TranscriptSeatCount{{SeatID: logger, Count: 1}},
		Proposals: []activities.TranscriptProposal{{ApprovalID: ids.NewV7(), SeatID: colleague}},
	}
	names := map[ids.UUID]string{logger: "Lena Logger", colleague: "Anna Example"}
	en := transcriptCopyFor("en")

	toLogger := en.outcomeSentence(facts, transcriptAudience{reader: logger, names: names})
	if want := "added 1 task for you; sent 1 suggestion to Anna Example to confirm"; toLogger != want {
		t.Errorf("the logger reads %q, want %q", toLogger, want)
	}
	toColleague := en.outcomeSentence(facts, transcriptAudience{reader: colleague, names: names})
	if want := "added 1 task for Lena Logger; 1 proposal awaits your decision"; toColleague != want {
		t.Errorf("the colleague reads %q, want %q", toColleague, want)
	}
	unnamed := en.outcomeSentence(facts, transcriptAudience{reader: logger})
	if !strings.Contains(unnamed, "sent 1 suggestion to a colleague to confirm") {
		t.Errorf("a seat with no name to show reads %q, want it called a colleague", unnamed)
	}
	german := transcriptCopyFor("de").outcomeSentence(facts, transcriptAudience{reader: logger, names: names})
	if want := "1 Aufgabe für dich erstellt; 1 Vorschlag an Anna Example zur Bestätigung gesendet"; german != want {
		t.Errorf("a German reader reads %q, want %q", german, want)
	}
}

// Every failure code a reading can close with phrases as its own sentence, and
// an unknown code falls back to the refusal rather than to nothing.
func TestEveryTranscriptFailureCodeRenders(t *testing.T) {
	en := transcriptCopyFor("en")
	seen := map[string]string{}
	for _, code := range []string{
		activities.TranscriptFailureNotTranscript, activities.TranscriptFailureTooLong, activities.TranscriptFailureBlank,
		activities.TranscriptFailureMissing, activities.TranscriptFailureRefused, activities.TranscriptFailureNoModel,
	} {
		got := en.outcomeSentence(activities.TranscriptReadFacts{Failure: &activities.TranscriptFailure{Code: code, Lines: 9, Chars: 99}}, transcriptAudience{})
		if got == "" {
			t.Fatalf("%s renders nothing", code)
		}
		if other, dup := seen[got]; dup {
			t.Errorf("%s and %s render the same sentence %q", code, other, got)
		}
		seen[got] = code
	}
	if got := en.outcomeSentence(activities.TranscriptReadFacts{Failure: &activities.TranscriptFailure{Code: "later_code"}}, transcriptAudience{}); got != en.Say(transcriptWords.Refused) {
		t.Errorf("an unknown failure code renders %q, want the refusal", got)
	}
}
