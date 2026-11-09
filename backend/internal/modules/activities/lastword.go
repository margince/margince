// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"slices"

	"github.com/margince/margince/backend/internal/shared/kernel/occurredday"
)

// LastWord picks, from a record's exchanges at its newest stored instant listed
// newest first, the one whose move the record reads. That is their message none
// of ours at that instant answers (occurredday.Answers), or else our newest.
// tied is never empty.
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
