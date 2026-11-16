// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"strings"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/mailcopy"
)

// groundsLimit is the contract's maxLength for a reason, in characters; a cut
// text spends one of them on its ellipsis.
const groundsLimit = 600

// GroundsFromProse is why a piece of work exists, taken from what its maker
// wrote about it: the first paragraph, as one line. Nil when they wrote none.
func GroundsFromProse(prose string) *crmcontracts.WorkGrounds {
	text := mailcopy.Snippet(firstParagraph(prose), groundsLimit-1)
	if text == "" {
		return nil
	}
	return &crmcontracts.WorkGrounds{Text: text}
}

// TaskGrounds is a task's reason, from its text as the reader sees it:
// Margince's own words in the reader's language, else what its author wrote.
func TaskGrounds(task crmcontracts.Activity) *crmcontracts.WorkGrounds {
	switch {
	case task.SystemDisplay != nil && task.SystemDisplay.Body != nil:
		return GroundsFromProse(*task.SystemDisplay.Body)
	case task.Body != nil:
		return GroundsFromProse(*task.Body)
	}
	return nil
}

// firstParagraph is the first block of prose, without its headings: a
// heading names a section, it never says why the work exists.
func firstParagraph(markdown string) string {
	for block := range strings.SplitSeq(strings.ReplaceAll(markdown, "\r\n", "\n"), "\n\n") {
		var prose []string
		for line := range strings.SplitSeq(block, "\n") {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "#") {
				continue
			}
			if line = strings.TrimSpace(strings.TrimLeft(line, ">-*+ ")); line != "" {
				prose = append(prose, line)
			}
		}
		if len(prose) > 0 {
			return strings.Join(prose, " ")
		}
	}
	return ""
}
