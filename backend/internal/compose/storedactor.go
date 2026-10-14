// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"
	"fmt"

	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// The two ways a stored actor can have lost the authority it acted with. Both
// are permission refusals: retrying cannot bring a seat or a passport back.
var (
	errActorSeatGone = fmt.Errorf(
		"the member this work was queued under no longer holds a seat that may write: %w", apperrors.ErrPermissionDenied)
	errActorPassportRevoked = fmt.Errorf(
		"the agent passport this work was queued under is no longer live: %w", apperrors.ErrPermissionDenied)
)

// storedActor turns the actor saved with queued work back into a principal. It
// acts under the authority its human holds now, not the one held at queueing.
type storedActor struct {
	authority *identity.Service
}

// rebuild re-derives the principal a stored actor names.
//
// The grants are the human's, read live with the seat in one snapshot, so a
// stored agent cannot widen them. The agent's own stored id is kept: one derived
// from the human's id would merge every agent of that human into one.
// An agent keeps those of scopes its live passport still holds.
func (s storedActor) rebuild(ctx context.Context, stored migration.StoredActor, scopes ...principal.Scope) (principal.Principal, error) {
	if stored.Type != principal.PrincipalHuman && stored.Type != principal.PrincipalAgent {
		return principal.Principal{}, fmt.Errorf("a stored %q actor cannot be acted as: %w", stored.Type, apperrors.ErrPermissionDenied)
	}
	ws, ok := principal.WorkspaceID(ctx)
	if !ok {
		return principal.Principal{}, errors.New("rebuilding a stored actor outside workspace context")
	}
	human := stored.UserID
	rbac, seat, err := s.authority.EffectiveAuthority(ctx, ws, human)
	if errors.Is(err, apperrors.ErrNotFound) {
		return principal.Principal{}, errActorSeatGone
	}
	if err != nil {
		return principal.Principal{}, fmt.Errorf("reading the authority of %s: %w", human, err)
	}
	if !seat.CanMutate() {
		return principal.Principal{}, errActorSeatGone
	}

	actor := principal.Principal{
		Type:        principal.PrincipalHuman,
		ID:          "human:" + human.String(),
		UserID:      human,
		TeamIDs:     rbac.TeamIDs,
		SeatType:    seat,
		Permissions: rbac.Permissions,
		// Queued work names whose it is even when they act themselves, so its
		// audit rows say who it was for, not only who ran it.
		OnBehalfOf: human,
	}
	if !stored.OnBehalfOf.IsZero() {
		actor.OnBehalfOf = stored.OnBehalfOf
	}
	if stored.Type == principal.PrincipalHuman {
		return actor, nil
	}
	if stored.ID == "" {
		return principal.Principal{}, fmt.Errorf("a stored agent actor for %s names no agent", human)
	}
	live, ok, err := s.passportStillLive(ctx, stored.PassportID, human)
	if err != nil {
		return principal.Principal{}, err
	}
	if !ok {
		return principal.Principal{}, errActorPassportRevoked
	}
	actor.Type = principal.PrincipalAgent
	actor.ID = stored.ID
	actor.PassportID = stored.PassportID
	actor.ConnectionID = stored.ConnectionID
	actor.Scopes = live.within(scopes)
	return actor, nil
}

// livePassport is what a re-authenticated passport still grants.
type livePassport struct {
	scopes principal.ScopeSet
	// unbounded marks an agent that presented no passport: no credential
	// exists to narrow the verbs, so the human's authority alone governs.
	unbounded bool
}

// within keeps the verbs this passport still holds.
func (l livePassport) within(scopes []principal.Scope) principal.ScopeSet {
	if len(scopes) == 0 {
		return nil
	}
	kept := principal.NewScopeSet()
	for _, sc := range scopes {
		if l.unbounded || l.scopes.Has(sc) {
			kept[sc] = struct{}{}
		}
	}
	return kept
}

// passportStillLive re-authenticates a stored passport, and answers false when
// it is no longer live.
//
// The human's authority cannot see a revoked passport. Without this check, work
// an operator stopped by revoking the credential would go on under it.
func (s storedActor) passportStillLive(ctx context.Context, passport, human ids.UUID) (livePassport, bool, error) {
	if passport.IsZero() {
		return livePassport{unbounded: true}, true, nil
	}
	live, err := s.authority.AuthenticateAgentByID(ctx, ids.From[ids.PassportKind](passport))
	if errors.Is(err, apperrors.ErrNotFound) {
		// Revoked, expired, or its human is gone.
		return livePassport{}, false, nil
	}
	if err != nil {
		return livePassport{}, false, fmt.Errorf("re-authenticating the stored passport: %w", err)
	}
	// A passport that now belongs to somebody else is not the credential the
	// work was queued under, whatever its id says.
	if live.OnBehalfOf.UUID != human {
		return livePassport{}, false, nil
	}
	return livePassport{scopes: live.Scopes}, true, nil
}

// actAsRun binds the principal that queued an import attempt, and that
// attempt's own correlation, so every slice of one attempt shares one trace.
//
// Both import tools spend write, so an agent acts with write and nothing more.
func (s storedActor) actAsRun(ctx context.Context, run migration.Run) (context.Context, error) {
	if run.Actor == nil {
		return nil, fmt.Errorf("import run %s names no actor to act as: %w", run.ID, apperrors.ErrPermissionDenied)
	}
	if run.AttemptCorrelationID == nil {
		return nil, fmt.Errorf("import run %s attempt %d carries no correlation", run.ID, run.JobAttempt)
	}
	actor, err := s.rebuild(ctx, *run.Actor, principal.ScopeWrite)
	if err != nil {
		return nil, err
	}
	return principal.WithCorrelationID(principal.WithActor(ctx, actor), *run.AttemptCorrelationID), nil
}
