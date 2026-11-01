// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package approvals

import (
	"context"
	"sync"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// StagedBy is who staged a proposal, in the columns staging writes from its actor.
type StagedBy struct {
	ProposedBy   string
	OnBehalfOf   ids.UUID
	PassportID   ids.UUID
	ConnectionID ids.UUID
}

// EffectActor re-derives, on the caller's transaction, the live principal of
// the agent that staged a proposal. That agent redeems it, so the effect runs
// as this principal and not as whoever reviews it.
type EffectActor func(ctx context.Context, tx pgx.Tx, by StagedBy) (principal.Principal, error)

// WithEffectActor makes the context pins and the preview read an agent's
// proposal as that agent. Without it they read as the caller.
func (s *Service) WithEffectActor(resolve EffectActor) *Service {
	s.effectActor = resolve
	return s
}

func stagedByRow(a row) StagedBy {
	by := StagedBy{ProposedBy: a.ProposedBy}
	if a.OnBehalfOf != nil {
		by.OnBehalfOf = a.OnBehalfOf.UUID
	}
	if a.PassportID != nil {
		by.PassportID = a.PassportID.UUID
	}
	if a.ConnectionID != nil {
		by.ConnectionID = *a.ConnectionID
	}
	return by
}

func stagedByActor(p principal.Principal) StagedBy {
	return StagedBy{ProposedBy: p.ID, OnBehalfOf: p.OnBehalfOf, PassportID: p.PassportID, ConnectionID: p.ConnectionID}
}

// runsAsAgent is serverProposed's other side: a row with a passport is spent
// by the agent's own retry, and a server proposal by its decider.
func (s *Service) runsAsAgent(by StagedBy) bool {
	return s.effectActor != nil && !by.PassportID.IsZero()
}

// effectContext binds ctx to the principal the proposal's effect runs as.
func (s *Service) effectContext(ctx context.Context, tx pgx.Tx, by StagedBy) (context.Context, error) {
	if !s.runsAsAgent(by) {
		return ctx, nil
	}
	actor, err := s.effectActor(ctx, tx, by)
	if err != nil {
		return ctx, err
	}
	return principal.WithActor(ctx, actor), nil
}

type previewEffectKey struct{}

// withPreviewEffect lets the preview builders ask for the effect principal.
// It is resolved on first use, since most kinds never ask.
func (s *Service) withPreviewEffect(ctx context.Context, subject row) context.Context {
	by := stagedByRow(subject)
	if !s.runsAsAgent(by) {
		return ctx
	}
	resolve := sync.OnceValues(func() (principal.Principal, error) {
		var actor principal.Principal
		err := s.db.Tx(ctx, func(tx pgx.Tx) error {
			var err error
			actor, err = s.effectActor(ctx, tx, by)
			return err
		})
		return actor, err
	})
	return context.WithValue(ctx, previewEffectKey{}, resolve)
}

// EffectContext answers ctx bound to the principal the previewed proposal's
// effect runs as. asAgent is false when that principal is the caller, as it
// is for a server proposal, which its decider's own decision runs.
func EffectContext(ctx context.Context) (effect context.Context, asAgent bool, err error) {
	resolve, ok := ctx.Value(previewEffectKey{}).(func() (principal.Principal, error))
	if !ok {
		return ctx, false, nil
	}
	actor, err := resolve()
	if err != nil {
		return ctx, true, err
	}
	return principal.WithActor(ctx, actor), true, nil
}
