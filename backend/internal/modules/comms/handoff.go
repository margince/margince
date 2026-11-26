// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package comms

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// ProviderHandoffBudget leaves time for a waiting stop to commit within the
// HTTP response deadline. Credential refresh and receipt reads use the job budget.
const ProviderHandoffBudget = 20 * time.Second

type handoffDisposition struct {
	outcome Outcome
	wait    time.Duration
	cause   error
}

func (e *handoffDisposition) Error() string { return "comms: provider handoff was not authorized" }
func (e *handoffDisposition) Unwrap() error { return e.cause }

func (d *Dispatcher) handToProvider(ctx context.Context, del Delivery, seam sendSeam, issued commsauthz.TransmitTicket, files []connector.OutboundFile) (connector.SendReceipt, Outcome, time.Duration, error) {
	receipt, err := seam.transmit(ctx, files, func(wire context.Context, send func(context.Context) (connector.SendReceipt, error)) (connector.SendReceipt, error) {
		receipt, outcome, wait, err := d.authorizedHandoff(wire, del, seam, issued, send)
		if outcome != outcomeUndecided {
			return receipt, &handoffDisposition{outcome: outcome, wait: wait, cause: err}
		}
		return receipt, err
	})
	if stopped, ok := errors.AsType[*handoffDisposition](err); ok {
		return receipt, stopped.outcome, stopped.wait, stopped.cause
	}
	return receipt, outcomeUndecided, 0, err
}

// Release precedes classification and receipt writers: a provider failure may
// itself record a stop, and must not wait on its own permit.
func (d *Dispatcher) authorizedHandoff(ctx context.Context, del Delivery, seam sendSeam, issued commsauthz.TransmitTicket, send func(context.Context) (connector.SendReceipt, error)) (connector.SendReceipt, Outcome, time.Duration, error) {
	var receipt connector.SendReceipt
	bounded, cancel := context.WithTimeout(ctx, ProviderHandoffBudget)
	defer cancel()
	permit, err := d.consent.AcquireTransmitPermit(bounded, transmitRequestFor(del), issued)
	if err != nil {
		outcome, wait, err := d.retry(ctx, del.ID, err)
		return receipt, outcome, wait, err
	}
	if permit == nil {
		outcome, wait, err := d.postpone(ctx, del.ID, "A communication check is finishing; this message will be checked again.", time.Second)
		return receipt, outcome, wait, err
	}
	defer permit.Close(ctx)
	if outcome, wait, err := d.gateRecheckedTicket(bounded, del, issued, permit.Ticket()); outcome != outcomeUndecided || err != nil {
		return receipt, outcome, wait, err
	}
	if err := permit.Ready(bounded); err != nil {
		outcome, wait, err := d.retry(ctx, del.ID, err)
		return receipt, outcome, wait, err
	}
	if outcome, wait, err := d.guardAtMostOnce(bounded, del, seam); outcome != outcomeUndecided || err != nil {
		return receipt, outcome, wait, err
	}
	receipt, err = send(bounded)
	if health := permit.Ready(bounded); health != nil {
		if bounded.Err() == nil {
			slog.WarnContext(ctx, "provider handoff could not confirm its stop-ordering lease", "delivery_id", del.ID, "err", health)
		}
		if err != nil {
			err = errors.Join(err, connector.ErrSendOutcomeUnknown, health)
		}
	}
	if err != nil && bounded.Err() != nil {
		err = errors.Join(err, connector.ErrSendOutcomeUnknown)
	}
	return receipt, outcomeUndecided, 0, err
}
