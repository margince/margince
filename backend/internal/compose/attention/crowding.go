// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// The anti-monopoly rule: how much of ONE kind of work a reader meets before
// they meet the others. Six overdue tasks, or six bounces, otherwise fill the
// top of a page with the same sentence and nothing demotes them.
//
// It hangs off the SOURCE every row carries rather than off the individual
// lanes, so a lane added tomorrow is capped the day it ships.

import (
	"sort"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// crowdLead is how many rows of one source lead the page. Not a cap on the
// source — the rest stay ranked and reachable — and the number answers "how many
// can somebody act on this morning" rather than "how many are there".
const crowdLead = 8

// markCrowding marks every row past the lead group of its own source.
//
// ORDERED BY THE RANKING ITSELF, with crowding switched off: the ordering
// already decides which rows a reader meets first, so a per-lane sort would be
// seventeen answers to one question. Not circular, though it reads that way —
// every flag is false when this runs, so the ordering's crowding step decides
// nothing and what it walks is the page as it would stand with the rule off.
//
// Marks in place and returns the same slice, so a caller cannot keep an
// unmarked copy by accident.
func markCrowding(rows []ranked) []ranked {
	order := make([]int, len(rows))
	for i := range order {
		order[i] = i
	}
	sort.SliceStable(order, func(a, b int) bool { return less(rows[order[a]], rows[order[b]]) })
	led := make(map[crmcontracts.WorklistItemSource]int, len(rows))
	for _, i := range order {
		source := rows[i].item.Source
		led[source]++
		if led[source] > crowdLead {
			// Only where it sits changes: the level still says a customer is
			// waiting, because that is what it is and the summary counts on it.
			// Rewriting the level would have the page contradict itself.
			rows[i].crowded = true
		}
	}
	return rows
}
