// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package leaddraft

// How a lead and its correspondence fold into the draft's Input.
//
// The Input, and everything that reads it, is persondraft's. This file only
// says which lead field answers which of its questions.

import (
	"strings"
	"time"

	"github.com/margince/margince/backend/internal/compose/persondraft"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/convstate"
	"github.com/margince/margince/backend/internal/shared/kernel/draftfloor"
)

// FromLead folds the lead and its correspondence into the draft's input.
//
// Deal, Project and Claims stay empty, and that is the honest answer rather
// than an unfinished one: all three hang off a contact, and a lead is the
// record that exists BEFORE a contact does. Reaching for a nearby account's
// deal would put a number in front of a prospect that nobody has agreed with
// them.
func FromLead(
	lead crmcontracts.Lead,
	activities []crmcontracts.Activity,
	intent string,
	envelope draftfloor.Envelope,
) persondraft.Input {
	return persondraft.Input{
		Intent:    strings.TrimSpace(intent),
		Envelope:  envelope,
		Recipient: recipientOf(lead, activities),
		Recent:    persondraft.FoldRecent(activities),
	}
}

// recipientOf is who the draft is addressed to, off the lead's own columns.
func recipientOf(lead crmcontracts.Lead, activities []crmcontracts.Activity) persondraft.RecipientIn {
	full := deref(lead.FullName)
	first, last := splitName(full)
	out := persondraft.RecipientIn{
		ID:        lead.Id.String(),
		Name:      full,
		FirstName: first,
		LastName:  last,
		// The company as the lead WROTE it. `company_name` is free text on the
		// lead rather than an organization link — the contract says so where
		// the column is declared — so this is what they called themselves and
		// never a record we hold about them.
		Employer: deref(lead.CompanyName),
	}
	if lead.Email != nil {
		out.Email = string(*lead.Email)
	}
	lastIn, lastOut := lastEachWay(activities)
	if !lastIn.IsZero() {
		out.LastInbound = lastIn.Format(time.RFC3339)
	}
	if !lastOut.IsZero() {
		out.LastOutbound = lastOut.Format(time.RFC3339)
	}
	return out
}

// splitName is the greeting name and the formal name, off the one column a
// lead has.
//
// A lead carries `full_name` and nothing else — no stored first or last name,
// which is what persondraft prefers when a contact has them. So the split IS
// the answer here rather than a fallback, and a one-word name is a name: it
// becomes the greeting name with no surname, not an empty greeting.
func splitName(full string) (first, last string) {
	fields := strings.Fields(full)
	if len(fields) == 0 {
		return "", ""
	}
	if len(fields) == 1 {
		return fields[0], ""
	}
	return fields[0], fields[len(fields)-1]
}

// lastEachWay is when each side last wrote, zero for a direction that never
// happened.
//
// Derived from the correspondence rather than read off a column, because a
// lead has no last_inbound_at / last_outbound_at pair — it carries one
// `last_activity_at`, which cannot say which way the last message went. Which
// direction went last is the whole question a follow-up answers, so it is
// derived here rather than collapsed into one stamp.
//
// The activities arrive newest first, so the first of each direction is the
// latest of it. An activity with no direction is not a side speaking.
func lastEachWay(activities []crmcontracts.Activity) (inbound, outbound time.Time) {
	for _, activity := range activities {
		if activity.Direction == nil {
			continue
		}
		switch *activity.Direction {
		case crmcontracts.ActivityDirectionInbound:
			if inbound.IsZero() {
				inbound = activity.OccurredAt.UTC()
			}
		case crmcontracts.ActivityDirectionOutbound:
			if outbound.IsZero() {
				outbound = activity.OccurredAt.UTC()
			}
		}
		if !inbound.IsZero() && !outbound.IsZero() {
			return inbound, outbound
		}
	}
	return inbound, outbound
}

// ConversationState is where this correspondence stands, for the envelope.
//
// It reads the same lastEachWay the recipient's two stamps are formatted from,
// so the envelope's account of the conversation and the draft's are one
// derivation rather than two.
//
// Held by: TestTheConversationStateReadsTheSameTwoInstants
// (backend/internal/compose/leaddraft/fold_test.go)
func ConversationState(activities []crmcontracts.Activity, now time.Time) convstate.State {
	inbound, outbound := lastEachWay(activities)
	return convstate.Classify(now, inbound, outbound)
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
