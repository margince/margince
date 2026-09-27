// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package auth

// Which teams' coaching weeks a caller may open.
//
// A team's week names each member with a verdict their lead is meant to raise
// with them. Row scope answers whose RECORDS a seat reads, which is a different
// question: a read-only auditor reaches every row and leads nobody. So the
// answer is built from two authorities that say "lead" and "oversee" outright,
// and row scope only takes part as the own-scope exclusion.

import (
	"context"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// objTeamOversight is the grant that opens every team's week to a seat that
// leads none of them. Seeded to admin and management only.
const objTeamOversight = "team_oversight"

// TeamWeekReach is how far a caller's reading of team weeks reaches.
type TeamWeekReach int

const (
	// ReachesNoTeam refuses every team's week.
	ReachesNoTeam TeamWeekReach = iota
	// ReachesTeamsLed opens the week of a team the caller is a live member
	// of. The membership half is the caller's to ask: platform cannot read
	// team_membership.
	ReachesTeamsLed
	// ReachesEveryTeam opens any team's week, membership unasked.
	ReachesEveryTeam
)

// TeamWeekReachOf answers which teams' weeks this caller may open.
//
// The ONE answer for both the store that serves a team's week and the field
// Home offers the week on, so the control cannot offer a door the store
// refuses.
//
// A lead is a coaching seat, the same one RequireCoach admits: team_membership
// records who is on a team and not who leads it, and a coaching role is the
// only thing in the tree that says "this seat leads the team it is on". An
// own-scoped seat is refused whatever it holds, because the page it would get
// is about colleagues whose rows it cannot read.
//
// Home offers the week on this answer, carried as the worklist's `team_week`;
// the worklist's own `team` scope is the team's live work and stays on row
// scope.
//
// The system principal reaches every team, as it passes every object grant. A
// buyer reaches none: Allows refuses it and it is not a human seat.
func TeamWeekReachOf(ctx context.Context) TeamWeekReach {
	p, ok := principal.Actor(ctx)
	if !ok || p.Permissions.RowScope == principal.RowScopeOwn {
		return ReachesNoTeam
	}
	// The week carries each member's won deals and pipeline totals, so a seat
	// whose deal read was revoked reads none of them, on either arm.
	if !Allows(ctx, "deal", principal.ActionRead) {
		return ReachesNoTeam
	}
	if Allows(ctx, objTeamOversight, principal.ActionRead) {
		return ReachesEveryTeam
	}
	if p.Type == principal.PrincipalHuman && coaches(p) {
		return ReachesTeamsLed
	}
	return ReachesNoTeam
}
