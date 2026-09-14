// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package webread

import (
	"strings"

	"golang.org/x/net/html"
)

// HeadingSections preserves the blocks a page labels with headings. The
// ordinary Text contract stays whitespace-normalized; callers needing entity
// attribution can retain these boundaries without rewriting stored evidence.
func HeadingSections(doc string) []string {
	tokens := html.NewTokenizer(strings.NewReader(doc))
	var raw strings.Builder
	var sections []string
	headings := 0
	flush := func() {
		if text := StripTags(raw.String()); text != "" {
			sections = append(sections, text)
		}
		raw.Reset()
	}
	for {
		kind := tokens.Next()
		if kind == html.ErrorToken {
			flush()
			break
		}
		chunk := string(tokens.Raw())
		if kind == html.StartTagToken {
			name, _ := tokens.TagName()
			if len(name) == 2 && name[0] == 'h' && name[1] >= '1' && name[1] <= '6' {
				flush()
				headings++
			}
		}
		raw.WriteString(chunk)
	}
	if headings == 0 {
		return nil
	}
	return sections
}
