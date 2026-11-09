// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"slices"

	"github.com/margince/margince/backend/internal/shared/kernel/occurredday"
)

// LastWord picks, from a non-empty tie group in the timeline's order, their message none of ours answers
// (occurredday.Answers), or else the first of ours. Capture order may rank two of ours, never an answer.
func LastWord[T any](tied []T, read func(T) (at occurredday.Moment, inbound bool)) T {
	ours := -1
	for i, row := range tied {
		at, inbound := read(row)
		if !inbound {
			if ours < 0 {
				ours = i
			}
			continue
		}
		answered := slices.ContainsFunc(tied, func(reply T) bool {
			replyAt, theirsToo := read(reply)
			return !theirsToo && occurredday.Answers(replyAt, at)
		})
		if !answered {
			return row
		}
	}
	return tied[max(ours, 0)]
}
