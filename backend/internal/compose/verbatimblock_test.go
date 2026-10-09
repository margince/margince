// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import "strings"

// verbatimBlock fences text a model or client receives, so a reference page
// quotes it rather than presenting it as the page's own prose. It wraps at
// spaces only, so the words are the served words and only line breaks differ.
func verbatimBlock(text string) string {
	const width = 100
	var out strings.Builder
	out.WriteString("```text\n")
	for paragraph := range strings.SplitSeq(text, "\n") {
		line := 0
		for i, word := range strings.Fields(paragraph) {
			if i > 0 && line+1+len(word) > width {
				out.WriteString("\n")
				line = 0
			} else if i > 0 {
				out.WriteString(" ")
				line++
			}
			out.WriteString(word)
			line += len(word)
		}
		out.WriteString("\n")
	}
	out.WriteString("```\n")
	return out.String()
}
