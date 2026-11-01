// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package leaddraft

// How a lead and its correspondence fold into the draft's Input.
//
// The Input, and everything that reads it, is contactdraft's. This file only
// says which lead field answers which of its questions.

import (
	"strings"
	"time"

	"github.com/margince/margince/backend/internal/compose/contactdraft"
	"github.com/margince/margince/backend/internal/compose/draftcore"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/convstate"
	"github.com/margince/margince/backend/internal/shared/kernel/draftfloor"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
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
	req Request,
	envelope draftfloor.Envelope,
) contactdraft.Input {
	return contactdraft.Input{
		Intent:    strings.TrimSpace(req.Intent),
		RewriteOf: strings.TrimSpace(req.RewriteOf),
		Envelope:  envelope,
		Recipient: recipientOf(lead, activities),
		Recent:    contactdraft.FoldRecent(activities, envelope.At(), envelope.Zone()),
	}
}

// recipientOf is who the draft is addressed to, off the lead's own columns.
func recipientOf(lead crmcontracts.Lead, activities []crmcontracts.Activity) contactdraft.RecipientIn {
	full := deref(lead.FullName)
	first, last := greetingNames(full)
	out := contactdraft.RecipientIn{
		ID:        lead.Id.String(),
		Kind:      datasource.EntityLead,
		Name:      full,
		FirstName: first,
		LastName:  last,
		// The company as the lead WROTE it. `company_name` is free text on the
		// lead rather than a company link — the contract says so where
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

// greetingNames is the greeting name and the formal name, off the one column a
// lead has. A one-word name is the greeting name with no surname. A name whose
// order the string does not state is greeted by its whole display name,
// because its first word may be the family name.
func greetingNames(full string) (first, last string) {
	if first, last, ok := contacts.SplitStoredName(full); ok {
		return first, last
	}
	return strings.Join(strings.Fields(full), " "), ""
}

// wroteItOurSide answers who held the pen.
//
// The row's voice when it names a side. A colleague writing from one of our own
// domains arrives inbound, and direction alone calls that mail theirs.
//
// A machine and an unresolved sender name no side, so those read the transport.
func wroteItOurSide(activity crmcontracts.Activity) (ours, known bool) {
	// Direction first, because having one is what makes a row correspondence. A
	// note or a task is written down rather than sent, whoever authored it, so
	// no side spoke and neither stamp moves.
	if activity.Direction == nil {
		return false, false
	}
	if voice := activity.Voice; voice != nil && voice.Author != nil {
		switch voice.Author.Kind {
		case crmcontracts.ActivityAuthorKindColleague:
			return true, true
		case crmcontracts.ActivityAuthorKindContactRecord:
			return false, true
		}
	}
	switch *activity.Direction {
	case crmcontracts.ActivityDirectionOutbound:
		return true, true
	case crmcontracts.ActivityDirectionInbound:
		return false, true
	}
	return false, false
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
		ours, known := wroteItOurSide(activity)
		switch {
		case !known:
			continue
		case ours && outbound.IsZero():
			outbound = activity.OccurredAt.UTC()
		case !ours && inbound.IsZero():
			inbound = activity.OccurredAt.UTC()
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
// derivation rather than two. A logged note or meeting counts as contact by
// the contact draft's rule, because the lead's draft is folded by it.
//
// Held by: TestTheConversationStateReadsTheSameTwoInstants
// (backend/internal/compose/leaddraft/fold_test.go)
func ConversationState(activities []crmcontracts.Activity, now time.Time, zone *time.Location) convstate.State {
	inbound, outbound := lastEachWay(activities)
	return draftcore.ClassifyWithLogged(now, zone, inbound, outbound, activities)
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
