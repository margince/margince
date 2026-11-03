// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package mailquote

import (
	"regexp"
	"slices"
)

// An Outlook reply header is a sender field with a sent-date field within the
// next few lines. "Von:" alone is ordinary prose in German mail.
var (
	replyHeaderFrom = regexp.MustCompile(`^\*{0,2}(Von|From):\*{0,2}\s`)
	replyHeaderSent = regexp.MustCompile(`^\*{0,2}(Gesendet|Sent|Datum|Date):\*{0,2}\s`)
)

// replyHeaderWindow is how many lines from the sender field the sent-date
// field may sit.
const replyHeaderWindow = 5

// OpensReplyHeader reports whether line starts an Outlook quote block, given
// the lines that follow it.
func OpensReplyHeader(line string, following []string) bool {
	if !replyHeaderFrom.MatchString(line) {
		return false
	}
	return slices.ContainsFunc(following[:min(replyHeaderWindow-1, len(following))], replyHeaderSent.MatchString)
}
