// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

// Who owns a record the caller was handed, said in a way a reader can act on.
//
// A company is readable across the whole workspace by design: `company`
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

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// SeatNamer resolves user ids to display names. An owner is a SEAT rather than
// a record, so it is named through identity's read and not the contacts store's
// — and this module may not import identity, so compose injects it.
//
// Batch, because a page of rows shares owners: twenty companies owned by three
// reps is one call, not twenty.
type SeatNamer func(ctx context.Context, seats []ids.UUID) (map[ids.UUID]string, error)

// WithSeatNamer injects the SeatNamer that search_records, list_records,
// read_record and query_workspace resolve owners through, so the four tools give
// one answer to whose a record is.
func WithSeatNamer(name SeatNamer) RegistryOption {
	return func(r *Registry) { r.seats = name }
}

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
	// IsYou answers for the HUMAN the call is made as — the caller themselves,
	// or the contact whose authority an agent is acting under. An assistant
	// reading a page on your behalf must see your own accounts as yours; an
	// agent that marked them as somebody else's would send you to check with
	// yourself and bury the colleague-owned rows in the same noise.
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
// not a zero UUID, which would read as an owner nobody can name. Fields that
// will not decode answer the error, so the caller says ownership went unread.
func ownerIDOf(fields json.RawMessage) (ids.UUID, bool, error) {
	if len(fields) == 0 {
		return ids.UUID{}, false, nil
	}
	var envelope struct {
		OwnerID *ids.UUID `json:"owner_id"`
	}
	if err := json.Unmarshal(fields, &envelope); err != nil {
		return ids.UUID{}, false, err
	}
	if envelope.OwnerID == nil || *envelope.OwnerID == (ids.UUID{}) {
		return ids.UUID{}, false, nil
	}
	return *envelope.OwnerID, true, nil
}

// callerSeat is the human a call is made AS: the caller themselves, or the
// contact whose authority an agent or connector is acting under.
//
// This is identity's actingHuman rule, and it has to be the same rule. A
// passport carries its represented human in UserID and OnBehalfOf both, so
// reading only a human principal's seat would leave every assistant-driven
// read marking its own operator's accounts as a colleague's — the exact
// inversion this disclosure exists to prevent.
//
// A system principal has no human behind it and answers not-ok, rather than a
// zero UUID: a zero seat compared against a zero owner would mark an unowned
// record as the caller's own.
func callerSeat(ctx context.Context) (ids.UUID, bool) {
	p, ok := principal.Actor(ctx)
	if !ok {
		return ids.UUID{}, false
	}
	seat := p.UserID
	if seat.IsZero() {
		seat = p.OnBehalfOf
	}
	if seat.IsZero() {
		return ids.UUID{}, false
	}
	return seat, true
}

// CodeOwnerNamesUnavailable says the seat lookup failed, so the rows carry
// owner ids and is_you markers but no names.
//
// It exists because the alternative reading is WRONG in a way that matters: an
// unnamed owner otherwise looks exactly like a departed one, and a caller
// cannot tell a database timeout from a colleague who left. Only one of those
// means "ask around before you contact this account".
const CodeOwnerNamesUnavailable = "owner_names_unavailable"

// attachOwners names the owner on every query row that has one, in ONE lookup.
//
// It runs after the rows are assembled rather than during, so the query is
// per PAGE and not per row. A naming failure is not fatal and does not fail
// the read: the rows are already admitted and already the caller's to see, and
// answering them unnamed is strictly better than answering nothing. The id and
// is_you still ride out, so the disclosure degrades rather than disappearing —
// and the caller is TOLD it degraded, which is what keeps "no name" honest.
func attachOwners(ctx context.Context, name SeatNamer, rows []QueryWorkspaceRow) ([]QueryWorkspaceRow, *QueryNote) {
	fields := make([]json.RawMessage, len(rows))
	for i, row := range rows {
		fields[i] = row.Record.Fields
	}
	owners, unnamed := ownersOf(ctx, name, fields)
	for i := range rows {
		rows[i].Owner = owners[i]
	}
	if !unnamed {
		return rows, nil
	}
	return rows, &QueryNote{Code: CodeOwnerNamesUnavailable, Detail: ownerNamesUnavailableDetail}
}

const ownerNamesUnavailableDetail = "the owner of one or more of these records could not be named; " +
	"each row whose owner could be read still says who owns it by id and whether it is yours, " +
	"but a missing name or owner here does not mean the record is unowned or the owner has left"

// recordWithOwner is a record served to a reader together with whose it is — the
// row search_records and list_records page through and read_record answers.
// A record found by name is as likely to be a colleague's as one found by a
// plan, so it carries the same owner a query row does.
type recordWithOwner struct {
	wireRecord
	// Owner is absent for a record that carries no owner; see RecordOwner.
	Owner *RecordOwner `json:"owner,omitempty"`
}

// withOwners names the owners of a page of served records in one lookup. The
// envelope carries the degradation warning, because these results have no
// notes of their own.
func withOwners(ctx context.Context, name SeatNamer, records []wireRecord) []recordWithOwner {
	fields := make([]json.RawMessage, len(records))
	for i, rec := range records {
		fields[i] = rec.Fields
	}
	owners, unnamed := ownersOf(ctx, name, fields)
	if unnamed {
		noteWarning(ctx, CodeOwnerNamesUnavailable, ownerNamesUnavailableDetail)
	}
	out := make([]recordWithOwner, len(records))
	for i, rec := range records {
		out[i] = recordWithOwner{wireRecord: rec, Owner: owners[i]}
	}
	return out
}

// ownersOf resolves the owner of each record's fields, nil where a record has
// none. unnamed reports a failed seat lookup or an owner that would not decode,
// never a seat that simply did not resolve: an owner who left is an ordinary
// answer.
func ownersOf(ctx context.Context, name SeatNamer, fields []json.RawMessage) (owners []*RecordOwner, unnamed bool) {
	owners = make([]*RecordOwner, len(fields))
	ownerIDs := make(map[int]ids.UUID, len(fields))
	seats := make([]ids.UUID, 0, len(fields))
	seen := make(map[ids.UUID]bool, len(fields))
	for i, f := range fields {
		owner, ok, err := ownerIDOf(f)
		unnamed = unnamed || err != nil
		if !ok {
			continue
		}
		ownerIDs[i] = owner
		if !seen[owner] {
			seen[owner] = true
			seats = append(seats, owner)
		}
	}
	if len(ownerIDs) == 0 {
		return owners, unnamed
	}
	named := map[ids.UUID]string{}
	// A nil namer is not a failure and is not reported: the installation cannot
	// name seats at all, which is a standing property, and noting it per call
	// would put the same warning on every answer it ever gives.
	if name != nil {
		resolved, err := name(ctx, seats)
		unnamed = unnamed || err != nil
		named = resolved
	}
	me, haveSeat := callerSeat(ctx)
	for i, owner := range ownerIDs {
		owners[i] = &RecordOwner{ID: owner, Name: named[owner], IsYou: haveSeat && owner == me}
	}
	return owners, unnamed
}
