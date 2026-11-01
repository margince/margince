// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package approvals

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// ContextPin digests the facts outside the target row that a kind's reviewed
// outcome reads, such as the company a promotion resolves. The version pin
// cannot see them, because changing them never bumps the target.
//
// The service runs it as the principal the effect runs as (effectContext).
// So the stager, the approver and the redeeming agent digest the same facts.
type ContextPin func(ctx context.Context, tx pgx.Tx, targetID ids.UUID, proposedChange json.RawMessage) (string, error)

// WithContextPin registers the context pin for one staging kind. Staging takes
// it, and the preview, decision, redemption and re-stage all check it.
func (s *Service) WithContextPin(kind string, pin ContextPin) *Service {
	s.contextPins[kind] = pin
	return s
}

// stagedContextDigest is empty for a kind with no context pin.
func (s *Service) stagedContextDigest(ctx context.Context, tx pgx.Tx, in StageInput) (string, error) {
	pin, ok := s.contextPins[in.Kind]
	if !ok {
		return "", nil
	}
	stager, ok := principal.Actor(ctx)
	if !ok {
		return "", errors.New("crmapprovals: no actor bound to context")
	}
	pinCtx, err := s.effectContext(ctx, tx, stagedByActor(stager))
	if err != nil {
		return "", err
	}
	return pin(pinCtx, tx, in.TargetID, in.ProposedChange)
}

// checkContextPin refuses an approval whose context moved since it was pinned.
// Redemption also refuses a row with no digest, which no approval should leave.
func (s *Service) checkContextPin(ctx context.Context, tx pgx.Tx, a row) error {
	pin, ok := s.contextPins[a.Kind]
	if !ok {
		return nil
	}
	if a.ContextDigest == nil || a.TargetID == nil {
		return auth.ReleasedContextMoved()
	}
	pinCtx, err := s.effectContext(ctx, tx, stagedByRow(a))
	if err != nil {
		return err
	}
	current, err := pin(pinCtx, tx, *a.TargetID, a.ProposedChange)
	if err != nil {
		return err
	}
	if current != *a.ContextDigest {
		return auth.ReleasedContextMoved()
	}
	return nil
}

// previewContextPin checks the pin of a row a human is reading. A pending row
// staged before its kind had a pin is pinned now.
func (s *Service) previewContextPin(ctx context.Context, tx pgx.Tx, a row) error {
	if _, ok := s.contextPins[a.Kind]; ok && a.ContextDigest == nil && a.Status == statusPending {
		return s.storeContextDigest(ctx, tx, a)
	}
	return s.checkContextPin(ctx, tx, a)
}

// pinDecidedContext pins the payload an approval releases, inside the decision.
// An edit can change what the payload reads, so an edited row is pinned again.
func (s *Service) pinDecidedContext(ctx context.Context, tx pgx.Tx, id ids.ApprovalID, edited bool) error {
	if len(s.contextPins) == 0 {
		return nil
	}
	a, err := get(ctx, tx, id)
	if err != nil {
		return err
	}
	if _, ok := s.contextPins[a.Kind]; !ok {
		return nil
	}
	if a.ContextDigest != nil && !edited {
		return s.checkContextPin(ctx, tx, a)
	}
	return s.storeContextDigest(ctx, tx, a)
}

func (s *Service) storeContextDigest(ctx context.Context, tx pgx.Tx, a row) error {
	if a.TargetID == nil {
		return auth.ReleasedContextMoved()
	}
	pinCtx, err := s.effectContext(ctx, tx, stagedByRow(a))
	if err != nil {
		return err
	}
	digest, err := s.contextPins[a.Kind](pinCtx, tx, *a.TargetID, a.ProposedChange)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `UPDATE approval SET context_digest = $2 WHERE id = $1`, a.ID, digest)
	return err
}
