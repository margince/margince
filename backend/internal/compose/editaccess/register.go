// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package editaccess

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Register puts the accept, its precheck and the decline on the deciding
// approvals service.
func Register(svc *approvals.Service, db *database.DB, grants *identity.Service) {
	svc.WithEffect(Kind, acceptEffect(svc, grants))
	svc.WithPrecheck(Kind, precheck(db))
	svc.WithDeclinedEffect(Kind, func(ctx context.Context, tx pgx.Tx, approvalID ids.ApprovalID, proposedChange json.RawMessage) error {
		return tellOutcome(ctx, tx, approvalID, proposedChange, declined)
	})
}

// RegisterExpiry puts the expiry line on the service the clock's sweep runs.
func RegisterExpiry(svc *approvals.Service) *approvals.Service {
	return svc.WithExpiredEffect(Kind, func(ctx context.Context, tx pgx.Tx, approvalID ids.ApprovalID, proposedChange json.RawMessage) error {
		return tellOutcome(ctx, tx, approvalID, proposedChange, expired)
	})
}
