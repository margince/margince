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
// Team and all are served by the lanes' own row scope, which already stops at
// the reader's tier: asking for `all` cannot widen what the database returns,
// it only stops this surface narrowing it further.
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
