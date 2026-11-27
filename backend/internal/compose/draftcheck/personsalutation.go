// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package draftcheck

import (
	"github.com/margince/margince/backend/internal/shared/kernel/draftfloor"
	"regexp"
	"strings"
)

var addressedTitle = regexp.MustCompile(`(?i)\b(mr|mrs|ms|herr|frau)\.?\s+\p{L}`)

// A title is permitted only by the named recipient's stored choice.
// Group mail and familiar greetings use the neutral floor instead.
func wrongSalutation(opening string, envelope draftfloor.Envelope, to Grounds) bool {
	found := addressedTitle.FindStringSubmatch(opening)
	if len(found) == 0 {
		return false
	}
	if len(to.CoRecipients) > 0 || !envelope.Formal() || strings.TrimSpace(to.LastName) == "" {
		return true
	}
	title := "ms"
	if strings.EqualFold(found[1], "mr") || strings.EqualFold(found[1], "herr") {
		title = "mr"
	}
	return title != envelope.Salutation
}
