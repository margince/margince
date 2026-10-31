// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package approvals

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// ContextPin digests the facts outside the target row that a kind's reviewed
// outcome reads, such as the company a promotion resolves. The version pin
// cannot see them, because changing them never bumps the target.
//
// It must not depend on who asks. Staging runs as the stager, the preview as
// the approver and redemption as the agent, and all three must agree.
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
	return pin(ctx, tx, in.TargetID, in.ProposedChange)
}

// checkContextPin refuses an approval whose context moved since staging. A row
// with no digest was staged before its kind had a pin, so it refuses too.
func (s *Service) checkContextPin(ctx context.Context, tx pgx.Tx, a row) error {
	pin, ok := s.contextPins[a.Kind]
	if !ok {
		return nil
	}
	if a.ContextDigest == nil || a.TargetID == nil {
		return auth.ReleasedContextMoved()
	}
	current, err := pin(ctx, tx, *a.TargetID, a.ProposedChange)
	if err != nil {
		return err
	}
	if current != *a.ContextDigest {
		return auth.ReleasedContextMoved()
	}
	return nil
}
