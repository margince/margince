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
	ctx, err := admit(ctx, actor, objectTeamAdmin, principal.ActionCreate)
	if err != nil {
		return Team{}, err
	}
	name, err = validTeamName(name)
	if err != nil {
		return Team{}, err
	}
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
//
// A rename changes nobody's reach and stays on team_admin. Archiving and
// restoring switch every member's team reach off and on, so they are an
// admin's, like every other change to who is on a team.
func (s *Service) UpdateTeam(ctx context.Context, actor Identity, id ids.UUID, in UpdateTeamInput) (Team, error) {
	ctx, err := admit(ctx, actor, objectTeamAdmin, principal.ActionUpdate)
	if err != nil {
		return Team{}, err
	}
	if err := refuseTeamMembershipUnlessAdmin(actor, in.Archived != nil); err != nil {
		return Team{}, err
	}
	var name *string
	if in.Name != nil {
		valid, err := validTeamName(*in.Name)
		if err != nil {
			return Team{}, err
		}
		name = &valid
	}
	var out Team
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		if err := lockAuthorization(ctx, tx); err != nil {
			return err
		}
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
			err := tx.QueryRow(ctx, `UPDATE team SET `+set+` WHERE id = $1 RETURNING archived_at`, id).Scan(&out.ArchivedAt)
			if storekit.IsUniqueViolation(err) {
				return fmt.Errorf("%w: a team named %q already exists", apperrors.ErrConflict, before.Name)
			}
			if err != nil {
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
	ctx, err := admit(ctx, actor, objectTeamAdmin, principal.ActionUpdate)
	if err != nil {
		return err
	}
	if err := refuseTeamMembershipUnlessAdmin(actor, true); err != nil {
		return err
	}
	return s.db.Tx(ctx, func(tx pgx.Tx) error {
		if err := lockAuthorization(ctx, tx); err != nil {
			return err
		}
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
			change = changeMemberAdded
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
			map[string]any{teamAuditKeyMember: userID, "on": !on},
			map[string]any{teamAuditKeyMember: userID, "on": on})
	})
}

// errTeamMembershipRequiresAdmin refuses a change to who is on a team by a
// caller who is not an admin.
var errTeamMembershipRequiresAdmin = fmt.Errorf("%w: only an admin changes who is on a team", apperrors.ErrPermissionDenied)

// refuseTeamMembershipUnlessAdmin holds the one rule for teams: only an admin
// changes who is on a team. Adding a member widens their reach and makes them
// coachable by the team's leads; removing one ends both. Either reshapes
// authority the way a role change does, and team_admin is not role authority.
func refuseTeamMembershipUnlessAdmin(actor Identity, changesMembership bool) error {
	if changesMembership && !actor.hasRole(roleAdmin) {
		return errTeamMembershipRequiresAdmin
	}
	return nil
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
// this cannot be asked about two other contacts. That is the gate: the answer
// discloses one edge of the company chart, and the only edge a reader is
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
	me, err := teamMembershipHuman(ctx)
	if err != nil {
		return false, err
	}
	if me == other.UUID {
		return true, nil
	}
	var shares bool
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		var err error
		shares, err = SharesLiveTeamWithCallerTx(ctx, tx, other)
		return err
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
	if _, err := teamMembershipHuman(ctx); err != nil {
		return false, err
	}
	var member bool
	err := s.db.Tx(ctx, func(tx pgx.Tx) error { var err error; member, err = CallerLeadsLiveTeamTx(ctx, tx, team); return err })
	return member, err
}

// validTeamName trims and bounds a team name.
func validTeamName(raw string) (string, error) {
	name := strings.TrimSpace(raw)
	if !values.HasVisibleText(name) || utf8.RuneCountInString(name) > maxTeamName {
		return "", &values.ParseError{
			Field: "name", Code: "invalid_team_name",
			Message: fmt.Sprintf("a team name is 1 to %d characters", maxTeamName),
		}
	}
	return name, nil
}
