// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// An agent may run its user's member administration, but never the parts that
// would let a credential widen what a credential may do. Those are the admin
// role, its own user's seat, an administrator's seat and the address a password
// reset goes to. Each is refused here, beside the checks a human passes.

func agentLimit(detail string) error {
	return &httperr.DetailedError{Status: http.StatusForbidden, Code: "agent_member_limit", Detail: detail}
}

func callingAgent(ctx context.Context) (principal.Principal, bool) {
	p, ok := principal.Actor(ctx)
	return p, ok && p.Type == principal.PrincipalAgent
}

// refuseAgentMemberReach refuses an agent acting on its own user's seat or on
// an administrator's. It holds the authorization lock first, because some
// callers do not, and a promotion must not land between this read and the act.
func refuseAgentMemberReach(ctx context.Context, tx pgx.Tx, target ids.UserID) error {
	p, isAgent := callingAgent(ctx)
	if !isAgent {
		return nil
	}
	if err := lockAuthorization(ctx, tx); err != nil {
		return err
	}
	_, roles, err := loadRoleDocuments(ctx, tx, []ids.UUID{target.UUID})
	if err != nil {
		return err
	}
	return refuseAgentReachOf(p, target.UUID, roles[target.UUID])
}

func refuseAgentReachOf(p principal.Principal, target ids.UUID, roles []string) error {
	if p.OnBehalfOf == target {
		return agentLimit("an agent does not change its own user's membership, role or teams; ask them to do it in the app")
	}
	if slices.Contains(roles, roleAdmin) {
		return agentLimit("an agent does not change an administrator's seat; an administrator does that in the app")
	}
	return nil
}

// refuseAgentTeamReach refuses an agent archiving or restoring a team holding a
// member it may not act on. Either act switches that member's team reach.
func refuseAgentTeamReach(ctx context.Context, tx pgx.Tx, team ids.UUID) error {
	p, isAgent := callingAgent(ctx)
	if !isAgent {
		return nil
	}
	rows, err := tx.Query(ctx, `SELECT user_id FROM team_membership WHERE team_id = $1`, team)
	if err != nil {
		return err
	}
	members, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return err
	}
	_, roles, err := loadRoleDocuments(ctx, tx, members)
	if err != nil {
		return err
	}
	for _, member := range members {
		if err := refuseAgentReachOf(p, member, roles[member]); err != nil {
			return err
		}
	}
	return nil
}

// refuseAgentAdminRole refuses an agent handing out the admin role.
func refuseAgentAdminRole(ctx context.Context, role string) error {
	if _, isAgent := callingAgent(ctx); isAgent && role == roleAdmin {
		return agentLimit("an agent does not grant the admin role; an administrator does that in the app")
	}
	return nil
}

// refuseAgentEmailChange refuses an agent moving a member's address, which is
// where their password reset goes.
func refuseAgentEmailChange(ctx context.Context, tx pgx.Tx, target ids.UserID, email string) error {
	if _, isAgent := callingAgent(ctx); !isAgent {
		return nil
	}
	var current string
	err := tx.QueryRow(ctx, `SELECT email FROM app_user WHERE id = $1`, target).Scan(&current)
	if errors.Is(err, pgx.ErrNoRows) {
		return apperrors.ErrNotFound
	}
	if err != nil {
		return err
	}
	if !strings.EqualFold(current, email) {
		return agentLimit("an agent does not change a member's email address; send the same address, or ask an administrator")
	}
	return nil
}

// RefuseAgentOwnReceiver refuses an agent handing a colleague's work to its own
// user: a departed member's private records would land where its passport
// reads them.
func RefuseAgentOwnReceiver(ctx context.Context, receiver ids.UUID) error {
	if p, isAgent := callingAgent(ctx); isAgent && p.OnBehalfOf == receiver {
		return agentLimit("an agent does not hand a colleague's work to its own user; choose another colleague, or ask an administrator")
	}
	return nil
}
