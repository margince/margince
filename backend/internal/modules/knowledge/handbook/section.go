// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package handbook

import (
	"regexp"
	"strings"
)

// Section is one `##` or `###` part of a page. It holds the heading, the line
// the heading sits on, and the settings page its answer is on, if it names one.
type Section struct {
	Title string
	// Line is the 1-based line of the heading.
	Line int
	// Opens is a settings page id from the `<!-- opens: settings/<id> -->`
	// marker on the line under the heading, or "" when the section has none.
	Opens string
}

// opensMarker is the whole line a page writes under a heading to name the
// settings page that section is about. The Markdown reader hides whole-line
// comments, so a reader of the page never sees it.
var opensMarker = regexp.MustCompile(`^<!-- opens: settings/([a-z][a-z-]*) -->$`)

// SectionAt is the section a line of a page sits in: the nearest `##` or `###`
// heading at or above it. False when this build does not ship the page, or
// when the line is above the page's first section.
func SectionAt(filename string, line int) (Section, bool) {
	content, ok := Open(filename)
	if !ok {
		return Section{}, false
	}
	var at Section
	found := false
	for _, s := range Sections(string(content)) {
		if s.Line > line {
			break
		}
		at, found = s, true
	}
	return at, found
}

// Sections lists a page's `##` and `###` headings in order, outside code
// fences and comments, each with the settings page its marker names.
func Sections(markdown string) []Section {
	lines := strings.Split(markdown, "\n")
	var out []Section
	var fence string
	var commented bool
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		switch {
		case commented:
			commented = !strings.Contains(trimmed, "-->")
		case fence != "":
			if closesFence(trimmed, fence) {
				fence = ""
			}
		case fenceOpening(trimmed) != "":
			fence = fenceOpening(trimmed)
		case strings.HasPrefix(trimmed, "<!--"):
			commented = !strings.Contains(trimmed, "-->")
		default:
			if title, ok := sectionHeading(line); ok {
				out = append(out, Section{Title: title, Line: i + 1, Opens: opensUnder(lines, i)})
			}
		}
	}
	return out
}

func sectionHeading(line string) (string, bool) {
	for _, mark := range []string{"### ", "## "} {
		if title, ok := strings.CutPrefix(line, mark); ok {
			return strings.TrimSpace(title), true
		}
	}
	return "", false
}

// opensUnder reads the marker on the line directly under the heading at i.
func opensUnder(lines []string, i int) string {
	if i+1 >= len(lines) {
		return ""
	}
	if m := opensMarker.FindStringSubmatch(strings.TrimSpace(lines[i+1])); m != nil {
		return m[1]
	}
	return ""
}
