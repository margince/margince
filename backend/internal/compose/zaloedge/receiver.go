// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package zaloedge is the edge that Zalo's Official Account webhook reaches.
// It verifies each event's signature, stores a connected account's message once
// and queues its ingest job. All of that happens in the request, without a call
// to Zalo, because Zalo expects an answer within two seconds. Filing the stored
// message is the ingest worker's work.
package zaloedge

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/compose/chatedge"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/capture/zalo"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// Endpoints answers the live accounts with the keys their events are checked by.
type Endpoints interface {
	LiveEndpoints(ctx context.Context) ([]capture.ZaloEndpoint, error)
}

// Receiver implements capture.ZaloEventReceiver.
type Receiver struct {
	Endpoints Endpoints
	DB        *database.DB
	Inserter  chatedge.TxInserter
	// Job builds the ingest job of one stored message.
	Job         func(workspace, connection, rawCapture ids.UUID) river.JobArgs
	MaxAttempts int
	Log         *slog.Logger
}

// Receive verifies one event and stores it when it is a message.
//
// The secret key is per connection. The body is checked against every connected
// account's key. An account whose key verifies is trusted for its own OA id and
// app id only. A body that no key signs is refused before any field beyond the
// two the signature covers is read.
func (r *Receiver) Receive(ctx context.Context, body []byte, signature string) (crmcontracts.ZaloWebhookReceipt, error) {
	var none crmcontracts.ZaloWebhookReceipt
	endpoints, err := r.Endpoints.LiveEndpoints(ctx)
	if err != nil {
		return none, fmt.Errorf("%w: %w", capture.ErrChannelEventsUnavailable, err)
	}
	var verified []capture.ZaloEndpoint
	for _, endpoint := range endpoints {
		if zalo.VerifySignature(endpoint.OASecretKey, signature, body) == nil {
			verified = append(verified, endpoint)
		}
	}
	if len(verified) == 0 {
		return none, capture.ErrChannelEventRejected
	}
	skipped := crmcontracts.ZaloWebhookReceipt{Skipped: 1}
	ev, err := zalo.Decode(body)
	if err != nil {
		// A retry cannot fix a signed body that is no event, so it is skipped and answered 200.
		r.Log.WarnContext(ctx, "zalo: a signed delivery is not a decodable event", "err", err)
		return skipped, nil
	}
	if ev.Kind != zalo.KindMessage || ev.Message == nil || ev.Message.ID == "" {
		return skipped, nil
	}
	conn, ok := owner(verified, ev)
	if !ok {
		r.Log.InfoContext(ctx, "zalo: an event for an account this delivery's secret does not cover was not stored")
		return skipped, nil
	}
	inbox := chatedge.Inbox{DB: r.DB, Inserter: r.Inserter, Job: r.Job, MaxAttempts: r.MaxAttempts}
	stored, _, err := inbox.Store(ctx, []chatedge.Message{{
		Connection: conn, Customers: []connector.ChannelIdentity{zalo.CustomerIdentity(ev)},
		SourceSystem: zalo.Provider, SourceID: zalo.RawSourceID(ev.OAID, ev.Message.ID), Payload: body,
	}})
	if err != nil {
		return none, fmt.Errorf("%w: %w", capture.ErrChannelEventsUnavailable, err)
	}
	if stored == 0 {
		r.Log.InfoContext(ctx, "zalo: a message of an erased customer or of an account no longer connected was not stored")
		return skipped, nil
	}
	return crmcontracts.ZaloWebhookReceipt{Stored: 1}, nil
}

// owner finds the verified connection the event belongs to: the OA id and the
// app id must both be the connection's own.
func owner(verified []capture.ZaloEndpoint, ev zalo.Event) (ids.UUID, bool) {
	for _, endpoint := range verified {
		if endpoint.OAID == ev.OAID && endpoint.AppID == ev.AppID {
			return endpoint.ConnectionID, true
		}
	}
	return ids.Nil, false
}
