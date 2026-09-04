// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

// Team administration. A team is what resolves `row_scope: team` — who may
// EDIT whose records, since customer identity is workspace-readable — and it
// is a share subject, so every change here moves somebody's write authority
// from the next request on. Admin-only, and each change is one transaction:
// the row, its audit row and team.changed on the identity stream.

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// maxTeamName mirrors the contract's maxLength.
const maxTeamName = 120

// Team is one team row as the admin surface returns it.
type Team struct {
	ID         ids.UUID
	Name       string
	ArchivedAt *time.Time
}

// CreateTeam makes a team. A name already in use answers ErrConflict — two
// teams with one name would be two answers to "which team is DACH Sales".
func (s *Service) CreateTeam(ctx context.Context, actor Identity, name string) (Team, error) {
	if !actor.hasRole(roleAdmin) {
		return Team{}, apperrors.ErrPermissionDenied
	}
	name, err := validTeamName(name)
	if err != nil {
		return Team{}, err
	}
	ctx = actorCtx(ctx, actor)
	var out Team
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `INSERT INTO team (name) VALUES ($1) RETURNING id, name`, name).
			Scan(&out.ID, &out.Name)
		if storekit.IsUniqueViolation(err) {
			return fmt.Errorf("%w: a team named %q already exists", apperrors.ErrConflict, name)
		}
		if err != nil {
			return err
		}
		return s.recordTeamChange(ctx, tx, actor, out.ID, nil, "created", nil, map[string]any{"name": name})
	})
	return out, err
}

// UpdateTeamInput is one rename and/or archive flip; nil leaves a field alone.
type UpdateTeamInput struct {
	Name     *string
	Archived *bool
}

// UpdateTeam renames, archives or restores a team. Archiving keeps the rows
// and the memberships; an archived team stops resolving scope and shares
// because every reader of team_membership joins a live team.
func (s *Service) UpdateTeam(ctx context.Context, actor Identity, id ids.UUID, in UpdateTeamInput) (Team, error) {
	if !actor.hasRole(roleAdmin) {
		return Team{}, apperrors.ErrPermissionDenied
	}
	var name *string
	if in.Name != nil {
		valid, err := validTeamName(*in.Name)
		if err != nil {
			return Team{}, err
		}
		name = &valid
	}
	ctx = actorCtx(ctx, actor)
	var out Team
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		var before Team
		if err := tx.QueryRow(ctx, `SELECT id, name, archived_at FROM team WHERE id = $1 FOR UPDATE`, id).
			Scan(&before.ID, &before.Name, &before.ArchivedAt); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return apperrors.ErrNotFound
			}
			return err
		}
		out = before
		if name != nil && *name != before.Name {
			err := tx.QueryRow(ctx, `UPDATE team SET name = $2 WHERE id = $1 RETURNING name`, id, *name).Scan(&out.Name)
			if storekit.IsUniqueViolation(err) {
				return fmt.Errorf("%w: a team named %q already exists", apperrors.ErrConflict, *name)
			}
			if err != nil {
				return err
			}
			if err := s.recordTeamChange(ctx, tx, actor, id, nil, "renamed",
				map[string]any{"name": before.Name}, map[string]any{"name": out.Name}); err != nil {
				return err
			}
		}
		if in.Archived != nil && *in.Archived != (before.ArchivedAt != nil) {
			change, set := "restored", `archived_at = NULL`
			if *in.Archived {
				change, set = "archived", `archived_at = now()`
			}
			if err := tx.QueryRow(ctx, `UPDATE team SET `+set+` WHERE id = $1 RETURNING archived_at`, id).Scan(&out.ArchivedAt); err != nil {
				return err
			}
			if err := s.recordTeamChange(ctx, tx, actor, id, nil, change,
				map[string]any{"archived": before.ArchivedAt != nil}, map[string]any{"archived": *in.Archived}); err != nil {
				return err
			}
		}
		return nil
	})
	return out, err
}

