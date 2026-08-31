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

// The two approval kinds that are a message the product WROTE and is holding.
//
// Constants rather than literals at each site: the classifier and the grouper
// both ask about them, and a typo in either would put the same decision in two
// places while both halves still compiled.
const (
	// categorySystem is the badge a broken pipe wears. A constant because the
	// grouper asks about it three times and a typo would silently stop an
	// incident grouping while every half still compiled.
	categorySystem = crmcontracts.WorklistItemCategory("system")

	kindHeldDraft     = "held_draft"
	kindScheduledSend = "scheduled_send_held"
)

// foldRoutineDecisions replaces alike routine decisions with one row each.
//
// The members are kept in the fold's order, so the sample names the ones the
// reader would have met first. Everything that is not routine passes through
// untouched.
func foldRoutineDecisions(rows []ranked) []ranked {
	return foldRoutineDecisionsBounded(rows, false)
}

// foldRoutineDecisionsBounded is the same fold, told whether the read that
// produced these rows stopped at its own bound.
//
// A group whose members were cut short says so: "200+" rather than "200",
// because a bound printed as a total is a wrong number rather than a bounded
// one, and the reader has no way to tell the two apart.
func foldRoutineDecisionsBounded(rows []ranked, bounded bool) []ranked {
	// Keyed by the group AND its cause: every contact question shares one cause
	// (there is none to name), while two failing rules are two incidents that
	// must not read as one.
	type groupKey struct {
		key   crmcontracts.WorklistBatchKey
		cause string
	}
	groups := map[groupKey][]ranked{}
	order := []groupKey{}
	kept := make([]ranked, 0, len(rows))
	for _, row := range rows {
		key, groupable := batchKeyOf(row)
		if !groupable {
			kept = append(kept, row)
			continue
		}
		cause, _ := systemCause(row)
		at := groupKey{key: key, cause: cause}
		if _, seen := groups[at]; !seen {
			order = append(order, at)
		}
		groups[at] = append(groups[at], row)
	}
	// A group under the floor is not a pile; its rows go back as themselves.
	// Walked in the order the groups were met, so one read's answer is the
	// next read's answer rather than whatever the map iterated.
	for _, at := range order {
		members := groups[at]
		if len(members) < batchFloor {
			kept = append(kept, members...)
			continue
		}
		kept = append(kept, batchRow(at.key, at.cause, members, bounded))
	}
	return kept
}

// systemCause is what a broken system row has in common with its siblings: the
// rule that failed, the AI task that failed, the mailbox that stopped.
//
// A row with no cause it can name does not group. Grouping by SOURCE alone
// would put a failing mailbox and a failing rule in one row, which tells the
// reader two things are broken and names neither.
func systemCause(row ranked) (string, bool) {
	if row.item.Category != categorySystem {
		return "", false
	}
	switch row.item.Source {
	case "notice":
		// A notice is addressed to one person about one thing. Two of them are
		// two messages, not one condition.
		return "", false
	case "bounce":
		// A bounced email is a CUSTOMER CONSEQUENCE, not a system condition:
		// this customer did not get this message, and somebody has to fix that
		// address. Three bounces are three customers, and grouping them by
		// their provider's reason would hide two of them behind one row —
		// which is the distinction the whole incident rule rests on.
		return "", false
	}
	// WHICH field names the cause depends on the producer, and getting this
	// wrong is the difference between one incident and a hundred.
	//
	// An automation's title IS the rule ("Post-meeting recap draft"), stable
	// across every firing, so eight failures of it are one broken rule. Its
	// `kind` is the outcome — "failed" — which every broken thing shares, so
	// grouping on that would file a dead mailbox under the same heading.
	//
	// An AI run's title is its own SUMMARY, written per run: grouping on it
	// would draw a hundred and sixty-three incidents for one broken task. Its
	// `kind` is what ran, which is the thing that is broken.
	if row.item.Source == "automation_run" && row.item.Title != nil && *row.item.Title != "" {
		return *row.item.Title, true
	}
	if row.item.Kind == nil || *row.item.Kind == "" {
		return "", false
	}
	return *row.item.Kind, true
}

// batchKeyOf says which group a row belongs to, and whether it may be grouped
// at all.
//
// Only LEVEL 6 decisions are foldable. A decision that blocks a customer sits
// at level 5 and stays its own row however many of it there are: the reader has
// to see each one, because each holds up somebody different.
func batchKeyOf(row ranked) (crmcontracts.WorklistBatchKey, bool) {
	if row.item.Category == categorySystem {
		if _, ok := systemCause(row); ok {
			return "system_incident", true
		}
		return "", false
	}
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
	case kindHeldDraft:
		return kindHeldDraft, true
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

// batchID names one group. Two causes under one key are two groups, so the
// cause is part of the identity rather than only its description.
func batchID(key crmcontracts.WorklistBatchKey, cause string) string {
	if cause == "" {
		return string(key)
	}
	return string(key) + ":" + cause
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
func batchRow(key crmcontracts.WorklistBatchKey, cause string, members []ranked, bounded bool) ranked {
	sample := make([]string, 0, batchSample)
	for _, member := range members {
		if len(sample) == batchSample {
			break
		}
		if member.item.Title != nil && *member.item.Title != "" {
			sample = append(sample, *member.item.Title)
		}
	}
	// An incident is not hygiene. It says something is BROKEN, and while it is
	// broken every quiet claim on this page is suspect — a mailbox that stopped
	// makes "nobody is waiting" a sentence the product cannot support. So it
	// keeps its members' own band and their own consequence rather than being
	// filed with the routine tidying.
	level, category, consequence := levelRoutine,
		crmcontracts.WorklistItemCategory("decisions"),
		crmcontracts.WorklistItemConsequence("data_drifts")
	if key == "system_incident" {
		level = members[0].item.Level
		category = categorySystem
		consequence = members[0].item.Consequence
	}
	count := len(members)
	row := crmcontracts.WorklistItem{
		// The key and its cause ARE the id: one row per kind per cause per read,
		// so a client that re-reads finds the same row rather than a new one,
		// and two failing rules stay two rows.
		Id:          batchID(key, cause),
		Source:      "batch",
		Category:    category,
		Level:       level,
		Consequence: consequence,
		Because:     []crmcontracts.WorklistReason{reason("routine", nil)},
		Batch: &crmcontracts.WorklistBatch{
			Key:    key,
			Count:  count,
			Sample: &sample,
		},
		Actions: []crmcontracts.WorklistItemActions{},
	}
	if cause != "" {
		row.Batch.Cause = &cause
	}
	if bounded {
		atLeast := true
		row.Batch.AtLeast = &atLeast
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
