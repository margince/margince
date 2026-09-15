// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import "strings"

// legalSectionText restores heading boundaries before the ordinary passage
// packer runs. Short company blocks stay whole; short subheadings still pack
// together and long blocks retain the same sentence/rune limits. Headings
// affect packing, never the scope of the legal attribution check.
func legalSectionText(page crawlPage) string {
	if len(page.Sections) == 0 {
		return page.Text
	}
	var out strings.Builder
	cursor := 0
	for _, section := range page.Sections {
		if section == "" {
			continue
		}
		at := strings.Index(page.Text[cursor:], section)
		if at < 0 {
			// A malformed document may reduce differently section by
			// section. Only the original Text can supply evidence.
			continue
		}
		start := cursor + at
		out.WriteString(page.Text[cursor:start])
		out.WriteByte('\n')
		out.WriteString(section)
		out.WriteByte('\n')
		cursor = start + len(section)
	}
	out.WriteString(page.Text[cursor:])
	return out.String()
}
