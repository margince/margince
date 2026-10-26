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

// SectionFor names the section a span of lines, first to last, is about.
// A span that opens on a heading is about that section. A span that runs from
// the tail of one section into a heading is about the later one. The chunker
// cuts a passage wherever it likes, so both happen.
// False when this build does not ship the page, or when the span lies above the
// page's first section.
func SectionFor(filename string, first, last int) (Section, bool) {
	content, ok := Open(filename)
	if !ok {
		return Section{}, false
	}
	var at Section
	found := false
	for _, s := range sections(string(content)) {
		if s.Line > max(first, last) {
			break
		}
		at, found = s, true
		if s.Line >= first {
			break
		}
	}
	return at, found
}

// sections lists a page's `##` and `###` headings in order, outside code
// fences and comments, each with the settings page its marker names.
func sections(markdown string) []Section {
	lines := strings.Split(markdown, "\n")
	var out []Section
	for i, line := range proseLines(markdown) {
		if title, ok := sectionHeading(line); ok {
			out = append(out, Section{Title: title, Line: i + 1, Opens: opensUnder(lines, i)})
		}
	}
	return out
}

// sectionHeading reads a `##` or `###` heading, without the closing `#` run
// CommonMark allows after a space. Up to three leading spaces still make a
// heading; four make a code block.
func sectionHeading(line string) (string, bool) {
	if trimmed := strings.TrimLeft(line, " "); len(line)-len(trimmed) < 4 {
		line = trimmed
	}
	for _, mark := range []string{"### ", "## "} {
		if title, ok := strings.CutPrefix(line, mark); ok {
			title = strings.TrimSpace(title)
			if closed := strings.TrimRight(title, "#"); strings.HasSuffix(closed, " ") {
				title = strings.TrimSpace(closed)
			}
			return title, true
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
