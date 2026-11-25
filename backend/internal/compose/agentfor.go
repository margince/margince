// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The product acting for one named person: an agent principal named for the
// job, carrying that person's grants as they are now.
//
// A job bound as the system principal passes auth.Require without reading any
// grant, so a job someone started could do what that person may not. Bound
// through here, the person's live grants decide, and a person who left or was
// demoted cannot act through the job.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/authz"
)

// agentFor binds the product acting FOR person: an agent principal named
// actorID, carrying OnBehalfOf and the person's grants as they are now. A
// person who left answers apperrors.ErrNotFound, and the caller does not act.
func agentFor(ctx context.Context, users *identity.Service, person ids.UUID, actorID string) (context.Context, error) {
	wsID, ok := principal.WorkspaceID(ctx)
	if !ok {
		return nil, errors.New("compose: acting for a person outside a bound workspace")
	}
	member, err := authz.MemberPrincipal(ctx, users, wsID, person)
	if err != nil {
		return nil, fmt.Errorf("compose: authority of the person to act for: %w", err)
	}
	agent := agentPrincipalFor(member, actorID)
	lang, err := users.ReadingLanguage(ctx, agent)
	return withOwnersLanguage(ctx, agent, lang, err)
}

// agentForTx is agentFor read in the write's own transaction. The write acts
// on the grants a role change or suspension committed before it left.
func agentForTx(users *identity.Service, person ids.UUID, actorID string) func(context.Context, pgx.Tx) (context.Context, error) {
	return func(ctx context.Context, tx pgx.Tx) (context.Context, error) {
		rbac, seat, err := users.EffectiveAuthorityTx(ctx, tx, person)
		if err != nil {
			return nil, fmt.Errorf("compose: authority of the person to act for: %w", err)
		}
		agent := agentPrincipalFor(authz.HumanPrincipal(person, rbac, seat), actorID)
		lang, err := identity.ReadingLanguageTx(ctx, tx, agent)
		return withOwnersLanguage(ctx, agent, lang, err)
	}
}

// agentPrincipalFor is the agent acting for person under actorID.
func agentPrincipalFor(person principal.Principal, actorID string) principal.Principal {
	return principal.Principal{
		Type:        principal.PrincipalAgent,
		ID:          actorID,
		UserID:      person.UserID,
		OnBehalfOf:  person.UserID,
		SeatType:    person.SeatType,
		TeamIDs:     person.TeamIDs,
		Permissions: person.Permissions,
		// Write, and only write: a pass acting unattended changes records and
		// never puts a message on the wire.
		Scopes: principal.ScopeSet{principal.ScopeWrite: struct{}{}},
	}
}
