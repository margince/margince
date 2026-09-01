// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

// WHO is on my team. The enumerating half of the membership question whose
// yes/no half is SharesLiveTeamWithCaller, and both answer from the same
// predicate on purpose: a board that listed somebody the yes/no read then
// refused would show a manager a name they cannot open.

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// teamRosterCap bounds one answer. A team is people, not a directory, and a
// manager reading a hundred names has a reporting question rather than a queue
// one.
const teamRosterCap = 100

// TeamMember is one live human seat sharing a live team with the caller.
//
// No permissions and no team name: the question this answers is "whose work may
// I look at", and which team the edge came through is not what the asker does
// next. A member of two shared teams is one member here.
type TeamMember struct {
	UserID      ids.UUID
	DisplayName string
	Email       string
}

// LiveTeammatesOfCaller lists the live human seats sharing a live team with the
// caller, the caller included.
//
// The caller is IN the list, and that is the point rather than an accident: a
// board of everyone else's load with the reader's own absent invites the
// reading that the manager carries none, and a lead who also sells needs their
// own row beside their team's.
//
// A caller on NO live team gets themselves alone rather than nothing. Empty and
// "only you" are different answers to a manager — the first reads as "this
// installation has no people" and sends them looking for the outage, the second
// says plainly that nobody has been put on a team with them yet. An admin
// reaches every row by tier and can still be on no team at all, so this is the
// ordinary case rather than a corner.
//
// Same three joins as SharesLiveTeamWithCaller — live team, live seat — because
// the two must not disagree. If this listed a departed colleague the board would
// print a name whose queue the other read refuses to open, and the manager would
// read the 403 as a bug rather than as the seat being gone.
//
// Agent seats are absent, which SharesLiveTeamWithCaller does not have to say:
// nothing puts an agent in a team, but a board is a list of people to coach and
// an agent is not one of them.
//
// The parent_team_id hierarchy is NOT walked here either, for the reason it is
// not walked there: row scope does not walk it, so a wider answer would name
// people whose rows the reader cannot then read.
//
// The bool reports that the roster was CUT — more live teammates exist than the
// answer names. A caller drawing a board says so rather than presenting a
// hundred people as the whole team, which is the same under-reporting rule every
// bounded read here keeps: a truncation nobody is told about reads exactly like
// a complete answer.
func (s *Service) LiveTeammatesOfCaller(ctx context.Context) ([]TeamMember, bool, error) {
	// Refuses an agent seat and a Deal Room buyer. The seated-identity check
	// follows for the principals RequireHuman admits without a place on the
	// chart — a background pass has no team to enumerate.
	if err := auth.RequireHuman(ctx); err != nil {
		return nil, false, err
	}
	actor, ok := principal.Actor(ctx)
	if !ok || actor.Type != principal.PrincipalHuman || actor.UserID.IsZero() {
		return nil, false, apperrors.ErrPermissionDenied
	}
	me := ids.From[ids.UserKind](actor.UserID)
	var members []TeamMember
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		// UNION, not UNION ALL: the caller is normally on their own teams, so
		// the two arms overlap and the duplicate has to collapse. The second
		// arm is what makes "only you" the answer for a teamless reader.
		//
		// The CALLER SORTS FIRST, ahead of the alphabet, so the cap can never
		// cut them. A bare `ORDER BY name LIMIT` dropped the caller out of a
		// team whose first hundred names sorted above theirs — silently, and a
		// board missing the reader's own row invites the reading that the lead
		// carries nothing.
		//
		// One row past the cap is read so the caller can tell a full answer from
		// a truncated one. Asking for the count instead would be a second query
		// over the same joins to learn one bit.
		rows, err := tx.Query(ctx, `
			SELECT id, display_name, email FROM (
			  SELECT u.id, u.display_name, u.email
			    FROM team_membership ma
			    JOIN team_membership mb ON mb.team_id = ma.team_id
			    JOIN team t ON t.id = ma.team_id AND t.archived_at IS NULL
			    JOIN app_user u ON u.id = mb.user_id AND NOT u.is_agent
			                   AND `+LiveMemberSQL("u")+`
			   WHERE ma.user_id = $1
			  UNION
			  SELECT u.id, u.display_name, u.email
			    FROM app_user u
			   WHERE u.id = $1 AND NOT u.is_agent AND `+LiveMemberSQL("u")+`
			) roster
			 ORDER BY (id <> $1), display_name, id
			 LIMIT $2`, me, teamRosterCap+1)
		if err != nil {
			return err
		}
		defer rows.Close()
		members = []TeamMember{}
		for rows.Next() {
			var member TeamMember
			if err := rows.Scan(&member.UserID, &member.DisplayName, &member.Email); err != nil {
				return err
			}
			members = append(members, member)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, false, err
	}
	// The extra row is the SIGNAL, never part of the answer: reporting it would
	// put one member past the cap on the board and leave the next read's cut in
	// a different place.
	if len(members) > teamRosterCap {
		return members[:teamRosterCap], true, nil
	}
	return members, false, nil
}
