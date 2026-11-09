// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package draftcheck

import (
	"regexp"
	"strings"
)

// Only a sentence offering an enclosure claims an outgoing file; thanking
// someone for a file they attached does not.
var attachmentClaim = regexp.MustCompile(
	`(?im)(?:^|[.!?]\s+)\s*(?:(?:as requested|wie gewünscht),?\s+)?(?:` +
		`please find attached|attached (?:is|are|you['’]ll find)|enclosed (?:is|are)|` +
		`i(?: have|'ve|’ve| am|'m|’m) (?:attached|attaching|enclosed)|` +
		`the [^.!?\n]{1,80} (?:is|are) attached|` +
		`anbei (?:sende ich|schicke ich|erhalten sie|findest du|finden sie)|` +
		`im anhang (?:finden sie|findest du|sende ich|schicke ich)|` +
		`ich habe (?:beigefügt|angehängt)|tôi đính kèm|mình đính kèm)`)

// ClaimsAttachment asks whether the draft promises a file not supplied to drafting.
func ClaimsAttachment(body string) bool { return attachmentClaim.MatchString(body) }

func attachmentFindings(body string) []Finding {
	phrase := strings.TrimSpace(attachmentClaim.FindString(body))
	if phrase == "" {
		return nil
	}
	return []Finding{{
		Rule: RuleUnattachedDocument, Phrase: phrase,
		Why: "no outgoing attachments are supplied; do not claim a file is attached or enclosed",
	}}
}
