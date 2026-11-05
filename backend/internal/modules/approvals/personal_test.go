// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package approvals

import (
	"encoding/json"
	"testing"

	"github.com/margince/margince/backend/internal/platform/approvalsubject"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestPersonalDecisionsRequireAnExplicitRecipient(t *testing.T) {
	reader, colleague := ids.New[ids.UserKind](), ids.New[ids.UserKind]()
	passport := ids.New[ids.PassportKind]()
	promise := func(seat, promiser *ids.UserID) json.RawMessage {
		body, err := json.Marshal(map[string]*ids.UserID{"seat_id": seat, "promiser_seat_id": promiser})
		if err != nil {
			t.Fatal(err)
		}
		return body
	}
	cases := []struct {
		name     string
		proposal row
		own      bool
	}{
		{"requested promise", row{Kind: kindCommitmentTask, ProposedChange: promise(&reader, &reader), OnBehalfOf: &reader}, true},
		{"colleague promise", row{Kind: kindCommitmentTask, ProposedChange: promise(&colleague, &colleague), OnBehalfOf: &colleague}, false},
		{"a promise naming nobody", row{Kind: kindCommitmentTask, ProposedChange: promise(nil, nil)}, false},
		{"a promise staged before its promiser had a seat", row{Kind: kindCommitmentTask, ProposedChange: promise(nil, &reader)}, true},
		{"a colleague's promise staged for the reader", row{Kind: kindCommitmentTask, ProposedChange: promise(&reader, &colleague), OnBehalfOf: &reader}, false},
		{"agent authority is not assignment", row{Kind: "advance_deal", OnBehalfOf: &reader}, false},
		{"delegated deal decision", row{Kind: "advance_deal", PassportID: &passport, OnBehalfOf: &reader}, true},
		{"delegated colleague decision", row{Kind: "send_email", PassportID: &passport, OnBehalfOf: &colleague}, false},
		{"held send", row{Kind: KindScheduledSendHeld, OnBehalfOf: &reader}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := requestedFor(c.proposal, reader.UUID); got != c.own {
				t.Fatalf("personal = %v, want %v", got, c.own)
			}
		})
	}
	body, err := json.Marshal(approvalsubject.Counterparty{OwnerID: reader.UUID})
	if err != nil {
		t.Fatal(err)
	}
	proposal := row{Kind: approvalsubject.KindCounterparty, ProposedChange: body, OnBehalfOf: &colleague, PassportID: &passport}
	if !requestedFor(proposal, reader.UUID) || requestedFor(proposal, colleague.UUID) {
		t.Fatal("payload responsibility was replaced by the granting user")
	}
}
