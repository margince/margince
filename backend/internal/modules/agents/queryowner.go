// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

// Who owns a record the caller was handed, said in a way a reader can act on.
//
// A company is readable across the whole workspace by design: `organization`
// is an identity table, so the ownership arm of its row scope renders TRUE and
// a rep sees every account. auth/tableclass.go states the reason — a rep who
// cannot see that a company already belongs to another team contacts it again.
//
// That trade only pays if the answer SAYS who the account belongs to. The
// record already carries `owner_id`, so the fact was never withheld; it was
// unreadable. A bare UUID tells a human nothing and tells a model less, and a
// model reading a page of them has no way to tell an account it may approach
// from one a colleague is already working. The failure is concrete: asked who
// is nearby, an assistant recommended visiting an account whose owner had an
// unsent contract with that customer, and never mentioned the owner existed.
//
// So the id is resolved to a name and marked against the caller, which is the
// same argument HandoffProject's OwnerName already makes for a project —
// "who owns this work now" answered as a UUID restates the question.

import (
	"context"
	"encoding/json"

	"github.com/gradionhq/margince/backend/internal/shared/kernel/ids"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/principal"
)

// SeatNamer resolves user ids to display names. An owner is a SEAT rather than
// a record, so it is named through identity's read and not the people store's
// — and this module may not import identity, so compose injects it.
//
// Batch, because a page of rows shares owners: twenty companies owned by three
// reps is one call, not twenty.
type SeatNamer func(ctx context.Context, seats []ids.UUID) (map[ids.UUID]string, error)

// RecordOwner is who a served record belongs to.
//
// The three members answer three different questions and none substitutes for
// another. ID is the durable handle. Name is what makes it legible. IsYou is
// the one a reader acts on, and it is stated rather than left to be derived —
// a client comparing ID against its own seat would have to know its own seat,
// which an assistant reading a tool result does not.
type RecordOwner struct {
	ID ids.UUID `json:"id"`
	// Name is absent when the seat no longer resolves — an archived member, or
	// a row whose owner was removed. The id stays, so the answer says "owned,
	// by someone not currently a member" rather than silently reading as
	// unowned.
	Name string `json:"name,omitempty"`
	// IsYou is false for an agent principal, and that is correct rather than a
	// gap: an agent holds no seat, so no record is its own, and every record it
	// reports belongs to a human it should name.
	IsYou bool `json:"is_you"`
}

// ownerIDOf reads `owner_id` out of a hydrated record's fields.
//
// It reads the JSON rather than the typed contract struct because that is what
// hydration carries — the datasource seam hands back json.RawMessage, and the
// record type is known only as a string. Every record type that has an owner
// spells it `owner_id` in the contract, so one accessor serves all of them and
// a new owned record type is covered the moment it exists.
//
// A record with no owner, or a type that has no owner at all, answers false —
// not a zero UUID, which would read as an owner nobody can name.
func ownerIDOf(fields json.RawMessage) (ids.UUID, bool) {
	if len(fields) == 0 {
		return ids.UUID{}, false
	}
	var envelope struct {
		OwnerID *ids.UUID `json:"owner_id"`
	}
	if err := json.Unmarshal(fields, &envelope); err != nil {
		return ids.UUID{}, false
	}
	if envelope.OwnerID == nil || *envelope.OwnerID == (ids.UUID{}) {
		return ids.UUID{}, false
	}
	return *envelope.OwnerID, true
}

// callerSeat is the human seat asking, when there is one.
//
// An agent or a system principal has no seat, so nothing is "yours" to it. It
// answers not-ok rather than a zero UUID for the same reason ownerIDOf does:
// a zero seat compared against a zero owner would mark an unowned record as
// the caller's own.
func callerSeat(ctx context.Context) (ids.UUID, bool) {
	p, ok := principal.Actor(ctx)
	if !ok || p.Type != principal.PrincipalHuman {
		return ids.UUID{}, false
	}
	if p.UserID == (ids.UUID{}) {
		return ids.UUID{}, false
	}
	return p.UserID, true
}

// attachOwners names the owner on every row that has one, in ONE lookup.
//
// It runs after the rows are assembled rather than during, so the query is
// per PAGE and not per row. A naming failure is not fatal and does not fail
// the read: the rows are already admitted and already the caller's to see, and
// answering them unnamed is strictly better than answering nothing. The id and
// is_you still ride out, so the disclosure degrades rather than disappearing.
func attachOwners(ctx context.Context, name SeatNamer, rows []QueryWorkspaceRow) []QueryWorkspaceRow {
	owners := make(map[int]ids.UUID, len(rows))
	seats := make([]ids.UUID, 0, len(rows))
	seen := make(map[ids.UUID]bool, len(rows))
	for i, row := range rows {
		owner, ok := ownerIDOf(row.Record.Fields)
		if !ok {
			continue
		}
		owners[i] = owner
		if !seen[owner] {
			seen[owner] = true
			seats = append(seats, owner)
		}
	}
	if len(owners) == 0 {
		return rows
	}
	named := map[ids.UUID]string{}
	if name != nil {
		if resolved, err := name(ctx, seats); err == nil {
			named = resolved
		}
	}
	me, haveSeat := callerSeat(ctx)
	for i, owner := range owners {
		rows[i].Owner = &RecordOwner{
			ID: owner, Name: named[owner], IsYou: haveSeat && owner == me,
		}
	}
	return rows
}
