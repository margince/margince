// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package httperr

import (
	"slices"
	"strings"
)

// withoutSentinelWords takes the sentinel's words out of every text of fault a
// caller reads.
func withoutSentinelWords(fault Fault) Fault {
	fault.Detail = withoutSentinelText(fault.Detail)
	// A copy, because Fields may be the slice of the caller's own DetailedError.
	fields := slices.Clone(fault.Fields)
	for i := range fields {
		fields[i].Message = withoutSentinelText(fields[i].Message)
	}
	fault.Fields = fields
	return fault
}

// The machine code carries the sentinel; its words add no guidance to a named refusal.
func withoutSentinelText(detail string) string {
	for {
		trimmed := detail
		for _, m := range mapping {
			trimmed = strings.TrimSuffix(trimmed, ": "+m.sentinel.Error())
		}
		if trimmed == detail {
			return detail
		}
		detail = trimmed
	}
}
