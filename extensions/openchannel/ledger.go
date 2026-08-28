// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package openchannel

// What this unit records about its OWN endpoint rows: the ledger row, and the
// event other listeners hear.
//
// EVERY state change on an endpoint is recorded — it is opened, its secret is
// replaced, it is paused or resumed, its outward address moves — because each
// one changes who can reach this installation without a session, or where this
// installation talks back to. That is precisely the class of fact somebody asks
// about after the event.
//
// The one write that is NOT recorded is the anonymous inbound path's touch on
// the traffic counters, and that exemption is stated where it is taken
// (inbound.go): a counter moving is not a decision anybody made, and a ledger
// row per arriving request would turn the audit trail into a traffic log that
// buries the decisions in it.

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/margince/margince/backend/pkg/extension"
)

// The verbs this unit publishes about its own endpoint rows. The type on the
// bus is `ext_openchannel.<verb>` — the core prefixes the namespace, so these
// are verbs and not types.
const (
	eventOpened        = "opened"
	eventSecretMinted  = "secret_minted"
	eventEnabled       = "enabled"
	eventDisabled      = "disabled"
	eventURLRegistered = "url_registered"
	// The two facts about a QUEUED request that somebody asks about afterwards:
	// this installation accepted a message and will now never act on it, and a
	// message it did act on is no longer on any timeline. Neither is derivable
	// from the endpoint's counters, which is why they are the queue table's own
	// verbs rather than more of the endpoint's.
	eventRequestParked    = "request_parked"
	eventRequestWithdrawn = "request_withdrawn"
)

// recordEndpoint writes the ledger row and the event for one endpoint write, in
// the caller's transaction.
//
// before and after are the row's own images, which every statement here has in
// hand: each RETURNs the row it wrote, so what is recorded is what the database
// holds rather than what this code believed it sent.
func recordEndpoint(ctx context.Context, tx extension.Tx, action extension.AuditAction, verb string,
	before, after *endpoint,
) error {
	subject := after
	if subject == nil {
		subject = before
	}
	if subject == nil {
		return fmt.Errorf("openchannel: recording a %s needs one image — the row's id comes from whichever side of the write it has", verb)
	}
	beforeImage, err := endpointImage(before)
	if err != nil {
		return err
	}
	afterImage, err := endpointImage(after)
	if err != nil {
		return err
	}
	payload, err := json.Marshal(struct {
		Slug    string `json:"slug"`
		Enabled bool   `json:"enabled"`
	}{Slug: subject.Slug, Enabled: subject.Enabled})
	if err != nil {
		return err
	}
	return tx.Record(ctx,
		extension.Change{
			Action: action,
			Entity: endpointEntity,
			ID:     subject.ID,
			Before: beforeImage,
			After:  afterImage,
		},
		extension.Event{Verb: verb, Payload: payload})
}

// endpointImage renders one side of a change, or nothing at all. A missing
// image is nil rather than `null`: an open has no before, and the ledger's own
// reading of "there was no such state" is an absent column rather than a JSON
// null sitting in one.
//
// THE IMAGE CARRIES NO SECRET, which is true by construction rather than by
// filtering here: the row has no secret column. The signing material lives in
// the unit's sealed secret namespace, so an audit trail of endpoints cannot
// become a place credentials are kept in the clear.
func endpointImage(e *endpoint) (json.RawMessage, error) {
	if e == nil {
		return nil, nil
	}
	return json.Marshal(e)
}

// recordParked writes the ledger row for a request the drain has stopped
// attempting, in the caller's transaction.
//
// WHY A QUEUE ROW EARNS ONE WHEN AN ARRIVAL DOES NOT. An arrival is not a
// decision anybody made — the file header says why recording one per anonymous
// request would bury the decisions. Parking is: this installation accepted a
// message, told the sender so, and has now decided that nothing further will be
// done with it. "This connector has been parking every request since Tuesday" is
// a question somebody has to be able to answer.
//
// It carries a DETAIL and no images. The images would be the row's own fields —
// the state and the attempt count — which the audit action and this detail
// already say, while what a reader actually wants is why it stopped, and that is
// context about the write rather than a field of the record.
func recordParked(ctx context.Context, tx extension.Tx, req queued, class extension.FailureClass) error {
	detail, err := json.Marshal(struct {
		Class    string `json:"class"`
		Attempts int    `json:"attempts"`
	}{Class: class.Class, Attempts: req.attempts + 1})
	if err != nil {
		return err
	}
	return tx.Record(ctx,
		extension.Change{
			Action: extension.AuditArchive,
			Entity: inboundEntity,
			ID:     req.id,
			Detail: detail,
		},
		extension.Event{Verb: eventRequestParked, Payload: detail})
}

// recordWithdrawn writes the ledger row for a request whose timeline entry has
// been archived, in the caller's transaction.
//
// The CAUSE goes in the detail rather than in an image: the row's own fields do
// not record why they changed, and a reader asking who withdrew this is asking
// about the event, not about the row.
func recordWithdrawn(ctx context.Context, tx extension.Tx, requestID string, d extension.Delivery) error {
	detail, err := json.Marshal(struct {
		Cause      string `json:"cause"`
		ActivityID string `json:"activity_id"`
		EventID    string `json:"event_id"`
	}{Cause: d.Type, ActivityID: d.Entity.ID, EventID: d.EventID})
	if err != nil {
		return err
	}
	return tx.Record(ctx,
		extension.Change{
			Action: extension.AuditArchive,
			Entity: inboundEntity,
			ID:     requestID,
			Detail: detail,
		},
		extension.Event{Verb: eventRequestWithdrawn, Payload: detail})
}
