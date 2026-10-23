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

const (
	settingsPage    = "your-own-settings.md"
	timeZoneHeading = "### How do I change my time zone?"
)

// handbookPassage is lines [first, last] of the shipped settings page as one
// passage. It is cut from this build's own copy, so SectionFor reads its lines.
func handbookPassage(t *testing.T, first, last int) knowledge.Passage {
	t.Helper()
	lines := settingsPageLines(t)
	return knowledge.Passage{
		ChunkID:      ids.NewV7(),
		DocumentID:   ids.NewV7(),
		DocumentName: settingsPage,
		Text:         strings.Join(lines[first-1:last], "\n"),
		Similarity:   0.8,
		StartLine:    first,
	}
}

// timeZonePassage is the time zone section, heading to the blank line after it.
func timeZonePassage(t *testing.T) knowledge.Passage {
	t.Helper()
	heading := handbookLineOf(t, timeZoneHeading)
	lines := settingsPageLines(t)
	end := heading
	for end < len(lines) && lines[end] != "" {
		end++
	}
	return handbookPassage(t, heading, end)
}

func onlyClaim(t *testing.T, answer crmcontracts.KnowledgeAnswer) crmcontracts.KnowledgeClaim {
	t.Helper()
	if answer.Claims == nil || len(*answer.Claims) != 1 {
		t.Fatalf("outcome %s carried claims %v, want exactly one", answer.Outcome, answer.Claims)
	}
	return (*answer.Claims)[0]
}

func handbookState(shipped bool) knowledge.Readiness {
	state := answeredState()
	state.Corpus.Shipped = shipped
	return state
}

func askHandbook(t *testing.T, shipped bool, lane corpusAskLane, passage knowledge.Passage) crmcontracts.KnowledgeAnswer {
	t.Helper()
	return AnswerCorpus(t.Context(), lane, handbookState(shipped), "how do I change my time zone",
		[]knowledge.Passage{passage}, string(textlang.English), corpusQuietLog())
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
	claim := onlyClaim(t, askHandbook(t, true, timeZoneLane(t, passage), passage))
	if claim.Section == nil || claim.Section.Title != "How do I change my time zone?" || claim.Section.Line != passage.StartLine {
		t.Fatalf("section = %+v, want the time zone heading at line %d", claim.Section, passage.StartLine)
	}
	if claim.Opens == nil || claim.Opens.SettingsPage != "meetings" {
		t.Fatalf("opens = %+v, want settings page meetings", claim.Opens)
	}
}

// With no lane the claim is the bare passage, which starts wherever the chunker
// cut it. One starting on the tail of the section above is still about the
// section its lines run into.
func TestAnUnreviewedPassageNamesTheSectionItRunsInto(t *testing.T) {
	heading := handbookLineOf(t, timeZoneHeading)
	answer := askHandbook(t, true, nil, handbookPassage(t, heading-3, heading+3))
	if answer.Outcome != crmcontracts.KnowledgeAnswerOutcomeKnowledgeAnswerOutcomeUnreviewed {
		t.Fatalf("outcome = %s, want unreviewed with no lane", answer.Outcome)
	}
	claim := onlyClaim(t, answer)
	if claim.Section == nil || claim.Section.Line != heading {
		t.Fatalf("section = %+v, want the time zone heading at line %d", claim.Section, heading)
	}
	if claim.Opens == nil || claim.Opens.SettingsPage != "meetings" {
		t.Fatalf("opens = %+v, want settings page meetings on the bare passage", claim.Opens)
	}
}

// An uploaded set may hold a file named like a handbook page. Its lines are not
// the handbook's, so nothing is read from the page.
func TestAClaimFromAnUploadedSetNamesNoSection(t *testing.T) {
	passage := timeZonePassage(t)
	claim := onlyClaim(t, askHandbook(t, false, timeZoneLane(t, passage), passage))
	if claim.Section != nil || claim.Opens != nil {
		t.Fatalf("an uploaded set's claim carries section %+v and opens %+v", claim.Section, claim.Opens)
	}
}

func TestAClaimWithNoLineOrOnAPageNotShippedIsLeftAsItIs(t *testing.T) {
	line := 40
	claims := []crmcontracts.KnowledgeClaim{
		{DocumentName: settingsPage},
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
	claims := []crmcontracts.KnowledgeClaim{{DocumentName: settingsPage, Line: &line}}
	stampHandbookSections(claims)
	if claims[0].Section != nil {
		t.Fatalf("line 3 sits above the first section, got %+v", claims[0].Section)
	}
	open := handbookLineOf(t, "### How do I open my own settings?") + 1
	claims = []crmcontracts.KnowledgeClaim{{DocumentName: settingsPage, Line: &open}}
	stampHandbookSections(claims)
	if claims[0].Section == nil || claims[0].Opens != nil {
		t.Fatalf("section %+v opens %+v; want a section and no settings page", claims[0].Section, claims[0].Opens)
	}
}

func settingsPageLines(t *testing.T) []string {
	t.Helper()
	content, ok := handbook.Open(settingsPage)
	if !ok {
		t.Fatalf("this build ships no %s", settingsPage)
	}
	return strings.Split(string(content), "\n")
}

func handbookLineOf(t *testing.T, heading string) int {
	t.Helper()
	for i, line := range settingsPageLines(t) {
		if line == heading {
			return i + 1
		}
	}
	t.Fatalf("no %q in %s", heading, settingsPage)
	return 0
}
