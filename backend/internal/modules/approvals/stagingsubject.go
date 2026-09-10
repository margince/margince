// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package approvals

import (
	"fmt"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// Whose proposal a staged row is, for the statements that match "the same
// proposal" as one another.
//
// decidable states the boundary for READING and DECIDING: two shapes belong to
// one member rather than to the shared inbox — a self-only kind, and a staged
// create against a table whose rows belong to one human each. The staging
// engine's own matching statements did not know it. They matched on kind,
// target and either the logical identity or the diff hash, so one member's
// pending row could be joined as another's, expired as superseded by another's,
// and one member's decline could refuse another member's proposal — for exactly
// the kinds whose own gate says a row is one person's business.
//
// The engine's answer to this used to be that a caller builds a
// collision-proof identity. No caller states that property and none tests it,
// and #3410 shows how subtle it is: a bare full name is not a card's own
// addressing, it is every card sharing that name. An unstated, untested
// property held by every future caller is a hope rather than a boundary.

// subjectScopedShape reports whether a staging of this shape belongs to ONE
// member, and is the same question decidable asks of a stored row.
//
// Asked of the SHAPE rather than of a kind list at each statement, so a kind
// added to selfOnlyKinds, or a table enrolled in probeOwnerOnly, inherits the
// narrowing instead of quietly staying shared.
//
// It must answer for EVERY kind decidable narrows to a seat, not only the
// self-only ones. The two run on opposite sides of one row — this when it is
// written, decidable when it is read — so a kind narrowed on the read side
// alone still MATCHES across members on the write side, and the mismatch has
// three shapes, all silent: one seat's pending proposal is joined and handed
// back to another, a second seat's differing payload supersedes the first's,
// and one seat's rejection suppresses the other's. Each time, the row that
// survives names a seat the reader is not, so it is withheld from the very
// person it was staged for.
func subjectScopedShape(in StageInput) bool {
	if selfOnlyKinds[in.Kind] || decidedByTheSeatStagedFor[in.Kind] {
		return true
	}
	var targetType *string
	if in.TargetType != "" {
		targetType = &in.TargetType
	}
	return stagedForStagerOnly(targetType, in.TargetID != ids.Nil)
}

// subjectScope is the clause narrowing a same-proposal match to the member this
// staging is for, and the empty string for a shared shape — where matching
// across members is the point, and narrowing would split one team's proposal
// into one row per person.
//
// It APPENDS the subject to args and derives the placeholder from the position
// it landed in, because a hand-typed $N is a number nothing checks: the column
// list, the placeholders and the arguments have to agree and only counting them
// says so.
//
// IS NOT DISTINCT FROM, never `=`. on_behalf_of is nullable — a system pass
// proposes on nobody's behalf — and `=` against NULL is NULL, so an `=` here
// would match nothing at all for those rows: the join never joins, the
// supersede never supersedes and the decline is never remembered, each failing
// silently as a control that quietly does nothing.
func subjectScope(in StageInput, p principal.Principal, args *[]any) string {
	if !subjectScopedShape(in) {
		return ""
	}
	*args = append(*args, nullUUID(p.OnBehalfOf))
	return fmt.Sprintf(" AND on_behalf_of IS NOT DISTINCT FROM $%d", len(*args))
}
