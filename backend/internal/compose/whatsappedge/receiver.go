// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package whatsappedge is the edge that Meta's WhatsApp Business webhook reaches.
// It answers the subscription check and verifies each delivery's signature.
// It stores every item of a connected number once and queues its ingest job.
// All of that happens in the request, without a call to Meta. Filing the stored
// items is a later job's work.
package whatsappedge

import (
	"context"
	"fmt"
	"log/slog"
	"net/url"

	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/compose/chatedge"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/capture/whatsapp"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// Endpoints answers the live numbers with the secrets their calls are checked by.
type Endpoints interface {
	LiveEndpoints(ctx context.Context) ([]capture.WhatsAppEndpoint, error)
}

// TxInserter queues a job in the caller's transaction.
type TxInserter = chatedge.TxInserter

// Receiver implements capture.WhatsAppEventReceiver.
type Receiver struct {
	Endpoints Endpoints
	DB        *database.DB
	Inserter  TxInserter
	// Job builds the ingest job of one stored item.
	Job         func(workspace, connection, rawCapture ids.UUID) river.JobArgs
	MaxAttempts int
	Log         *slog.Logger
}

// Challenge answers Meta's subscription check. Every connected number's verify
// token is compared, so the time taken does not say which one matched.
func (r *Receiver) Challenge(ctx context.Context, query url.Values) (string, error) {
	endpoints, err := r.Endpoints.LiveEndpoints(ctx)
	if err != nil {
		return "", fmt.Errorf("%w: %w", capture.ErrWhatsAppEventsUnavailable, err)
	}
	var challenge string
	matched := false
	for _, endpoint := range endpoints {
		if answer, err := whatsapp.VerifyChallenge(query, endpoint.VerifyToken); err == nil {
			challenge, matched = answer, true
		}
	}
	if !matched {
		return "", capture.ErrWhatsAppEventRejected
	}
	return challenge, nil
}

// Receive verifies one delivery and stores its items.
//
// The app secret is per connection, because two numbers may sit under two Meta
// apps. The body is checked against every connected number's secret. A number
// whose secret verifies is trusted for its own items only. Numbers that share an
// app verify together. A body that no secret signs is refused before decoding.
func (r *Receiver) Receive(ctx context.Context, body []byte, signature string) (crmcontracts.WhatsAppWebhookReceipt, error) {
	var none crmcontracts.WhatsAppWebhookReceipt
	endpoints, err := r.Endpoints.LiveEndpoints(ctx)
	if err != nil {
		return none, fmt.Errorf("%w: %w", capture.ErrWhatsAppEventsUnavailable, err)
	}
	trusted := map[string]number{}
	for _, endpoint := range endpoints {
		if whatsapp.VerifySignature(endpoint.AppSecret, body, signature) == nil {
			trusted[endpoint.PhoneNumberID] = number{connection: endpoint.ConnectionID, waba: endpoint.WABAID}
		}
	}
	if len(trusted) == 0 {
		return none, capture.ErrWhatsAppEventRejected
	}
	split, err := whatsapp.SplitItems(body)
	if err != nil {
		// A retry cannot fix a signed body that is no payload. It counts as one
		// skipped item and is answered like any other delivery.
		r.Log.WarnContext(ctx, "whatsapp: a signed delivery is not a business account payload")
		split = whatsapp.Split{Errors: []whatsapp.ItemError{{Path: "body", Err: err}}}
	}
	keep, foreign := partition(split.Items, trusted)
	if foreign > 0 {
		r.Log.InfoContext(ctx, "whatsapp: items that are no message of a number this delivery's secret covers were not stored", "items", foreign)
	}
	if len(split.Errors) > 0 {
		r.Log.WarnContext(ctx, "whatsapp: items that could not be decoded were not stored", "items", len(split.Errors))
	}
	stored, dropped, err := r.store(ctx, keep, trusted)
	if err != nil {
		return none, fmt.Errorf("%w: %w", capture.ErrWhatsAppEventsUnavailable, err)
	}
	if dropped > 0 {
		r.Log.InfoContext(ctx, "whatsapp: items of erased customers or of a number no longer connected were not stored", "items", dropped)
	}
	return crmcontracts.WhatsAppWebhookReceipt{
		Stored:  stored,
		Skipped: foreign + dropped + len(split.Errors) + len(split.UnhandledFields),
	}, nil
}

// number is a connected number whose app secret verified a delivery.
type number struct {
	connection ids.UUID
	waba       string
}

// partition splits items into those that are messages of a trusted number and the
// count of the rest. The entry's business account must be the one the number was
// connected under, so a delivery cannot name a number under another account. A
// delivery status is no message, so it is never stored.
func partition(items []whatsapp.Item, trusted map[string]number) ([]whatsapp.Item, int) {
	var keep []whatsapp.Item
	for _, item := range items {
		if n, ok := trusted[item.PhoneNumberID]; ok && n.waba == item.WABAID && !item.Status {
			keep = append(keep, item)
		}
	}
	return keep, len(items) - len(keep)
}

// store hands the items to the shared inbox. It writes each item's raw row and
// ingest job in one transaction, and returns the counts stored and dropped.
func (r *Receiver) store(ctx context.Context, items []whatsapp.Item, trusted map[string]number) (stored, dropped int, err error) {
	messages := make([]chatedge.Message, 0, len(items))
	for _, item := range items {
		messages = append(messages, chatedge.Message{
			Connection: trusted[item.PhoneNumberID].connection, Customers: customers(item),
			SourceSystem: whatsapp.Provider, SourceID: whatsapp.RawSourceID(item.PhoneNumberID, item.Key), Payload: item.Body,
		})
	}
	inbox := chatedge.Inbox{DB: r.DB, Inserter: r.Inserter, Job: r.Job, MaxAttempts: r.MaxAttempts}
	return inbox.Store(ctx, messages)
}

func customers(item whatsapp.Item) []connector.ChannelIdentity {
	out := make([]connector.ChannelIdentity, 0, len(item.Identities))
	for _, id := range item.Identities {
		out = append(out, connector.ChannelIdentity{Provider: whatsapp.Provider, AccountScope: id.Scope, ChannelUserID: id.UserID})
	}
	return out
}
