// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// AgentForStagedCallTx re-authenticates, on the caller's transaction, the
// agent that will spend a call it staged. That is the passport it staged
// under, or else the newest one minted on the same connection, which is the
// passport a refresh hands it. Both answer to the same liveness rule.
func (s *Service) AgentForStagedCallTx(ctx context.Context, tx pgx.Tx, human ids.UUID, passport ids.PassportID, connection ids.UUID,
) (AgentIdentity, error) {
	a, err := s.authenticateAgentWhere(ctx, tx, agentOfHumanByIDPredicate, passport, human)
	if !errors.Is(err, apperrors.ErrNotFound) || connection.IsZero() {
		return a, err
	}
	return s.authenticateAgentWhere(ctx, tx, agentNewestOnConnectionPredicate, connection, human)
}
