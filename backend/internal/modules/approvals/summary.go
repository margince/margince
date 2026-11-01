// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package approvals

import (
	"encoding/json"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/textcut"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// maxSummaryLen bounds the sentence an inbox row shows. Long enough for an
// operation, a target and the fields that matter; short enough that a
// crafted prefix cannot push them past what a human reads.
const maxSummaryLen = 240

// SanitizeSummary bounds untrusted approval captions and removes characters
// that can hide or reverse the action a reader is deciding.
func SanitizeSummary(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	lastWasSpace := false
	for _, r := range s {
		switch {
		case r == utf8.RuneError:
			continue // invalid encoding: not text, not shown
		case unicode.IsSpace(r):
			if !lastWasSpace && b.Len() > 0 {
				b.WriteByte(' ')
				lastWasSpace = true
			}
		case values.IsInvisibleRune(r):
			// Cf covers the bidirectional overrides and isolates
			// (U+202A-E, U+2066-9), the directional marks (U+200E/F,
			// U+061C) and the zero-width joiners a homograph relies on.
			continue
		default:
			b.WriteRune(r)
			lastWasSpace = false
		}
	}
	return boundRunes(strings.TrimSpace(b.String()), maxSummaryLen)
}

// SanitizeSummaryParts applies SanitizeSummary to every piece. A field whose
// name is left empty is dropped and counted among the fields left out.
func SanitizeSummaryParts(parts *crmcontracts.ApprovalSummaryParts) *crmcontracts.ApprovalSummaryParts {
	if parts == nil {
		return nil
	}
	clean := crmcontracts.ApprovalSummaryParts{
		Sentence:   SanitizeSummary(parts.Sentence),
		Fields:     make([]crmcontracts.ApprovalSummaryField, 0, len(parts.Fields)),
		MoreFields: max(parts.MoreFields, 0),
	}
	for _, field := range parts.Fields {
		name := SanitizeSummary(field.Field)
		if name == "" {
			clean.MoreFields++
			continue
		}
		clean.Fields = append(clean.Fields, crmcontracts.ApprovalSummaryField{Field: name, Value: SanitizeSummary(field.Value)})
	}
	return &clean
}

// marshalSummaryParts is the summary_parts column value: NULL for no parts.
func marshalSummaryParts(parts *crmcontracts.ApprovalSummaryParts) (json.RawMessage, error) {
	if parts == nil {
		return nil, nil
	}
	encoded, err := json.Marshal(parts)
	if err != nil {
		return nil, fmt.Errorf("encode approval summary parts: %w", err)
	}
	return encoded, nil
}

// readSummaryParts decodes the stored column. A value that does not decode
// reads as absent, and the reader falls back to the joined summary.
func readSummaryParts(stored json.RawMessage) *crmcontracts.ApprovalSummaryParts {
	if len(stored) == 0 {
		return nil
	}
	var parts crmcontracts.ApprovalSummaryParts
	if json.Unmarshal(stored, &parts) != nil {
		return nil
	}
	return &parts
}

// withholdPayload clears what a reader may not see once the preview held the
// staged call back. That is the call, its evidence and both summary forms.
func withholdPayload(approval *crmcontracts.Approval) {
	approval.ProposedChange, approval.Evidence = nil, nil
	approval.Summary, approval.SummaryParts = nil, nil
}

// boundRunes truncates so the RESULT is at most n bytes, marking that it did
// so — a silently cut summary reads as a complete one. The ellipsis is part of
// the budget, not added on top of it: a caller that passes a limit means the
// output, and a bound that overshoots by three bytes is not a bound.
func boundRunes(s string, n int) string {
	if len(s) <= n {
		return s
	}
	const ellipsis = "…"
	return strings.TrimSpace(textcut.Bytes(s, n-len(ellipsis))) + ellipsis
}
