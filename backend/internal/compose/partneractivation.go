// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The cg:partner-activation consumer: the first customer a partner sourced
// and won activates that partner.
//
// The trigger is the event, so every path that wins a deal reaches it. It
// reads the win from the payload, as the commission accrual does. Attribution
// can change after the win, and the payload says what was true at the win.
// Only `sourced` activates. Help on a running deal (`influenced`) never does.
//
// A replay changes nothing, because activation is stamped once. It has its
// own group, so a wedged accrual never holds an activation back.

import (
	"context"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/kernel/events"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// attributionSourced is the deal_partner_attribution_check value that
// activates a partner.
const attributionSourced = "sourced"

// PartnerActivation activates a partner on its first sourced win.
type PartnerActivation struct {
	pool     *pgxpool.Pool
	partners *contacts.Store
	log      *slog.Logger
}

// NewPartnerActivation builds the consumer over the partner store.
func NewPartnerActivation(pool *pgxpool.Pool, partners *contacts.Store, log *slog.Logger) *PartnerActivation {
	return &PartnerActivation{pool: pool, partners: partners, log: log}
}

// HandleEvent routes one envelope. Anything but a sourced win answers nil, so
// the group keeps flowing past the rest of the deal stream.
func (a *PartnerActivation) HandleEvent(ctx context.Context, env events.Envelope) error {
	if env.Entity.ID == ids.Nil || env.Entity.Type != commissionEntityDeal ||
		env.Type != deals.EventDealStageChanged {
		return nil
	}
	moved, err := deals.DecodeStageChanged(env.Payload)
	if err != nil {
		return err
	}
	if moved.ToStatus != dealStatusWon || moved.PartnerCompanyID == nil ||
		moved.PartnerAttribution == nil || *moved.PartnerAttribution != attributionSourced {
		return nil
	}
	ws, err := InstallationDB(a.pool).Workspace(ctx)
	if err != nil {
		return err
	}
	ctx = principal.WithWorkspaceID(ctx, ws.UUID)
	ctx = principal.WithCorrelationID(ctx, env.Trace.CorrelationID)
	ctx = principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalSystem, ID: "system:partner_activation",
		Permissions: principal.Permissions{RowScope: principal.RowScopeAll},
	})
	activated, err := a.partners.ActivatePartnerForWonDeal(ctx,
		ids.From[ids.CompanyKind](*moved.PartnerCompanyID), ids.From[ids.DealKind](env.Entity.ID))
	if err != nil {
		return err
	}
	if activated {
		a.log.InfoContext(ctx, "partner activated by a sourced win",
			"partner", moved.PartnerCompanyID.String(), "deal", env.Entity.ID.String())
	}
	return nil
}
