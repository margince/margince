// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The weekly job's measurement phase: freezing each team's week.
//
// Its own file because it is a different subject from the per-rep pass beside
// it — personal reviews measure one member under their own authority, while
// this totals the measured members before optional model or mail calls.

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/weekly"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// snapshotTeams freezes each live team's week, once the member reviews are in.
//
// One snapshot per team, assembled under the authority of a member who leads
// it: the read is of frozen rows the team's own contacts wrote, and the tier gate
// on TeamReview is what stops an own-scoped seat asking for one.
func (w *weeklyGenerateWorker) snapshotTeams(
	ctx context.Context, wsID ids.UUID, now time.Time, failedMembers map[ids.UUID]bool,
) []error {
	teams, err := w.liveTeams(ctx)
	if err != nil {
		return []error{fmt.Errorf("listing the workspace's teams: %w", err)}
	}
	var failures []error
	for _, team := range teams {
		if slices.ContainsFunc(team.candidates, func(id ids.UUID) bool { return failedMembers[id] }) {
			continue
		}
		if err := w.snapshotTeam(ctx, wsID, team, now); err != nil {
			failures = append(failures, fmt.Errorf("team weekly for %s: %w", team.id, err))
		}
	}
	return failures
}

// liveTeam is one team to snapshot, with the seat whose authority reads it.
type liveTeam struct {
	id   ids.UUID
	name string
	// Authority is resolved per candidate; UUID order does not identify a lead.
	candidates []ids.UUID
}

// liveTeams lists the workspace's live teams with a member to act as.
//
// A team with no live members is skipped rather than snapshotted empty: an
// empty week for a team that has nobody on it is a row saying the team did
// nothing, which is not what happened.
func (w *weeklyGenerateWorker) liveTeams(ctx context.Context) ([]liveTeam, error) {
	var teams []liveTeam
	err := database.WithWorkspaceTx(ctx, w.pool, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT t.id, t.name, array_agg(u.id::text ORDER BY u.id)
			  FROM team t
			  JOIN team_membership m ON m.team_id = t.id
			  JOIN app_user u ON u.id = m.user_id
			    AND `+identity.LiveMemberSQL("u")+` AND NOT u.is_agent
			 WHERE t.archived_at IS NULL
			 GROUP BY t.id, t.name
			 ORDER BY t.id`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var team liveTeam
			var candidates []string
			if err := rows.Scan(&team.id, &team.name, &candidates); err != nil {
				return err
			}
			for _, candidate := range candidates {
				id, err := ids.Parse(candidate)
				if err != nil {
					return err
				}
				team.candidates = append(team.candidates, id)
			}
			teams = append(teams, team)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return teams, nil
}

// snapshotTeam freezes one team's week under a member's own authority.
func (w *weeklyGenerateWorker) snapshotTeam(
	ctx context.Context, wsID ids.UUID, team liveTeam, now time.Time,
) error {
	for _, candidate := range team.candidates {
		err := w.snapshotTeamAs(ctx, wsID, team, candidate, now)
		if errors.Is(err, apperrors.ErrPermissionDenied) || errors.Is(err, apperrors.ErrNotFound) {
			continue
		}
		return err
	}
	w.log.InfoContext(ctx, "no authorized member can measure the team's week", "team", team.id)
	return nil
}

func (w *weeklyGenerateWorker) snapshotTeamAs(ctx context.Context, wsID ids.UUID, team liveTeam, candidate ids.UUID, now time.Time) error {
	rbac, seat, err := w.users.EffectiveAuthority(ctx, wsID, candidate)
	if err != nil {
		return fmt.Errorf("resolving the member's authority: %w", err)
	}
	memberCtx := principal.WithActor(ctx, principal.Principal{
		Type:        principal.PrincipalHuman,
		ID:          "human:" + candidate.String(),
		UserID:      candidate,
		SeatType:    seat,
		TeamIDs:     rbac.TeamIDs,
		Permissions: rbac.Permissions,
	})
	memberCtx = principal.WithCorrelationID(memberCtx, ids.NewV7())

	members, err := w.teamMembers(memberCtx, team.id)
	if err != nil {
		return err
	}
	_, _, err = w.engine.AssembleTeamFor(memberCtx, team.id, team.name, members, now)
	return err
}

// teamMembers lists who was on the team, for freezing into the snapshot.
//
// Read HERE rather than through the identity roster seam because that one
// answers "who shares a team with the caller" — the union across every team
// they are on. A snapshot is about ONE team, and a lead on two would otherwise
// freeze both teams' contacts into each.
func (w *weeklyGenerateWorker) teamMembers(
	ctx context.Context, teamID ids.UUID,
) ([]weekly.TeamMember, error) {
	var members []weekly.TeamMember
	err := database.WithWorkspaceTx(ctx, w.pool, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT u.id, u.display_name FROM team_membership m
			  JOIN app_user u ON u.id = m.user_id
			    AND `+identity.LiveMemberSQL("u")+` AND NOT u.is_agent
			 WHERE m.team_id = $1
			 ORDER BY u.display_name, u.id`, teamID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var member weekly.TeamMember
			if err := rows.Scan(&member.UserID, &member.DisplayName); err != nil {
				return err
			}
			members = append(members, member)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("weekly: listing the team's members: %w", err)
	}
	return members, nil
}
