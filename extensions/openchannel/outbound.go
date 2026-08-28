// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package openchannel

// The evidence of what this connector posted outward, and the listing a member
// reads it back through.
//
// IT IS A LEDGER AND NOT A QUEUE. Nothing here decides when a message is sent or
// how often it is tried — that is the product's staged delivery, upstream — so
// there is no state machine in this table and no row waiting to be picked up. A
// row appears when an attempt is about to leave and is completed when it has, and
// what it buys is that "my system never received that message" is a question the
// member can answer from their own screen.
//
// WHAT IT DOES NOT HOLD is the message body. The member wrote it and the CRM
// already keeps it on the record it was sent from; a second copy here would be a
// second place to erase it from.

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/margince/margince/backend/pkg/extension"
)

// What became of one attempt. Three values because the third is a different kind
// of answer rather than a degree of the second: a receiver that said no is one
// this installation heard from, and an unanswered request is one no later attempt
// can ever settle.
const (
	outcomeSent     = "sent"
	outcomeRefused  = "refused"
	outcomeUnknown  = "unknown"
	outboundPerPage = 100
)

// outboundColumns is the listing's projection, and it is also the order
// scanOutboundAttempt reads.
const outboundColumns = `id::text, delivery_key, attempt, recipient, outcome,
	coalesce(error_class, ''), created_at`

// outboundAttempt is one recorded attempt as a screen sees it.
type outboundAttempt struct {
	ID string `json:"id"`
	// DeliveryKey is the product's own id for the delivery, which is what makes
	// several attempts of one message recognisable as one message.
	DeliveryKey string `json:"delivery_key"`
	Attempt     int    `json:"attempt"`
	// Recipient is the account this connector addressed, as the CRM resolved it.
	Recipient string `json:"recipient"`
	Outcome   string `json:"outcome"`
	// ErrorClass is a class this unit chose, never the receiver's own message:
	// the address belongs to a member's own system, and what it says about
	// itself is not this installation's to render.
	ErrorClass string `json:"error_class,omitempty"`
	CreatedAt  string `json:"created_at"`
}

// outcomeOf reads a post's result as the column's vocabulary.
func outcomeOf(err error) string {
	switch {
	case err == nil:
		return outcomeSent
	case errors.Is(err, errBlocked):
		// Definite: the guard runs before a connection exists, so nothing left.
		return outcomeRefused
	case errors.Is(err, errUnanswered):
		return outcomeUnknown
	default:
		return outcomeRefused
	}
}

// attemptClass names a post's failure in the unit's declared vocabulary, or
// answers the zero class for one that succeeded — which the write below stores as
// no class at all rather than as an empty token.
func attemptClass(err error) extension.FailureClass {
	switch {
	case err == nil:
		return extension.FailureClass{}
	case errors.Is(err, errBlocked):
		return classDeliveryBlocked
	case errors.Is(err, errUnanswered):
		return classDeliveryUnanswered
	default:
		return classDeliveryRefused
	}
}

// recordAttempt writes what this attempt currently is, in a transaction of its
// own.
//
// UPSERT ON THE ATTEMPT, so the same attempt recorded twice is one row. That is
// not only the in-flight row being completed: a worker killed between the two
// writes leaves an `unknown` row, and the retry of that same attempt number is
// the same attempt being made again rather than a second message.
//
// The endpoint's counters move HERE, in the same transaction as the row that
// justifies them, so a screen never shows a send the ledger does not hold. They
// move only for an accepted one: a counter that included refusals would say a
// member's connector is busier than it is.
func recordAttempt(ctx context.Context, rt extension.Runtime, t transmission, outcome string, class extension.FailureClass) error {
	return rt.Tx(ctx, func(ctx context.Context, tx extension.Tx) error {
		if _, err := tx.Exec(ctx,
			`INSERT INTO `+outboundTable+`
			   (endpoint_id, user_id, delivery_key, attempt, recipient, outcome, error_class)
			 VALUES ($1::uuid, $2::uuid, $3, $4, $5, $6, nullif($7, ''))
			 ON CONFLICT (endpoint_id, delivery_key, attempt)
			 DO UPDATE SET outcome = excluded.outcome, error_class = excluded.error_class`,
			t.target.ID, t.target.UserID, t.msg.IdempotencyKey, t.msg.Attempt,
			t.msg.Recipient.ChannelUserID, outcome, class.Class); err != nil {
			return err
		}
		if outcome != outcomeSent {
			return nil
		}
		// Deliberately not a version bump and not a ledger row, for the reason
		// the inbound counters' touch is neither: it moves traffic columns and
		// nothing a member decided, and the decision to send this message was
		// recorded by the product where somebody made it.
		_, err := tx.Exec(ctx,
			`UPDATE `+endpointTable+` SET outbound_sent = outbound_sent + 1,
			        last_outbound_at = now(), updated_at = now()
			  WHERE id = $1::uuid`, t.target.ID)
		return err
	})
}

// listOutbound answers what this connector posted for the CALLER's own endpoint,
// newest first.
//
// Their own, and not an endpoint named in the arguments: the entries say who this
// member has been messaging and when, which is not a fact this unit hands to a
// colleague because they hold the same RBAC object.
func listOutbound(ctx context.Context, rt extension.Runtime, in json.RawMessage) (json.RawMessage, error) {
	if _, err := extension.DecodeArgs[struct{}](in); err != nil {
		return nil, err
	}
	member, err := callingMember(rt, "reading sent messages")
	if err != nil {
		return nil, err
	}
	attempts := make([]outboundAttempt, 0, outboundPerPage)
	err = rt.Tx(ctx, func(ctx context.Context, tx extension.Tx) error {
		mine, err := endpointOf(ctx, tx, member)
		if err != nil || mine == nil {
			// No endpoint is no attempts, not a failure: not having opened one
			// is the ordinary state of this screen.
			return err
		}
		rows, err := tx.Query(ctx,
			`SELECT `+outboundColumns+` FROM `+outboundTable+`
			 WHERE endpoint_id = $1::uuid ORDER BY created_at DESC LIMIT $2`,
			mine.ID, outboundPerPage)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			attempt, err := scanOutboundAttempt(rows.Scan)
			if err != nil {
				return err
			}
			attempts = append(attempts, attempt)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return json.Marshal(struct {
		Attempts []outboundAttempt `json:"attempts"`
	}{Attempts: attempts})
}

// scanOutboundAttempt reads outboundColumns off one row. The timestamp is scanned
// as a time and rendered afterwards, for the reason scanEndpoint gives.
func scanOutboundAttempt(scan func(...any) error) (outboundAttempt, error) {
	var (
		a         outboundAttempt
		createdAt time.Time
	)
	err := scan(&a.ID, &a.DeliveryKey, &a.Attempt, &a.Recipient, &a.Outcome,
		&a.ErrorClass, &createdAt)
	if err != nil {
		return outboundAttempt{}, err
	}
	a.CreatedAt = createdAt.UTC().Format(time.RFC3339)
	return a, nil
}
