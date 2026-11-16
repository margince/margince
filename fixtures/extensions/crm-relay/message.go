// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package relay

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/margince/margince/backend/pkg/extension"
)

// ingressSystem is the declared source a landed test message names.
const ingressSystem = "crm-relay"

// activityKindEmail is the core timeline kind of a captured email.
const activityKindEmail = "email"

// landBatch bounds how many posted messages one tick lands.
const landBatch = 20

// mailParty is one end of a posted test message.
type mailParty struct {
	Email string `json:"email"`
	Name  string `json:"name"`
}

// testMessage is the email-shaped document a developer posts.
type testMessage struct {
	MessageID  string    `json:"message_id"`
	Subject    string    `json:"subject"`
	Body       string    `json:"body"`
	OccurredAt time.Time `json:"occurred_at"`
	From       mailParty `json:"from"`
	To         mailParty `json:"to"`
}

func (m testMessage) check() error {
	switch {
	case strings.TrimSpace(m.MessageID) == "" || len(m.MessageID) > 200:
		return fmt.Errorf("%w: message_id is 1 to 200 characters", extension.ErrInvalid)
	case !strings.Contains(m.From.Email, "@"):
		return fmt.Errorf("%w: from.email is an address", extension.ErrInvalid)
	}
	return nil
}

// postMessage keeps one test message for the tick to land, once per message_id.
func postMessage(ctx context.Context, rt extension.Runtime, args json.RawMessage) (json.RawMessage, error) {
	owner := rt.Caller().UserID
	if owner == "" {
		return nil, fmt.Errorf("%w: a test message is posted by a signed-in member, whom the tick lands it on", extension.ErrForbidden)
	}
	var msg testMessage
	if err := json.Unmarshal(args, &msg); err != nil {
		return nil, fmt.Errorf("%w: the body is not a test message", extension.ErrInvalid)
	}
	if err := msg.check(); err != nil {
		return nil, err
	}
	var queued int64
	err := rt.Tx(ctx, func(ctx context.Context, tx extension.Tx) error {
		var err error
		queued, err = tx.Exec(ctx, `INSERT INTO ext.ext_crm_relay_message (owner_id, message_id, document)
			VALUES ($1, $2, $3) ON CONFLICT (owner_id, message_id) DO NOTHING`, owner, msg.MessageID, args)
		return err
	})
	if err != nil {
		return nil, err
	}
	return json.Marshal(map[string]bool{"queued": queued == 1})
}

type postedMessage struct {
	id, owner string
	document  []byte
	postedAt  time.Time
}

// landPosted lands every waiting message, each in its own ingest, and reports
// how many it handled. It reads, closes the transaction, then ingests.
func landPosted(ctx context.Context, rt extension.Runtime) (int, error) {
	var waiting []postedMessage
	err := rt.Tx(ctx, func(ctx context.Context, tx extension.Tx) error {
		rows, err := tx.Query(ctx, `SELECT id::text, owner_id::text, document, posted_at FROM ext.ext_crm_relay_message
			WHERE landed_at IS NULL ORDER BY posted_at, id LIMIT $1`, landBatch)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var p postedMessage
			if err := rows.Scan(&p.id, &p.owner, &p.document, &p.postedAt); err != nil {
				return err
			}
			waiting = append(waiting, p)
		}
		return rows.Err()
	})
	if err != nil {
		return 0, err
	}
	for _, p := range waiting {
		if err := landOne(ctx, rt, p); err != nil {
			return 0, err
		}
	}
	return len(waiting), nil
}

// landOne ingests one message and marks it handled whatever the core decided.
// A refused test message is the developer's to post again, not the tick's to retry.
func landOne(ctx context.Context, rt extension.Runtime, p postedMessage) error {
	var msg testMessage
	if err := json.Unmarshal(p.document, &msg); err != nil {
		return err
	}
	result, err := rt.Ingest(ctx, extension.UserID(p.owner), recordFor(msg, p.postedAt, p.document))
	if err != nil {
		return err
	}
	var activity *string
	if result.Disposition == extension.DispositionAccepted && result.Ref.ID != "" {
		activity = &result.Ref.ID
	}
	return rt.Tx(ctx, func(ctx context.Context, tx extension.Tx) error {
		_, err := tx.Exec(ctx, `UPDATE ext.ext_crm_relay_message
			SET landed_at = now(), activity_id = $2::uuid, document = NULL WHERE id = $1::uuid`, p.id, activity)
		return err
	})
}

// recordFor builds the email-shaped record: an inbound email from the sender to the member.
func recordFor(msg testMessage, postedAt time.Time, raw []byte) extension.Record {
	occurred := msg.OccurredAt
	if occurred.IsZero() {
		occurred = postedAt
	}
	addresses := []string{strings.TrimSpace(msg.From.Email)}
	if to := strings.TrimSpace(msg.To.Email); to != "" {
		addresses = append(addresses, to)
	}
	return extension.Record{
		System: ingressSystem,
		Key:    msg.MessageID,
		Activity: extension.ActivityFields{
			Kind:       activityKindEmail,
			Subject:    strings.TrimSpace(msg.Subject),
			Body:       strings.TrimSpace(msg.Body),
			OccurredAt: occurred,
			Direction:  extension.DirectionInbound,
		},
		ThreadKey: ingressSystem + ":" + msg.MessageID,
		Counterparty: extension.Counterparty{
			Email:       strings.TrimSpace(msg.From.Email),
			DisplayName: strings.TrimSpace(msg.From.Name),
			Domain:      domainOf(msg.From.Email),
			Direction:   extension.DirectionInbound,
		},
		Addresses: addresses,
		Raw:       raw,
	}
}

func domainOf(email string) string {
	at := strings.LastIndex(email, "@")
	if at < 0 {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(email[at+1:]))
}
