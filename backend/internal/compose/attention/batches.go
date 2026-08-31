// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// Routine decisions, as ONE row each kind.
//
// A hundred and fifty "is this address a contact?" questions are one kind of
// work. Asking them one at a time is what buried the customers on the page this
// replaces — and it is not a ranking problem, because no ordering saves a
// reader who must scroll past a hundred and fifty rows to reach the next thing.
//
// So the pile becomes a row. The reader answers the group, or opens it to
// answer the exceptions, and the ordering compares one hygiene row against one
// customer rather than a hundred against one.
//
// A decision that BLOCKS a customer is never folded: it is not routine, whatever
// it has in common with the rest.

import (
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// batchFloor is how many alike decisions it takes before a group reads better
// than the rows themselves.
//
// Three, not two: a reader meeting two similar questions answers both faster
// than they would open a group, and a "batch of 2" is a row that costs more
// than it saves. It is the pile that needs collapsing, not the pair.
const batchFloor = 3

// foldRoutineDecisions replaces alike routine decisions with one row each.
//
// The members are kept in the fold's order, so the sample names the ones the
// reader would have met first. Everything that is not routine passes through
// untouched.
func foldRoutineDecisions(rows []ranked) []ranked {
	groups := map[crmcontracts.WorklistBatchKey][]ranked{}
	kept := make([]ranked, 0, len(rows))
	for _, row := range rows {
		key, groupable := batchKeyOf(row)
		if !groupable {
			kept = append(kept, row)
			continue
		}
		groups[key] = append(groups[key], row)
	}
	// A group under the floor is not a pile; its rows go back as themselves.
	for key, members := range groups {
		if len(members) < batchFloor {
			kept = append(kept, members...)
			continue
		}
		kept = append(kept, batchRow(key, members))
	}
	return kept
}

// batchKeyOf says which group a row belongs to, and whether it may be grouped
// at all.
//
// Only LEVEL 6 decisions are foldable. A decision that blocks a customer sits
// at level 5 and stays its own row however many of it there are: the reader has
// to see each one, because each holds up somebody different.
func batchKeyOf(row ranked) (crmcontracts.WorklistBatchKey, bool) {
	if row.item.Category != "decisions" || row.item.Level != levelRoutine {
		return "", false
	}
	if row.item.Source == "dedupe_candidate" {
		return "duplicates", true
	}
	if row.item.Kind == nil {
		return "", false
	}
	switch *row.item.Kind {
	case "capture_counterparty":
		return contactBatchKey(row), true
	case "held_draft", "scheduled_send_held":
		return "held_draft", true
	default:
		return "", false
	}
}

// contactBatchKey splits the contact questions into the three the concept names.
//
// The split matters because the three are answered differently: a machine
// sender is rejected without thought, an address whose company we already know
// is usually accepted, and the remainder is the part that actually needs a
// person. One group of a hundred and fifty would still be a pile.
func contactBatchKey(row ranked) crmcontracts.WorklistBatchKey {
	switch {
	case row.machineSender:
		return "likely_automated"
	case row.knownCompany:
		return "company_match"
	default:
		return "uncertain_contact"
	}
}

// batchSample is how many members a batch names.
//
// Enough to recognise the kind, few enough to stay one line. A reader who
// cannot see what is in a group has to trust it, and a group nobody trusts is
// worse than the pile it replaced.
const batchSample = 3

// batchRow draws one group as a queue item.
//
// It carries no subject: a batch is about no single record, and offering `open`
// would send the reader to whichever member happened to be first. Its verb is
// the batch screen, which the client routes from the source alone.
func batchRow(key crmcontracts.WorklistBatchKey, members []ranked) ranked {
	sample := make([]string, 0, batchSample)
	for _, member := range members {
		if len(sample) == batchSample {
			break
		}
		if member.item.Title != nil && *member.item.Title != "" {
			sample = append(sample, *member.item.Title)
		}
	}
	count := len(members)
	row := crmcontracts.WorklistItem{
		// The key IS the id: one row per kind per read, and a client that
		// re-reads finds the same row rather than a new one each time.
		Id:          string(key),
		Source:      "batch",
		Category:    "decisions",
		Level:       levelRoutine,
		Consequence: "data_drifts",
		Because:     []crmcontracts.WorklistReason{reason("routine", nil)},
		Batch: &crmcontracts.WorklistBatch{
			Key:    key,
			Count:  count,
			Sample: &sample,
		},
		Actions: []crmcontracts.WorklistItemActions{},
	}
	// The oldest member's moment, so a batch sorts where its work would have.
	occurred := members[0].occurredAt
	for _, member := range members {
		if member.occurredAt.Before(occurred) {
			occurred = member.occurredAt
		}
	}
	return ranked{item: row, occurredAt: occurred}
}
