// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package handbook

import (
	"strings"
	"testing"
)

func TestSectionForNamesTheTimeZoneSectionAndItsSettingsPage(t *testing.T) {
	line := lineOf(t, "To change your own time zone in Margince")
	got, ok := SectionFor(ownSettings, line, line)
	if !ok {
		t.Fatalf("SectionFor(%q, %d) found no section", ownSettings, line)
	}
	if got.Title != "How do I change my time zone?" || got.Opens != "meetings" || got.Line != line-2 {
		t.Fatalf("SectionFor(%q, %d) = %+v, want the time zone section two lines up, opening meetings", ownSettings, line, got)
	}
}

func TestSectionForReadsTheHeadingLineItself(t *testing.T) {
	heading := lineOf(t, "### How do I change my time zone?")
	got, ok := SectionFor(ownSettings, heading, heading)
	if !ok || got.Line != heading {
		t.Fatalf("SectionFor on the heading line = %+v, %v; want the section that heading opens", got, ok)
	}
}

func TestSectionForAnswersNothingAboveTheFirstSectionOrForAPageNotShipped(t *testing.T) {
	if got, ok := SectionFor("your-own-settings.md", 1, 1); ok {
		t.Fatalf("line 1 sits above every section, got %+v", got)
	}
	if got, ok := SectionFor("no-such-page.md", 10, 10); ok {
		t.Fatalf("a page this build does not ship has no sections, got %+v", got)
	}
}

func TestASpanRunningIntoALaterHeadingIsAboutThatSection(t *testing.T) {
	heading := lineOf(t, "### How do I change my time zone?")
	got, ok := SectionFor(ownSettings, heading-3, heading+2)
	if !ok || got.Line != heading || got.Opens != "meetings" {
		t.Fatalf("a span from the section above into the time zone section = %+v, %v; want the time zone section", got, ok)
	}
	above, ok := SectionFor(ownSettings, heading-3, heading-1)
	if !ok || above.Line >= heading {
		t.Fatalf("a span ending above the heading = %+v, %v; want the section above", above, ok)
	}
}

func TestASpanOpeningOnAHeadingIsAboutThatSectionThoughItRunsIntoTheNext(t *testing.T) {
	heading := lineOf(t, "### How do I change my time zone?")
	next := lineOf(t, "### How do I share a booking link?")
	got, ok := SectionFor(ownSettings, heading, next+2)
	if !ok || got.Line != heading {
		t.Fatalf("a span from the time zone heading into the next section = %+v, %v; want the time zone section", got, ok)
	}
}

func TestAHeadingLosesItsClosingMarks(t *testing.T) {
	got := sections("## Account ##\n### Use `x`?\n### C# notes")
	want := []string{"Account", "Use `x`?", "C# notes"}
	for i, title := range want {
		if got[i].Title != title {
			t.Fatalf("sections[%d].Title = %q, want %q", i, got[i].Title, title)
		}
	}
}

func TestAHeadingMayStartWithUpToThreeSpaces(t *testing.T) {
	got := sections("# Page\n   ### Meetings\n    ### Indented code\n")
	if len(got) != 1 || got[0].Title != "Meetings" || got[0].Line != 2 {
		t.Fatalf("sections = %+v, want only the heading indented by three spaces", got)
	}
}

func TestSectionsSkipHeadingsInFencesAndComments(t *testing.T) {
	markdown := strings.Join([]string{
		"# Page",
		"## Real",
		"<!-- opens: settings/account -->",
		"```",
		"## In a fence",
		"```",
		"<!--",
		"### In a comment",
		"-->",
		"### Also real",
		"Text before the marker.",
		"<!-- opens: settings/meetings -->",
	}, "\n")
	got := sections(markdown)
	want := []Section{{Title: "Real", Line: 2, Opens: "account"}, {Title: "Also real", Line: 10}}
	if len(got) != len(want) {
		t.Fatalf("sections = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("sections[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

// TestEveryOpensMarkerSitsDirectlyUnderAHeading holds the marker rule over every
// shipped page. A marker anywhere else names a settings page for no section, so
// no answer would carry it.
func TestEveryOpensMarkerSitsDirectlyUnderAHeading(t *testing.T) {
	pages, err := Pages()
	if err != nil {
		t.Fatalf("reading the embedded handbook: %v", err)
	}
	markers := 0
	for _, page := range pages {
		lines := strings.Split(string(page.Content), "\n")
		for i, line := range lines {
			if !mentionsMarker(line) {
				continue
			}
			markers++
			if !opensMarker.MatchString(strings.TrimSpace(line)) {
				t.Errorf("%s:%d: %q is not `<!-- opens: settings/<page-id> -->`", page.Filename, i+1, line)
				continue
			}
			if _, heading := sectionHeading(lines[max(i-1, 0)]); i == 0 || !heading {
				t.Errorf("%s:%d: an opens marker must sit on the line directly under a ## or ### heading",
					page.Filename, i+1)
			}
		}
	}
	if markers == 0 {
		t.Fatal("no opens marker in the handbook: this gate read nothing and would pass on any page")
	}
}

// mentionsMarker is any line that names a settings page, or a comment that
// mentions opening one: a misspelt marker is still read, and then refused.
func mentionsMarker(line string) bool {
	folded := strings.ToLower(strings.TrimSpace(line))
	// A describes header names code paths, some under a settings/ folder; it
	// opens no settings page.
	if strings.HasPrefix(folded, "<!-- describes:") {
		return false
	}
	comment := strings.HasPrefix(folded, "<!--") && strings.Contains(folded, "open")
	return comment || strings.Contains(folded, "settings/")
}

// ownSettings is the page the section tests read: it holds the time zone section.
const ownSettings = "your-own-settings.md"

func lineOf(t *testing.T, text string) int {
	t.Helper()
	content, ok := Open(ownSettings)
	if !ok {
		t.Fatalf("this build ships no %s", ownSettings)
	}
	for i, line := range strings.Split(string(content), "\n") {
		if strings.HasPrefix(line, text) {
			return i + 1
		}
	}
	t.Fatalf("%s holds no line starting %q", ownSettings, text)
	return 0
}
