// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package openchannel

// What arrived on the caller's own edge, newest first.
//
// THE BODY IS NOT IN THE ANSWER, and its absence is a decision rather than an
// omission. A queue entry's body is bytes an anonymous remote party chose; the
// signature says the sender holds the endpoint's secret and says nothing about
// what they put in it. This listing exists to show WHAT ARRIVED AND WHAT
// BECAME OF IT — when, how often it has been attempted, and why it stopped —
// which needs the size of the payload and not the payload. Anything that acts
// on the bytes reads them from the queue under the owner's own authority.

import (
	"context"
	"encoding/json"
	"time"

	"github.com/margince/margince/backend/pkg/extension"
)

// inboundPageSize is how many entries one listing answers. A page rather than
// the whole queue because the queue is bounded at maxPendingInbound and a
// screen renders a screenful; the number matches the batch the core's own
// recovery sweep takes, so a reader comparing the two is comparing like with
// like.
const inboundPageSize = 100

// inboundColumns is the listing's projection. It deliberately does not include
// the body — see the file comment.
const inboundColumns = `id::text, nonce, state, attempts, coalesce(last_error_class, ''),
	octet_length(body), sent_at, received_at`

// inboundEntry is one received request as a screen sees it.
type inboundEntry struct {
	ID    string `json:"id"`
	Nonce string `json:"nonce"`
	// State is where this entry is in the queue: waiting, acted on, or stopped.
	State    string `json:"state"`
	Attempts int    `json:"attempts"`
	// LastErrorClass is a class this unit chose, never a remote party's own
	// message: it is rendered, and a stranger's prose is not this
	// installation's to display.
	LastErrorClass string `json:"last_error_class,omitempty"`
	// BodyBytes is how large the payload is. It is what a reader can act on
	// without being handed the payload itself.
	BodyBytes int `json:"body_bytes"`
	// SentAt is the instant the sender signed; ReceivedAt is this
	// installation's own clock. Both, because one drifting against the other
	// is a thing a reader can see rather than infer.
	SentAt     string `json:"sent_at"`
	ReceivedAt string `json:"received_at"`
}

// listInbound answers what arrived on the caller's own endpoint.
//
// Their own, and not an endpoint named in the arguments: the entries say who
// has been messaging this member and how often, which is not a fact this unit
// hands to a colleague because they hold the same RBAC object.
func listInbound(ctx context.Context, rt extension.Runtime, in json.RawMessage) (json.RawMessage, error) {
	if _, err := extension.DecodeArgs[struct{}](in); err != nil {
		return nil, err
	}
	member, err := callingMember(rt, "reading a queue")
	if err != nil {
		return nil, err
	}
	entries := make([]inboundEntry, 0, inboundPageSize)
	err = rt.Tx(ctx, func(ctx context.Context, tx extension.Tx) error {
		mine, err := endpointOf(ctx, tx, member)
		if err != nil {
			return err
		}
		if mine == nil {
			// No endpoint is no entries, not a failure: not having opened one
			// yet is the ordinary state of this screen.
			return nil
		}
		rows, err := tx.Query(ctx,
			`SELECT `+inboundColumns+` FROM `+inboundTable+`
			 WHERE endpoint_id = $1::uuid ORDER BY received_at DESC LIMIT $2`,
			mine.ID, inboundPageSize)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			entry, err := scanInboundEntry(rows.Scan)
			if err != nil {
				return err
			}
			entries = append(entries, entry)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return json.Marshal(struct {
		Entries []inboundEntry `json:"entries"`
	}{Entries: entries})
}

// scanInboundEntry reads inboundColumns off one row. The two timestamps are
// scanned as times and rendered afterwards, for the reason scanEndpoint gives.
func scanInboundEntry(scan func(...any) error) (inboundEntry, error) {
	var (
		e                  inboundEntry
		sentAt, receivedAt time.Time
	)
	err := scan(&e.ID, &e.Nonce, &e.State, &e.Attempts, &e.LastErrorClass,
		&e.BodyBytes, &sentAt, &receivedAt)
	if err != nil {
		return inboundEntry{}, err
	}
	e.SentAt = sentAt.UTC().Format(time.RFC3339)
	e.ReceivedAt = receivedAt.UTC().Format(time.RFC3339)
	return e, nil
}
