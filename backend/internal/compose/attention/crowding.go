// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// The anti-monopoly rule: how much of ONE kind of work a reader meets before
// they meet the others.
//
// It used to be two `if i >= 8` lines, one inside the who-is-waiting lane and
// one inside the owed-leads lane. Fifteen further lanes reach the page —
// meetings, the overnight brief, commitments, failed approvals, DSR, at-risk
// deals, planned tasks, bounces, undelivered mail, decisions, relationship
// decay, four health lanes, notices and introductions — and none of them had a
// cap at all. Six overdue tasks, or six bounces, produced a page whose top rows
// were all the same sentence, and nothing demoted them.
//
// So the rule moves off the two lanes that happened to have it and onto the
// thing every row carries: the SOURCE it names. A lane added tomorrow is capped
// the day it ships rather than the day somebody remembers.

import (
	"sort"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// crowdLead is how many rows of one source lead the page.
//
// Not a cap on the source: the rest are still ranked, still on the page and
// still reachable. It is a cap on how much of one kind a reader meets before
// they see the others, and the number answers "how many can somebody act on
// this morning" rather than "how many are there" — which is the number the two
// lanes that had this rule already carried, kept here so widening the rule to
// the other fifteen does not also change what the two it came from produce.
const crowdLead = 8

// markCrowding marks every row past the lead group of its own source.
//
// ORDERED BY THE RANKING ITSELF, with crowding switched off. Which members of a
// source lead has to be decided somehow, and the two lanes this replaces each
// answered it with a sort of their own — waits by when the customer wrote,
// leads by their deadline. Fifteen lanes have no such sort to borrow, and
// inventing one per lane would be seventeen answers to one question. The
// ordering already answers it for every row on the page, so the group that
// leads is the group the reader would have met first anyway.
//
// Not circular, though it reads that way: every flag is false when this runs,
// so the ordering's crowding step decides nothing and cannot decide anything.
// What it walks is the page as it would stand with the rule switched off, which
// is exactly the question "who would have led".
//
// It marks in place and returns the same slice, so a caller cannot keep an
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
			// Past the lead, a row sorts below the other kinds without ceasing
			// to be what it is: its level still says a customer is waiting,
			// because that is what it is and the summary counts on it. What
			// changes is only where it sits.
			//
			// Rewriting the level instead would have told the reader the ninth
			// waiting customer was agreed work, while the row went on saying a
			// buyer wrote last — the page contradicting itself.
			rows[i].crowded = true
		}
	}
	return rows
}
