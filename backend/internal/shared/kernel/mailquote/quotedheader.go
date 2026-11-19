// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package mailquote

import (
	"slices"
	"strings"
)

// QuotedHeader is the header block a mail program writes above forwarded or
// quoted mail. Every value is the text as written, which its sender could edit.
type QuotedHeader struct {
	From, Subject, Date string
}

// The labels of a quoted header block, by the field they fill.
var (
	fromLabels    = []string{"from", "von"}
	subjectLabels = []string{"subject", "betreff"}
	dateLabels    = []string{"date", "sent", "datum", "gesendet"}
)

// quotedHeaderReach is how many lines under an attribution the block may run.
// Past that the text is the quoted mail's body, and a "From:" there is prose.
const quotedHeaderReach = 8

// FirstQuotedHeader reads the first header block that names a sender: under an
// attribution line (IsAttribution), or an Outlook block on its own
// (OpensReplyHeader). A From line anywhere else is somebody's prose.
func FirstQuotedHeader(lines []string) (QuotedHeader, bool) {
	trimmed := make([]string, len(lines))
	for i, line := range lines {
		trimmed[i] = strings.TrimSpace(line)
	}
	for i, line := range trimmed {
		start := -1
		switch {
		case IsAttribution(line):
			start = i + 1
		case OpensReplyHeader(line, trimmed[i+1:]):
			start = i
		}
		if start < 0 {
			continue
		}
		if header := readQuotedHeader(trimmed[start:min(len(trimmed), start+quotedHeaderReach)]); header.From != "" {
			return header, true
		}
	}
	return QuotedHeader{}, false
}

func readQuotedHeader(block []string) QuotedHeader {
	var header QuotedHeader
	for _, line := range block {
		label, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		label = strings.ToLower(strings.Trim(label, " *"))
		value = strings.TrimSpace(strings.Trim(value, " *"))
		switch {
		case slices.Contains(fromLabels, label) && header.From == "":
			header.From = value
		case slices.Contains(subjectLabels, label) && header.Subject == "":
			header.Subject = value
		case slices.Contains(dateLabels, label) && header.Date == "":
			header.Date = value
		}
	}
	return header
}
