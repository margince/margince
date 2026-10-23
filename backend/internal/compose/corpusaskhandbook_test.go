// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/knowledge"
	"github.com/margince/margince/backend/internal/modules/knowledge/handbook"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

const timeZoneHeading = "### How do I change my time zone?"

// timeZonePassage is the shipped handbook's time zone section as one passage.
// It is cut from this build's own page, so its start line is the one SectionAt reads.
func timeZonePassage(t *testing.T) knowledge.Passage {
	t.Helper()
	content, ok := handbook.Open("your-own-settings.md")
	if !ok {
		t.Fatal("this build ships no your-own-settings.md")
	}
	lines := strings.Split(string(content), "\n")
	for i, line := range lines {
		if line != timeZoneHeading {
			continue
		}
		end := i
		for end < len(lines) && lines[end] != "" {
			end++
		}
		return knowledge.Passage{
			ChunkID:      ids.NewV7(),
			DocumentID:   ids.NewV7(),
			DocumentName: "your-own-settings.md",
			Text:         strings.Join(lines[i:end], "\n"),
			Similarity:   0.8,
			StartLine:    i + 1,
		}
	}
	t.Fatalf("your-own-settings.md has no %q section", timeZoneHeading)
	return knowledge.Passage{}
}

func handbookState(shipped bool) knowledge.Readiness {
	state := answeredState()
	state.Corpus.Shipped = shipped
	return state
}

func askTimeZone(t *testing.T, shipped bool, lane corpusAskLane) crmcontracts.KnowledgeAnswer {
	t.Helper()
	return AnswerCorpus(t.Context(), lane, handbookState(shipped), "how do I change my time zone",
		[]knowledge.Passage{timeZonePassage(t)}, string(textlang.English), corpusQuietLog())
}

func timeZoneLane(t *testing.T, passage knowledge.Passage) *fixedLane {
	t.Helper()
	return &fixedLane{text: corpusReply(askedClaim{
		Text:  "Pick your time zone on the Meetings page.",
		ID:    passage.ChunkID.String(),
		Quote: "choose a place in the **Timezone** list",
	})}
}

func TestAWrittenClaimFromTheShippedHandbookNamesItsSectionAndSettingsPage(t *testing.T) {
	passage := timeZonePassage(t)
	answer := AnswerCorpus(t.Context(), timeZoneLane(t, passage), handbookState(true),
		"how do I change my time zone", []knowledge.Passage{passage}, string(textlang.English), corpusQuietLog())
	if answer.Claims == nil || len(*answer.Claims) != 1 {
		t.Fatalf("claims = %v, want the one grounded claim", answer.Claims)
	}
	claim := (*answer.Claims)[0]
	if claim.Section == nil || claim.Section.Title != "How do I change my time zone?" || claim.Section.Line != passage.StartLine {
		t.Fatalf("section = %+v, want the time zone heading at line %d", claim.Section, passage.StartLine)
	}
	if claim.Opens == nil || claim.Opens.SettingsPage != "meetings" {
		t.Fatalf("opens = %+v, want settings page meetings", claim.Opens)
	}
}

func TestUnreviewedHandbookPassagesStillNameTheirSection(t *testing.T) {
	answer := askTimeZone(t, true, nil)
	if answer.Outcome != crmcontracts.KnowledgeAnswerOutcomeKnowledgeAnswerOutcomeUnreviewed {
		t.Fatalf("outcome = %s, want unreviewed with no lane", answer.Outcome)
	}
	claim := (*answer.Claims)[0]
	if claim.Opens == nil || claim.Opens.SettingsPage != "meetings" {
		t.Fatalf("opens = %+v, want settings page meetings on the bare passage", claim.Opens)
	}
}

// An uploaded set may hold a file named like a handbook page. Its lines are not
// the handbook's, so nothing is read from the page.
func TestAClaimFromAnUploadedSetNamesNoSection(t *testing.T) {
	passage := timeZonePassage(t)
	answer := AnswerCorpus(t.Context(), timeZoneLane(t, passage), handbookState(false),
		"how do I change my time zone", []knowledge.Passage{passage}, string(textlang.English), corpusQuietLog())
	claim := (*answer.Claims)[0]
	if claim.Section != nil || claim.Opens != nil {
		t.Fatalf("an uploaded set's claim carries section %+v and opens %+v", claim.Section, claim.Opens)
	}
}

func TestAClaimWithNoLineOrOnAPageNotShippedIsLeftAsItIs(t *testing.T) {
	line := 40
	claims := []crmcontracts.KnowledgeClaim{
		{DocumentName: "your-own-settings.md"},
		{DocumentName: "a-page-this-build-dropped.md", Line: &line},
	}
	stampHandbookSections(claims)
	for _, claim := range claims {
		if claim.Section != nil || claim.Opens != nil {
			t.Fatalf("claim on %s was stamped with %+v / %+v", claim.DocumentName, claim.Section, claim.Opens)
		}
	}
}

func TestASectionWithNoMarkerNamesNoSettingsPage(t *testing.T) {
	line := 3
	claims := []crmcontracts.KnowledgeClaim{{DocumentName: "your-own-settings.md", Line: &line}}
	stampHandbookSections(claims)
	if claims[0].Section != nil {
		t.Fatalf("line 3 sits above the first section, got %+v", claims[0].Section)
	}
	open := handbookLineOf(t, "### How do I open my own settings?") + 1
	claims = []crmcontracts.KnowledgeClaim{{DocumentName: "your-own-settings.md", Line: &open}}
	stampHandbookSections(claims)
	if claims[0].Section == nil || claims[0].Opens != nil {
		t.Fatalf("section %+v opens %+v; want a section and no settings page", claims[0].Section, claims[0].Opens)
	}
}

func handbookLineOf(t *testing.T, heading string) int {
	t.Helper()
	content, ok := handbook.Open("your-own-settings.md")
	if !ok {
		t.Fatal("this build ships no your-own-settings.md")
	}
	for i, line := range strings.Split(string(content), "\n") {
		if line == heading {
			return i + 1
		}
	}
	t.Fatalf("no %q in your-own-settings.md", heading)
	return 0
}
