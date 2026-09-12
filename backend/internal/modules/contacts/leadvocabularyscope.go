// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// What a lead vocabulary row's count may count.
//
// Its own file because it is the one place both vocabularies and the
// discovered-source roll-up go through, and because the question it settles is
// not the source list's: a `lead_source` row is configuration, and the number
// beside it is lead data.

import (
	"context"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// visibleLeadScope renders the lead row-scope clause for the vocabulary
// rows' lead_count — a count of the leads the CALLER MAY SEE, embedded in
// the representation the vocabulary writers return. It exists as its own
// spelling (rather than sharing scopeOrAllRows) so the write-authority gate
// waives exactly this probe: a future write path reaching the shared helper
// still fails the gate and states its own reason.
//
// BOTH HALVES OF RBAC, and the object half is asked HERE rather than at each
// vocabulary entry point because this is the one helper every lead count goes
// through — the two column lists and the discovered-sources roll-up alike.
//
// The row half alone was not enough, and the gap was not theoretical: these
// surfaces are gated on `custom_field`, so a role granted the vocabulary and
// not leads was answered a count of every lead its row scope reached — which,
// for an unbounded seat, is all of them — and the discovered-sources list hands
// back the SOURCE VALUES themselves, which are lead data and not a number. No
// seeded role has custom_field without lead, so nothing a default installation
// does changes; what changes is that a custom role cannot be built that reads
// leads through the settings screen.
func visibleLeadScope(ctx context.Context, arg func(any) int) (string, error) {
	if err := auth.Require(ctx, "lead", principal.ActionRead); err != nil {
		return "", err
	}
	clause, err := auth.ScopeClauseFor(ctx, "lead", "", arg)
	if err != nil || clause != "" {
		return clause, err
	}
	return scopeAllRows, nil
}
