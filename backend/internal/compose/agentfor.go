// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The product acting for one named colleague: an agent principal named for the
// job, carrying that colleague's grants as they are now.
//
// A system principal passes auth.Require without reading any grant. Bound
// through here, the colleague's live grants decide what the job may do.
// A job's own run row is kept by the system (forColleague), so a refused run
// still ends.

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/authz"
)

// agentActorPrefix and systemActorPrefix are the kinds of the actor ids this
// seam binds: the work's and the bookkeeping's.
const (
	agentActorPrefix  = "agent:"
	systemActorPrefix = "system:"
)

// agentFor binds the product acting FOR colleague: an agent principal named
// actorID, carrying OnBehalfOf and the colleague's grants as they are now. A
// colleague who left answers apperrors.ErrNotFound, and the caller does not act.
func agentFor(ctx context.Context, users *identity.Service, colleague ids.UUID, actorID string) (context.Context, error) {
	wsID, ok := principal.WorkspaceID(ctx)
	if !ok {
		return nil, errors.New("compose: acting for a colleague outside a bound workspace")
	}
	member, err := authz.MemberPrincipal(ctx, users, wsID, colleague)
	if err != nil {
		return nil, fmt.Errorf("compose: authority of the colleague to act for: %w", err)
	}
	agent, err := agentPrincipalFor(member, actorID)
	if err != nil {
		return nil, err
	}
	lang, err := users.ReadingLanguage(ctx, agent)
	return withOwnersLanguage(ctx, agent, lang, err)
}

// agentForTx is agentFor read in the write's own transaction. The write acts
// on the grants a role change or suspension committed before it left.
func agentForTx(users *identity.Service, colleague ids.UUID, actorID string) func(context.Context, pgx.Tx) (context.Context, error) {
	return func(ctx context.Context, tx pgx.Tx) (context.Context, error) {
		rbac, seat, err := users.EffectiveAuthorityTx(ctx, tx, colleague)
		if err != nil {
			return nil, fmt.Errorf("compose: authority of the colleague to act for: %w", err)
		}
		agent, err := agentPrincipalFor(authz.HumanPrincipal(colleague, rbac, seat), actorID)
		if err != nil {
			return nil, err
		}
		lang, err := identity.ReadingLanguageTx(ctx, tx, agent)
		return withOwnersLanguage(ctx, agent, lang, err)
	}
}

// agentPrincipalFor is the agent acting for colleague under actorID. storekit
// stamps actor_type from the type and actor_id from the id. An id of another
// kind would write an audit row naming two actors, so it is refused.
func agentPrincipalFor(colleague principal.Principal, actorID string) (principal.Principal, error) {
	if !strings.HasPrefix(actorID, agentActorPrefix) || len(actorID) == len(agentActorPrefix) {
		return principal.Principal{}, fmt.Errorf("compose: %q is not an agent id, so no agent may act under it", actorID)
	}
	return principal.Principal{
		Type:        principal.PrincipalAgent,
		ID:          actorID,
		UserID:      colleague.UserID,
		OnBehalfOf:  colleague.UserID,
		SeatType:    colleague.SeatType,
		TeamIDs:     colleague.TeamIDs,
		Permissions: colleague.Permissions,
		// Write, and only write: a pass acting unattended changes records and
		// never puts a message on the wire.
		Scopes: principal.ScopeSet{principal.ScopeWrite: struct{}{}},
	}, nil
}

// agentForActor binds actorID acting for the principal on ctx, with that
// principal's grants. An approved effect or an accepted reading writes as the
// product's agent, held to what the human who released it may do.
func agentForActor(ctx context.Context, actorID string) (context.Context, error) {
	human, ok := principal.Actor(ctx)
	if !ok {
		return nil, fmt.Errorf("compose: %s acts for nobody, as no principal is bound", actorID)
	}
	agent, err := agentPrincipalFor(human, actorID)
	if err != nil {
		return nil, err
	}
	return principal.WithActor(ctx, agent), nil
}

// colleagueJob names the two principals of a job the product runs for a named
// colleague. work is the agent that does the job's real work under the
// colleague's grants. books is the system that keeps the run's own row: claim,
// stage, finish, fail, defer.
type colleagueJob struct {
	work  string
	books string
}

// forColleague binds one run of job for colleague. books always binds, so the
// run can be ended whatever the colleague may still do. work is nil, and err
// answers workRefused, when the colleague left or lost their seat.
//
// Both name the colleague as the one acted for: aiactivity files a run on the
// colleague's activity rail through on_behalf_of, and owner-only checks read
// UserID.
func forColleague(ctx context.Context, users *identity.Service, colleague ids.UUID, job colleagueJob) (books, work context.Context, err error) {
	if !strings.HasPrefix(job.books, systemActorPrefix) || len(job.books) == len(systemActorPrefix) {
		return nil, nil, fmt.Errorf("compose: %q is not a system id, so the run's bookkeeping may not act under it", job.books)
	}
	books = principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalSystem, ID: job.books, UserID: colleague, OnBehalfOf: colleague,
	})
	work, err = agentFor(ctx, users, colleague, job.work)
	return books, work, err
}

// workIn moves the work's agent onto ctx, which derives from the bookkeeping.
// The work then keeps what was bound there since, such as a work lease. A
// refused work stays nil.
func workIn(ctx, work context.Context) context.Context {
	if work == nil {
		return nil
	}
	agent, ok := principal.Actor(work)
	if !ok {
		return nil
	}
	return principal.WithActor(ctx, agent)
}

// workRefused is the colleague's grants not covering the work: they left, lost
// a grant, or no longer see the record. The run is ended through its
// bookkeeping as failed. A retry would meet the same answer.
func workRefused(err error) bool {
	return errors.Is(err, apperrors.ErrPermissionDenied) || errors.Is(err, apperrors.ErrNotFound)
}
