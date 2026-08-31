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
	scopeTeam = "team"
	scopeAll  = "all"
)

// scopeOptionsFor answers which scopes this reader may ask for, narrowest
// first.
//
// It reads the row scope resolved at authentication rather than probing rows:
// the tier is a fact about the principal, and asking the database whether a
// colleague's deal is visible would be re-deriving per row what the policy
// already decided once (P11).
func scopeOptionsFor(ctx context.Context) []string {
	options := []string{scopeMine}
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
// answer. A message filed under nothing names nobody, and stays — an unowned
// customer writing in is everybody's, and dropping it would leave nobody
// looking at it.
func waitingIsMine(waiting WaitingCustomer, ownedDeals map[ids.UUID]bool) bool {
	if waiting.DealID.IsZero() {
		return true
	}
	return ownedDeals[waiting.DealID]
}
