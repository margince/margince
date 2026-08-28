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
