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

type attentionLeadResponses struct {
	store *people.Store
	// teammates answers whether a lead's owner shares a live team with the
	// reader. Required rather than optional: a lead is workspace-readable, so
	// without this the team scope cannot be narrowed at all and would show the
	// whole organization's inbound under a page headed `team`.
	teammates attention.Teammates
}

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
	//
	// It is asked WITHOUT the lead grant, deliberately. Whether this
	// installation measures first response is a property of the installation,
	// not of any lead, so a reader who may not read leads still gets the honest
	// answer "nothing measures this" rather than "a source was withheld from
	// you". The grant is still required for the rows themselves, one call down.
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
		// NOT "no narrowing", and this is the trap the deal-bearing lanes do
		// not have. A lead is an IDENTITY record, so its read predicate is TRUE
		// for every seat holding the grant (auth.identityTables): the store
		// hands back every lead in the workspace, and a page headed `team`
		// would be the whole organization's inbound.
		//
		// The store's own dial cannot express it either — OwnerTeamID names ONE
		// team and a reader may be in several — so the narrowing happens on the
		// rows below, through the membership question the product already
		// answers. An unbounded reader keeps the workspace, which is what `all`
		// means.
	}

	rows, _, err := l.store.ListLeads(ctx, in)
	if err != nil {
		return nil, false, err
	}
	// The team narrowing, applied to the rows because no query dial fits it.
	// Bounded by the page the store already cut, so this asks the membership
	// question at most `limit` times rather than once per lead in the
	// workspace.
	keep, err := l.narrowToTeam(ctx, scope, rows)
	if err != nil {
		return nil, false, err
	}
	owed := make([]attention.OwedLead, 0, len(keep))
	for _, row := range keep {
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

// narrowToTeam keeps the leads a TEAM-scoped reader may call theirs.
//
// Only the wide scope needs it: mine, unassigned and a named owner were all
// narrowed in the query, and an unbounded reader is entitled to the workspace.
// The question is the one identity already answers for the named-owner path, so
// the two agree by construction rather than by two copies of a rule.
//
// It fails CLOSED. Without the seam bound there is no way to tell a teammate's
// lead from a stranger's, and answering with every lead would be the widening
// this exists to prevent.
func (l attentionLeadResponses) narrowToTeam(
	ctx context.Context, scope attention.TaskScope, rows []crmcontracts.Lead,
) ([]crmcontracts.Lead, error) {
	if scope != attention.TasksVisible {
		return rows, nil
	}
	actor, ok := principal.Actor(ctx)
	if !ok {
		return nil, nil
	}
	// An unbounded seat reads every row by policy, so `all` is the honest
	// answer to a wide ask from one.
	if actor.Permissions.RowScope == principal.RowScopeAll {
		return rows, nil
	}
	if l.teammates == nil {
		return nil, nil
	}
	// One answer per owner, not per row: an inbox of forty leads routed to the
	// same rep asks once.
	shares := map[ids.UUID]bool{}
	kept := make([]crmcontracts.Lead, 0, len(rows))
	for _, row := range rows {
		owner := ownerOfLead(row)
		// A lead nobody owns is nobody's teammate's either, and it stays: an
		// unclaimed inbound is everybody's to pick up, which is what the
		// unassigned scope exists for and what a team page should still show.
		if owner.IsZero() {
			kept = append(kept, row)
			continue
		}
		if owner == actor.UserID {
			kept = append(kept, row)
			continue
		}
		mine, seen := shares[owner]
		if !seen {
			answer, err := l.teammates.SharesLiveTeamWithCaller(ctx, owner)
			if err != nil {
				return nil, err
			}
			shares[owner] = answer
			mine = answer
		}
		if mine {
			kept = append(kept, row)
		}
	}
	return kept, nil
}
