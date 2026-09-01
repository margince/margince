// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The lead lane's seam over the people store's own work queue.
//
// A binding rather than an implementation, and deliberately so: the first-
// response clock, its target, and the state a lead is in are the people
// module's to derive — sla_state and sla_deadline_at come back on every lead
// read. Re-deriving any of it here would be a second opinion about when a reply
// is late, and the lead screen and the Worklist would eventually disagree in
// front of a rep.

import (
	"context"
	"time"

	"github.com/margince/margince/backend/internal/compose/attention"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/people"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

type attentionLeadResponses struct{ store *people.Store }

// Owed reads the leads still waiting for their first reply.
//
// The ordering is the store's: ListLeads with no sort routes to the work queue,
// which ranks breached above at-risk above within-target and score below that
// (ADR-0119/A170). So the rows arrive in the order the product already says
// they should be worked, and this lane keeps it.
func (l attentionLeadResponses) Owed(
	ctx context.Context, scope attention.TaskScope, owner ids.UUID, limit int,
) ([]attention.OwedLead, bool, error) {
	// Asked FIRST, and the answer is not a filter. With no target set no lead
	// owes a reply at a stated time, so the lane is absent rather than empty —
	// the difference between "nothing is late" and "nothing measures late".
	tracked, err := l.store.FirstResponseTracked(ctx)
	if err != nil {
		return nil, false, err
	}
	if !tracked {
		return nil, false, nil
	}

	in := people.ListLeadsInput{Limit: &limit}
	// Narrowed in the QUERY, the way the task lane is: filtering afterwards
	// would let a colleague's leads fill the bound and hide the reader's own
	// overdue one behind a cut that had already happened.
	switch scope {
	case attention.TasksMine:
		actor, ok := principal.Actor(ctx)
		if !ok || actor.UserID.IsZero() {
			// No human, no "own work" to answer for.
			return nil, tracked, nil
		}
		own := ids.From[ids.UserKind](actor.UserID)
		in.OwnerID = &own
	case attention.TasksUnassigned:
		unassigned := true
		in.Unassigned = &unassigned
	case attention.TasksOwnedBy:
		if owner.IsZero() {
			return nil, tracked, nil
		}
		named := ids.From[ids.UserKind](owner)
		in.OwnerID = &named
	case attention.TasksVisible:
		// No narrowing of its own: the store's row-scope gate decides.
	}

	rows, _, err := l.store.ListLeads(ctx, in)
	if err != nil {
		return nil, false, err
	}
	owed := make([]attention.OwedLead, 0, len(rows))
	for _, row := range rows {
		// A lead that has been answered, or that never owed a reply, is not
		// this lane's work. The store ranks those last rather than dropping
		// them, because the same queue answers other questions.
		if row.SlaState == nil {
			continue
		}
		owed = append(owed, attention.OwedLead{
			ID:         ids.UUID(row.Id),
			Name:       leadDisplayName(row),
			OwnerID:    ownerOfLead(row),
			DeadlineAt: deadlineOfLead(row),
			State:      string(*row.SlaState),
		})
	}
	return owed, tracked, nil
}

// leadDisplayName is what the row calls the lead, or NOTHING.
//
// The empty answer is the honest one: the product ships three languages, so a
// placeholder composed here would reach a German reader in English. The client
// writes the stand-in in the reader's own words, as it does for a meeting with
// no title.
func leadDisplayName(row crmcontracts.Lead) string {
	if row.FullName != nil && *row.FullName != "" {
		return *row.FullName
	}
	if row.CompanyName != nil {
		return *row.CompanyName
	}
	return ""
}

func ownerOfLead(row crmcontracts.Lead) ids.UUID {
	if row.OwnerId == nil {
		return ids.UUID{}
	}
	return ids.UUID(*row.OwnerId)
}

func deadlineOfLead(row crmcontracts.Lead) time.Time {
	if row.SlaDeadlineAt == nil {
		return time.Time{}
	}
	return *row.SlaDeadlineAt
}
