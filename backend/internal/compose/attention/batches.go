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
// keySystemIncident groups repeated failures of ONE broken thing. It is the one
// batch key whose members are system trouble rather than routine tidying, and
// several rules read it to tell the two apart.
const keySystemIncident = crmcontracts.WorklistBatchKey("system_incident")

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
		// The bound belongs to the lane the members CAME from. It is the
		// decision lane's own scan depth, and a system incident read from a
		// different lane never hit it — marking one "8+" because an unrelated
		// lane filled up reports a number the reader cannot check.
		kept = append(kept, batchRow(at.key, at.cause, members, bounded && at.key != keySystemIncident))
	}
	return kept
}

// systemCause is the CONDITION a broken-system row reports: the rule that
// fails, the mailbox that stopped, the AI task that broke.
//
// It reads one field, `cause_ref`, which each producer sets to its own real
// identity — a rule id, a task kind, a condition-and-mailbox pair — and which
// is namespaced by source, because the condition words are not unique across
// producers: capture and overlay sync both say `sync_failing`, and one heading
// covering both would tell a reader two things are broken and name neither.
//
// Nothing here reads a display field. An automation's NAME is mutable and not
// unique, so two rules sharing one would merge and a rename would split a
// rule's own history. An AI run's TITLE is its own per-run summary, so it draws
// one incident per failure rather than one per broken task. Both were tried.
//
// A row with no `cause_ref` reports no shared condition and never groups. That
// is the safe direction: an ungrouped row is one row too many on the page,
// while a wrongly grouped one hides a failure the reader never learns about —
// which is why a BOUNCE sets none. A bounce is a customer consequence, not a
// system condition: this named person did not get this message, and folding
// three of them behind one row hides two customers.
func systemCause(row ranked) (string, bool) {
	if row.item.Category != categorySystem {
		return "", false
	}
	if row.item.CauseRef == nil || *row.item.CauseRef == "" {
		return "", false
	}
	return *row.item.CauseRef, true
}

// causeLabelOf is the words for the condition a group was formed on.
//
// The members share one cause, so they carry one label, and the first that has
// it answers for all. It walks rather than reading members[0] because a lane can
// name the condition on some of its rows and not others — an automation run
// whose rule was deleted between firings has the id and no name — and taking the
// first row's absence for the group's would leave a named condition unnamed.
//
// An empty answer is a group with no name of its own, which the client draws by
// its kind. It never falls back to the cause: that is an identity, and putting
// it on screen is the defect this pair of fields exists to prevent.
func causeLabelOf(members []ranked) string {
	for _, member := range members {
		if member.item.CauseLabel != nil && *member.item.CauseLabel != "" {
			return *member.item.CauseLabel
		}
	}
	return ""
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
			return keySystemIncident, true
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
// groupReason says why these rows are one row. An incident is not tidying: it
// reports that one thing broke repeatedly, and calling that "routine" tells the
// reader to leave a dead mailbox for later.
func groupReason(key crmcontracts.WorklistBatchKey) crmcontracts.WorklistReasonKind {
	if key == keySystemIncident {
		return "repeated_failure"
	}
	return "routine"
}

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
	if key == keySystemIncident {
		// The MOST urgent member's band, not the first one read. The members
		// arrive in the lane's own order, which is not urgency, so taking the
		// first would rank an incident by whichever failure happened to be
		// read first and could file an urgent one below a routine row.
		level, consequence = members[0].item.Level, members[0].item.Consequence
		for _, member := range members[1:] {
			if member.item.Level < level {
				level, consequence = member.item.Level, member.item.Consequence
			}
		}
		category = categorySystem
	}
	count := len(members)
	from := make([]crmcontracts.WorklistItemSource, 0, count)
	for _, member := range members {
		from = append(from, member.item.Source)
	}
	row := crmcontracts.WorklistItem{
		// The key and its cause ARE the id: one row per kind per cause per read,
		// so a client that re-reads finds the same row rather than a new one,
		// and two failing rules stay two rows.
		Id:          batchID(key, cause),
		Source:      "batch",
		Category:    category,
		Level:       level,
		Consequence: consequence,
		Because:     []crmcontracts.WorklistReason{reason(groupReason(key), nil)},
		Batch: &crmcontracts.WorklistBatch{
			Key:    key,
			Count:  count,
			Sample: &sample,
		},
		Actions: []crmcontracts.WorklistItemActions{},
	}
	if cause != "" {
		row.Batch.Cause = &cause
		// And the words for it. The members of a group share their condition, so
		// they share its name — this is a lookup among rows already in hand, not
		// a second read, and NOT the sampled member's own title: the members are
		// alike rather than identical, and one member's subject describes that
		// member instead of the thing they have in common.
		if label := causeLabelOf(members); label != "" {
			row.Batch.Label = &label
		}
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
	return ranked{item: row, foldedFrom: from, occurredAt: occurred}
}