// SetTeamMember puts a member on a team (on=true) or takes them off. Both are
// idempotent: the state the admin asked for is the state, and a change that
// changes nothing writes no audit noise. An agent seat holds no team.
func (s *Service) SetTeamMember(ctx context.Context, actor Identity, teamID, userID ids.UUID, on bool) error {
	if !actor.hasRole(roleAdmin) {
		return apperrors.ErrPermissionDenied
	}
	ctx = actorCtx(ctx, actor)
	return s.db.Tx(ctx, func(tx pgx.Tx) error {
		// The team is locked for the write: an archive committing between
		// this check and the insert would otherwise leave a member on a
		// team nobody can see, holding authority the moment it is restored.
		var teamExists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM team WHERE id = $1 AND archived_at IS NULL FOR UPDATE)`, teamID).Scan(&teamExists); err != nil {
			return err
		}
		if !teamExists {
			return apperrors.ErrNotFound
		}
		// Only a live, active human seat joins a team: an agent holds no
		// team, and a suspended or deactivated member would carry the
		// authority home the day they are reactivated.
		var isAgent bool
		var status string
		err := tx.QueryRow(ctx, `SELECT is_agent, status FROM app_user WHERE id = $1 AND archived_at IS NULL`, userID).Scan(&isAgent, &status)
		if errors.Is(err, pgx.ErrNoRows) {
			return apperrors.ErrNotFound
		}
		if err != nil {
			return err
		}
		if isAgent {
			return errAgentSeatHoldsNoRole
		}
		// An invited member joins: InviteUser itself puts one on teams at invite
		// time (joinTeamsTx), so refusing an admin the correction afterwards
		// would let a mis-typed invitation stand until the member redeems it.
		// A suspended or deactivated member is still refused — their access is
		// withdrawn, and a team grants record scope.
		if on && status != userStatusActive && status != userStatusInvited {
			return fmt.Errorf("%w: a suspended or deactivated member does not join a team; reactivate them first", apperrors.ErrConflict)
		}
		var tag pgconn.CommandTag
		change := "member_removed"
		if on {
			change = "member_added"
			tag, err = tx.Exec(ctx, `INSERT INTO team_membership (team_id, user_id) VALUES ($1, $2)
				ON CONFLICT (team_id, user_id) DO NOTHING`, teamID, userID)
		} else {
			tag, err = tx.Exec(ctx, `DELETE FROM team_membership WHERE team_id = $1 AND user_id = $2`, teamID, userID)
		}
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			return nil
		}
		return s.recordTeamChange(ctx, tx, actor, teamID, &userID, change,
			map[string]any{"member": userID, "on": !on}, map[string]any{"member": userID, "on": on})
	})
}

// recordTeamChange is the write shape's second half for every team change:
// the audit row on the team and team.changed on the identity stream.
func (s *Service) recordTeamChange(ctx context.Context, tx pgx.Tx, actor Identity, teamID ids.UUID, userID *ids.UUID, change string, before, after map[string]any) error {
	action := "update"
	switch change {
	case "created":
		action = "create"
	case "archived":
		action = "archive"
	case "restored":
		action = "restore"
	}
	auditID, err := storekit.Audit(ctx, tx, action, "team", teamID, before, after)
	if err != nil {
		return err
	}
	payload := crmcontracts.PublicEventTeamChanged{
		TeamId: openapi_types.UUID(teamID),
		Change: crmcontracts.PublicEventTeamChangedChange(change),
		By:     openapi_types.UUID(actor.UserID.UUID),
	}
	if userID != nil {
		u := openapi_types.UUID(*userID)
		payload.UserId = &u
	}
	return storekit.EmitEvent(ctx, tx, auditID, teamID, payload)
}

// SharesLiveTeamWithCaller reports whether the named user is on a team with the
// AUTHENTICATED caller.
//
// The caller's own id comes from the principal rather than from an argument, so
// this cannot be asked about two other people. That is the gate: the answer
// discloses one edge of the organization chart, and the only edge a reader is
// entitled to probe is one they are themselves an end of. A caller with no
// human behind it is refused — an agent or a system pass has no teammates, and
// answering "false" would read as a fact rather than as an absence.
//
// Live teams only, matching how row scope resolves membership: an archived team
// keeps its rows so a restore brings them back, but while archived it grants
// nothing, and an answer of true here would hand a reader authority the
// row-scope predicate does not agree with.
//
// The parent_team_id hierarchy is NOT walked, again matching row scope. A lead
// of a parent team reaches a child team's members by belonging to the child
// team too. Walking it here alone would make this answer wider than the
// predicate that decides what the reader then reads.
func (s *Service) SharesLiveTeamWithCaller(ctx context.Context, other ids.UserID) (bool, error) {
	// Refuses an agent seat and a Deal Room buyer. It admits the system and
	// connector principals, which is why the seated-identity check follows: a
	// background pass has no place on the chart to answer from.
	if err := auth.RequireHuman(ctx); err != nil {
		return false, err
	}
	actor, ok := principal.Actor(ctx)
	if !ok || actor.Type != principal.PrincipalHuman || actor.UserID.IsZero() {
		return false, apperrors.ErrPermissionDenied
	}
	me := ids.From[ids.UserKind](actor.UserID)
	// Asking about themselves needs no query: a reader is their own teammate,
	// and the caller need not special-case it.
	if me == other {
		return true, nil
	}
	var shares bool
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		// The other party must be a LIVE seat, not merely a row.
		// team_membership survives a deactivation — SetTeamMember refuses to
		// ADD a suspended member but nothing removes one who leaves — so a
		// membership-only answer would call a departed colleague a teammate,
		// and the callers act on that: one opens their queue, the other puts a
		// notice in it that nobody will ever read.
		return tx.QueryRow(ctx, `SELECT EXISTS (
		         SELECT 1 FROM team_membership ma
		           JOIN team_membership mb ON mb.team_id = ma.team_id AND mb.user_id = $2
		           JOIN team t ON t.id = ma.team_id AND t.archived_at IS NULL
		           JOIN app_user u ON u.id = mb.user_id AND `+LiveMemberSQL("u")+`
		          WHERE ma.user_id = $1)`, me, other).Scan(&shares)
	})
	return shares, err
}

// CallerLeadsLiveTeam reports whether the caller is a live member of a live
// team, by the team's id.
//
// The team-id counterpart to SharesLiveTeamWithCaller, and it holds the same
// posture for the same reasons: humans only, live team, live seat, and no walk
// up parent_team_id. What differs is only which end of the membership edge the
// caller names — a user there, a team here — so the two ask one question of one
// table rather than disagreeing about who is on a team.
//
// A caller asking about a team that does not exist gets false, not an error:
// the answer to "may I read this team" is no either way, and distinguishing the
// two would tell an outsider which team ids are real.
func (s *Service) CallerLeadsLiveTeam(ctx context.Context, team ids.UUID) (bool, error) {
	if err := auth.RequireHuman(ctx); err != nil {
		return false, err
	}
	actor, ok := principal.Actor(ctx)
	if !ok || actor.Type != principal.PrincipalHuman || actor.UserID.IsZero() {
		return false, apperrors.ErrPermissionDenied
	}
	me := ids.From[ids.UserKind](actor.UserID)
	var member bool
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT EXISTS (
		         SELECT 1 FROM team_membership m
		           JOIN team t ON t.id = m.team_id AND t.archived_at IS NULL
		           JOIN app_user u ON u.id = m.user_id AND `+LiveMemberSQL("u")+`
		          WHERE m.team_id = $1 AND m.user_id = $2)`, team, me).Scan(&member)
	})
	return member, err
}

// validTeamName trims and bounds a team name.
func validTeamName(raw string) (string, error) {
	name := strings.TrimSpace(raw)
	if name == "" || utf8.RuneCountInString(name) > maxTeamName {
		return "", &values.ParseError{Field: "name", Code: "invalid_team_name",
			Message: fmt.Sprintf("a team name is 1 to %d characters", maxTeamName)}
	}
	return name, nil
}
