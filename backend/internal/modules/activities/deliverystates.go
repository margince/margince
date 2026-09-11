// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// What happened to an outbound message, for the row that shows it.
//
// The timeline carried DIRECTION and nothing else, which reads as a delivery
// state and is not one: a message parked because the channel refused its files,
// because the recipient blocked the bot, or because the credential was rejected
// rendered exactly like one the provider confirmed. The delivery row knew
// better the whole time and none of it reached the screen.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// DeliveryState is one message's delivery, as the row records it.
//
// Status and bounce are two fields because the row keeps them apart: a bounce
// is a later fact about a send the provider DID accept, so the status stays
// `sent` and the return is recorded beside it. Collapsing them here would lose
// the distinction the screen needs.
type DeliveryState struct {
	Status       string
	Reason       *string
	SentAt       *time.Time
	BouncedAt    *time.Time
	BounceReason *string
	// Files is what the message was staged with, read back from the snapshot
	// the send wrote — never the live attachment list, which archiving a
	// document later rewrites.
	Files []OutboundFile
}

// DeliveryStatesFor reads what happened to each of these messages, in ONE
// statement over the whole page.
//
// The same batching rule the attachment counts and the email rows follow: a
// read per visible line turns a timeline of twenty into twenty statements.
//
// It carries NO gate of its own, and must only be called with ids a
// content-gated read already admitted. Whether a message left is something
// about the message, so the caller's own gate is what decides — exactly as the
// attachment count beside it.
//
// An id absent from the map had no delivery staged, which is not the same as a
// delivery that has not left: an inbound message has none, and so does an
// outbound one that was logged rather than sent. The row says nothing in that
// case rather than guessing at `pending`.
func DeliveryStatesFor(
	ctx context.Context, tx pgx.Tx, activityIDs []ids.UUID,
) (map[ids.UUID]DeliveryState, error) {
	if len(activityIDs) == 0 {
		return map[ids.UUID]DeliveryState{}, nil
	}
	rows, err := tx.Query(ctx, `
		SELECT activity_id, status, reason, sent_at, bounced_at, bounce_reason, attachments
		  FROM comms_outbound
		 WHERE activity_id = ANY($1)`, activityIDs)
	if err != nil {
		return nil, fmt.Errorf("activities: reading what happened to a page of messages: %w", err)
	}
	defer rows.Close()

	out := make(map[ids.UUID]DeliveryState, len(activityIDs))
	for rows.Next() {
		var id ids.UUID
		var state DeliveryState
		if err := rows.Scan(
			&id, &state.Status, &state.Reason, &state.SentAt,
			&state.BouncedAt, &state.BounceReason, &state.Files,
		); err != nil {
			return nil, fmt.Errorf("activities: scanning a delivery state: %w", err)
		}
		out[id] = state
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("activities: reading what happened to a page of messages: %w", err)
	}
	return out, nil
}

// applyDeliveryStates stamps onto the email rows of a page what happened to
// each message.
//
// It does not decide which rows may carry one. emailIDsOf does, and a row it
// left out has no entry here and so is told nothing — which is why the two are
// separate: a withheld summary has had its content stripped, and "this message
// you may not read was parked because the recipient blocked us" is that content
// in smaller print. That rule governs every fact a page picks up, so it sits
// where the ids are chosen rather than being restated for each one.
//
// The timeline page and the single-message presentation both reach the delivery
// through here, the latter as a page of one.
func applyDeliveryStates(page []crmcontracts.Activity, states map[ids.UUID]DeliveryState) {
	for i := range page {
		summary := page[i].EmailSummary
		if summary == nil {
			continue
		}
		state, known := states[ids.UUID(page[i].Id)]
		summary.Delivery = contractDelivery(state, known)
	}
}

// withDeliveryOn fills in what happened to ONE message.
//
// A page of one, so which rows may carry a delivery is decided where the
// timeline decides it. Not WithEmailRowFacts: the single-message read already
// has the attachments in hand, and counting them again would be a statement
// for a number that caller can see.
func withDeliveryOn(
	ctx context.Context, tx pgx.Tx, id openapi_types.UUID, summary *crmcontracts.EmailSummary,
) error {
	row := []crmcontracts.Activity{{Id: id, EmailSummary: summary}}
	states, err := DeliveryStatesFor(ctx, tx, emailIDsOf(row))
	if err != nil {
		return err
	}
	applyDeliveryStates(row, states)
	return nil
}

// contractDelivery renders one delivery for the wire, or nil when the message
// has none to report.
//
// A bounce outranks the status it sits on: the provider accepted the message,
// so the row says `sent`, and the receiving system handed it straight back.
// Showing `sent` there tells a rep their mail arrived.
//
// The reason travels only with a state that has one and the instant only with
// `sent`, because each is meaningless beside the others: a reason on a sent
// message would be the last park it recovered from, and a delivered_at on a
// parked one is the attempt that failed.
func contractDelivery(state DeliveryState, known bool) *crmcontracts.EmailDelivery {
	if !known {
		return nil
	}
	out := crmcontracts.EmailDelivery{
		State: crmcontracts.EmailDeliveryState(state.Status),
		Files: contractDeliveryFiles(state.Files),
	}
	switch {
	case state.BouncedAt != nil:
		out.State = crmcontracts.EmailDeliveryStateBounced
		out.Reason = state.BounceReason
	case out.State == crmcontracts.EmailDeliveryStateParked:
		out.Reason = state.Reason
	case out.State == crmcontracts.EmailDeliveryStateSent:
		out.DeliveredAt = state.SentAt
	}
	return &out
}

// contractDeliveryFiles renders the staged snapshot for the wire.
//
// Always a slice, never nil: "this message carried nothing" and "this row does
// not say what it carried" are the same answer here, because every delivery row
// records its attachments and an empty array is what the column holds when
// there were none.
func contractDeliveryFiles(files []OutboundFile) *[]crmcontracts.EmailDeliveryFile {
	out := make([]crmcontracts.EmailDeliveryFile, 0, len(files))
	for _, file := range files {
		rendered := crmcontracts.EmailDeliveryFile{Filename: file.Filename}
		if file.ContentType != "" {
			rendered.ContentType = &file.ContentType
		}
		if file.ByteSize != 0 {
			rendered.ByteSize = &file.ByteSize
		}
		out = append(out, rendered)
	}
	return &out
}
