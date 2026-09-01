// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// WHOSE day the queue answers.
//
// The default is the reader's own work, and that is the point rather than a
// convenience: an admin account can read every deal in the installation, so a
// queue that showed everything readable would hand a rep several hundred rows
// belonging to colleagues and call it their day. "Mine" is the honest default
// for a surface whose whole claim is "what should I do next".
//
// A wider scope is OFFERED only where the reader's row scope already reaches
// that far, and asking for one they do not hold is refused rather than quietly
// narrowed. Silently narrowing would answer a question about the team with
// facts about one person, and the reader would have no way to tell.

import (
	"context"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// The scopes a reader may ask the queue for.
const (
	scopeMine = "mine"
	// scopeUnassigned is the work nobody answers for.
	//
	// Its own scope because "mine" stopped carrying it. Unowned work is real and
	// somebody has to pick it up, but it arrives in a queue a reader opens on
	// purpose rather than in the one that claims to be theirs.
	scopeUnassigned = "unassigned"
	scopeTeam       = "team"
	scopeAll        = "all"
)

// scopeOptionsFor answers which scopes this reader may ask for, narrowest
// first.
//
// It reads the row scope resolved at authentication rather than probing rows:
// the tier is a fact about the principal, and asking the database whether a
// colleague's deal is visible would be re-deriving per row what the policy
// already decided once (P11).
// Unassigned is offered to EVERY reader, at every tier. It is the other half of
// making "mine" exact: work that belongs to nobody stopped appearing in each
// reader's own queue, so it has to be reachable from every one of them or the
// change would have hidden it. Nothing in it is a colleague's — that is what
// unassigned means — so no tier gates it.
func scopeOptionsFor(ctx context.Context) []string {
	options := []string{scopeMine, scopeUnassigned}
	actor, ok := principal.Actor(ctx)
	if !ok {
		return options
	}
	switch actor.Permissions.RowScope {
	case principal.RowScopeAll:
		return append(options, scopeTeam, scopeAll)
	case principal.RowScopeTeam:
		return append(options, scopeTeam)
	default:
		return options
	}
}

// resolveScope answers which scope this read runs at, or refuses.
//
// An empty ask means "mine", which is the default every reader holds. An ask
// the reader's row scope does not reach is ErrPermissionDenied — a 403 — and
// never a quiet narrowing to what they can see.
func resolveScope(ctx context.Context, asked string) (string, error) {
	if asked == "" {
		return scopeMine, nil
	}
	for _, allowed := range scopeOptionsFor(ctx) {
		if asked == allowed {
			return asked, nil
		}
	}
	return "", apperrors.ErrPermissionDenied
}

// resolveOwner answers whose queue this read is FOR, or refuses.
//
// A manager looking at a team exception needs to open the rep's own day — the
// exception says whose it is, and the next question is always "show me their
// queue". Without this they can only widen to `team`, which answers a question
// about everybody when they asked about one person.
//
// The rule is the one resolveScope keeps: an ask the reader's row scope does
// not reach is a 403, never a quiet narrowing. Narrowing here would be worse
// than elsewhere, because the answer would look like the named rep's day while
// being the reader's own.
//
// Reading somebody's queue is not reading their inbox. The per-user sources —
// a mailbox, a notice, a promise — stay bound to the acting user inside the
// modules that own them, exactly as they do under `team` and `all`, so what
// this reaches is the shared record-bearing work assigned to that person. The
// contract says so where the parameter is declared.
func (s *Service) resolveOwner(ctx context.Context, asked ids.UUID) (ids.UUID, error) {
	if asked.IsZero() {
		return ids.UUID{}, nil
	}
	owner := asked
	actor, ok := principal.Actor(ctx)
	if !ok {
		return ids.UUID{}, apperrors.ErrPermissionDenied
	}
	// Their own id needs no wider tier: "mine, spelled out" is the same
	// question the default answers.
	if owner == actor.UserID {
		return owner, nil
	}
	// An unbounded reader reaches every row, so for them the tier IS the whole
	// test and naming anyone is answerable.
	//
	// A TEAM-scoped reader is different, and the difference is not academic. Row
	// scope narrows the deal-bearing rows correctly, but the task lane's gate is
	// a link walk: auth.ActivityDiscoverClause coalesces the empty link set to
	// TRUE, so a task carrying no record link is discoverable by anyone. Naming
	// an out-of-team colleague would return exactly those rows, under a page
	// headed with that colleague's name. So membership is asked here, where the
	// question is "may I open this person's queue" rather than "may I read this
	// row".
	switch actor.Permissions.RowScope {
	case principal.RowScopeAll:
		return owner, nil
	case principal.RowScopeTeam:
		if s.teammates == nil {
			return ids.UUID{}, apperrors.ErrPermissionDenied
		}
		shares, err := s.teammates.SharesLiveTeamWithCaller(ctx, owner)
		if err != nil {
			return ids.UUID{}, err
		}
		if !shares {
			return ids.UUID{}, apperrors.ErrPermissionDenied
		}
		return owner, nil
	default:
		return ids.UUID{}, apperrors.ErrPermissionDenied
	}
}

// mineOnly reports whether this read keeps only the reader's own work.
//
// WHAT A WIDER SCOPE CAN AND CANNOT DO, because the difference is not obvious
// and a reader must not be misled by the word on the response.
//
// The record-bearing sources — tasks, deals at risk, meetings, duplicate pairs
// — widen: they are read under the caller's row scope, so `team` and `all`
// return what that tier reaches and `mine` narrows below it.
//
// The intrinsically PER-USER sources do not, and cannot: a notice is addressed
// to one person, a mailbox belongs to one person, a promise was made by one
// person, an approved action failed for the person who approved it. Those reads
// are bound to the acting user inside the modules that own them, so asking for
// `all` does not reach a colleague's notices — nor should it, since the request
// is for a wider view of shared work rather than a licence to read another
// rep's inbox.
//
// So `all` means "every shared record I may see, plus my own personal queue",
// which is the only honest reading available without a per-source authority
// model the product does not have.
func mineOnly(scope string) bool { return scope == scopeMine }

// ownedByReader reports whether an item is the reader's own work.
//
// Only the DEAL-bearing sources can be judged here, because only they carry an
// owner on the wire. Everything else on this queue is already per-reader by
// construction — a task the lane read for this viewer, an approval they may
// decide, their own mailbox, their own promises — so a row with no owner to
// check is the reader's by the lane that produced it, and dropping it would
// hide their own work from them.
func ownedByReader(item crmcontracts.WorklistItem, reader principal.Principal) bool {
	if item.Deal == nil || item.Deal.OwnerId == nil {
		return true
	}
	return ids.UUID(*item.Deal.OwnerId) == reader.UserID
}

// waitingIsMine reports whether an unanswered message belongs to the reader's
// own work.
//
// A message has no owner column, so the question is answered by the RECORD it
// is filed under: a thread about a colleague's deal is that colleague's to
// answer.
//
// A message filed under nothing names nobody, and stays — an unowned customer
// writing in is everybody's, and dropping it would leave nobody looking at it.
//
// That reading is the one this queue is otherwise moving away from, and it
// survives here for a reason worth stating: ownedDeals is built from the
// at-risk lane alone, so a deal the reader owns which is perfectly healthy is
// absent from it and reads as unowned. Judging a wait exactly against that set
// would drop the reader's own waiting customer whenever their deal was doing
// well. Making it exact needs the owner on the row itself, which is the read
// the waiting query does not yet carry.
func waitingIsMine(waiting WaitingCustomer, ownedDeals map[ids.UUID]bool) bool {
	if waiting.DealID.IsZero() {
		return true
	}
	return ownedDeals[waiting.DealID]
}

// keepReadersOwn drops the rows belonging to somebody else.
//
// It runs over the CANDIDATES, before the page is cut, so a reader asking for
// twenty-five of their own rows gets twenty-five where they exist. Narrowing
// after the cut returned a short page while the reader's own work sat just
// past it.
//
// It fails CLOSED. A call with no human behind it has no "own work" to answer
// for, and a queue that handed such a caller every row it had read would be
// widening a scope named `mine` — the opposite of what it says. An agent that
// needs the day reads it under a scope it can actually hold.
//
// The narrowing itself is a display cut, not the security boundary: the lanes
// are already row-scoped, so this changes what a wide-scoped reader is SHOWN
// by default rather than what any store was asked. Pushing the filter into
// each producer is the better shape and needs each to take an owner.
func keepReadersOwn(ctx context.Context, rows []ranked) []ranked {
	actor, ok := principal.Actor(ctx)
	if !ok || actor.UserID.IsZero() {
		return nil
	}
	kept := make([]ranked, 0, len(rows))
	for _, row := range rows {
		if ownedByReader(row.item, actor) {
			kept = append(kept, row)
		}
	}
	return kept
}

// keepOwnedBy keeps only the rows POSITIVELY attributable to one named person.
//
// The opposite default from keepReadersOwn, and the difference is the whole
// rule. That one keeps a row it cannot judge because the lane already bound it
// to the acting reader: a notice addressed to them, their own mailbox, their
// own promise. Under a named owner those same lanes are still bound to the
// ACTING reader, so keeping what cannot be judged hands a manager their own
// notices and meetings with somebody else's name on the page.
//
// Nothing crosses a scope boundary either way — every row here was already
// readable. What is at stake is whether the answer is true: a page headed
// "Lena's day" that is mostly the reader's own day is a worse failure than an
// empty one, because nothing on it says so.
//
// So a row survives when it names this owner's deal, or when it came from the
// task lane, which narrowed to them in its own query. Everything else is the
// reader's, and this is not their page.
//
// That is also the honest limit of the scope, and the contract states it where
// the parameter is declared: a manager opening a rep's queue sees the rep's
// deals and tasks, never their mailbox.
func keepOwnedBy(rows []ranked, owner ids.UUID) []ranked {
	kept := make([]ranked, 0, len(rows))
	for _, row := range rows {
		if row.item.Source == sourceTask {
			kept = append(kept, row)
			continue
		}
		// The lead lane narrowed to this owner in its own query, exactly as the
		// task lane did, so its rows are already theirs. Judging them by the
		// deal arm below would drop every one — a lead row carries a lead
		// subject and no deal — and a manager opening a rep's day would find
		// the lane silently missing rather than empty.
		if row.item.Source == sourceLeadResponse {
			kept = append(kept, row)
			continue
		}
		if row.item.Deal != nil && row.item.Deal.OwnerId != nil &&
			ids.UUID(*row.item.Deal.OwnerId) == owner {
			kept = append(kept, row)
		}
	}
	return kept
}

// keepUnowned keeps the rows that answer to nobody.
//
// The counterpart to keepReadersOwn, over the same evidence: a deal-bearing row
// with an owner belongs to that person and not to this queue. A row with no
// owner to check stays, which is what the scope is for.
func keepUnowned(rows []ranked) []ranked {
	kept := make([]ranked, 0, len(rows))
	for _, row := range rows {
		if row.item.Deal == nil || row.item.Deal.OwnerId == nil {
			kept = append(kept, row)
		}
	}
	return kept
}

// scopeOptions puts the resolver's answer on the wire.
func scopeOptions(options []string) []crmcontracts.WorklistScopeOptions {
	out := make([]crmcontracts.WorklistScopeOptions, 0, len(options))
	for _, option := range options {
		out = append(out, crmcontracts.WorklistScopeOptions(option))
	}
	return out
}
