// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The cg:approval-notice-retract consumer: a decided card takes back the lines
// it put in other seats' queues.
//
// ApprovalNotify announces a staged proposal to EVERY seat that could decide
// it, which is the honest fan-out — any of them may answer. One of them then
// does, and the rest keep a line asking them to decide something already
// settled: a badge that will not clear, and a card that refuses when opened.
//
// It lives here because the question crosses two modules: approvals owns the
// verdict, notices owns the line, and neither imports the other.
//
// A CONSUMER rather than a hook on the decide path, for the reason the stage
// outcome consumer gives: expiry is a real decision written by a sweep and not
// by any human, and it reaches the same approval.decided event. A hook on the
// human path would clear the answered cards and leave every expired one
// standing, which is the case that most needs clearing.
//
// EVERY copy goes, the decider's own included. The line asked for a decision
// that has been made, and singling out the seat that made it would need this
// consumer to re-derive who that was from a payload that does not say.
//
// Idempotency is the retraction's own predicate: Retract matches only rows not
// already retracted, so a redelivered verdict moves no instant. events.Dedupe
// sits in front of that as a cache, never as the guarantee.

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/modules/notices"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/events"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// approvalNoticeRetractActor names this consumer in the audit rows it causes.
const approvalNoticeRetractActor = "system:approval-notice-retract"

// ApprovalNoticeRetract withdraws the pending-decision lines of a settled card.
type ApprovalNoticeRetract struct {
	db       *database.DB
	notices  *notices.Store
	identity *identity.Service
	log      *slog.Logger
}

// NewApprovalNoticeRetract builds the consumer over the installation's handle.
func NewApprovalNoticeRetract(
	pool *pgxpool.Pool, store *notices.Store, ident *identity.Service, log *slog.Logger,
) *ApprovalNoticeRetract {
	return &ApprovalNoticeRetract{db: database.Bind(pool, ident.InstallationWorkspace), notices: store, identity: ident, log: log}
}

// HandleEvent routes one envelope. Anything that is not an approval decision
// answers nil, so the consumer group keeps flowing.
func (r *ApprovalNoticeRetract) HandleEvent(ctx context.Context, env events.Envelope) error {
	if env.Type != string(crmcontracts.ApprovalDecided) || env.Entity.ID == ids.Nil {
		return nil
	}
	// The envelope carries no tenant, and the subscriber binds none: without
	// this the workspace resolves to zero and every write is refused.
	ws, err := r.identity.InstallationWorkspace(ctx)
	if err != nil {
		return err
	}
	ctx = principal.WithWorkspaceID(ctx, ws.UUID)
	ctx = principal.WithCorrelationID(ctx, env.EventID)
	ctx = principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalSystem, ID: approvalNoticeRetractActor,
	})
	return r.db.Tx(ctx, func(tx pgx.Tx) error {
		taken, err := r.notices.Retract(ctx, tx, approvalNoticeDedupeKey(env.Entity.ID))
		if err != nil {
			return fmt.Errorf("approval notice retract: %w", err)
		}
		if taken > 0 {
			r.log.InfoContext(ctx, "took back pending-decision notices for a settled approval",
				"approval_id", env.Entity.ID, "notices", taken)
		}
		return nil
	})
}
