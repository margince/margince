// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// What a send states about why it is being sent, kept as JSON.
//
// A scheduled send freezes it with the message. A delivery keeps it too, so a
// send that repeats a bounced one asks the consent engine the same question
// the first one did. One shape serves both. A claim that only the freeze
// carried would let a resend reach the engine saying less.

import (
	"encoding/json"
	"fmt"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

// sendClaims is the stored form: ids as strings, because a row outlives the
// code that wrote it.
type sendClaims struct {
	Context          string         `json:"context,omitempty"`
	MarketingPurpose string         `json:"marketing_purpose,omitempty"`
	OperatorReason   string         `json:"operator_reason,omitempty"`
	Evidence         frozenEvidence `json:"evidence,omitzero"`
	// SendingEntity is the legal entity the send resolved. A row written
	// before it was kept has none, and the send resolves one.
	SendingEntity string `json:"sending_entity,omitempty"`
}

// SendClaims is what a send stated about why it was sent.
type SendClaims struct {
	Context          commsauthz.Category
	MarketingPurpose string
	OperatorReason   string
	Evidence         commsauthz.Evidence
	SendingEntity    ids.UUID
}

func freezeClaims(in SendEmailInput) sendClaims {
	return sendClaims{
		Context:          string(in.Context),
		MarketingPurpose: in.MarketingPurpose,
		OperatorReason:   in.OperatorReason,
		Evidence:         freezeEvidence(in.Evidence),
		SendingEntity:    idText(in.SendingEntity),
	}
}

func (c sendClaims) thaw() (SendClaims, error) {
	evidence, err := c.Evidence.thaw()
	if err != nil {
		return SendClaims{}, err
	}
	entity, err := frozenID("sending entity", c.SendingEntity)
	if err != nil {
		return SendClaims{}, err
	}
	// A category reserved for the installation's own notices can never
	// legitimately be in a stored claim: the door that wrote it refuses one.
	// Refusing it again here removes the assumption, so a second writer added
	// later cannot smuggle one past the send.
	if commsauthz.Category(c.Context).ServesTheSubject() {
		return SendClaims{}, &CommunicationContextError{
			Reason: "that category is reserved for the installation's own notices and cannot be claimed by a send",
		}
	}
	return SendClaims{
		Context:          commsauthz.Category(c.Context),
		MarketingPurpose: c.MarketingPurpose,
		OperatorReason:   c.OperatorReason,
		Evidence:         evidence,
		SendingEntity:    entity,
	}, nil
}

func newAuthorship(in SendEmailInput, origin SendOrigin, body, html string) (authorship, error) {
	claims, err := encodeSendClaims(in, origin)
	return authorship{body: body, html: html, claims: claims}, err
}

// encodeSendClaims is what a delivery keeps. A reply's anchor is kept as the
// evidence activity when the caller named none. The engine reads the thread's
// own message from there, and a send anchored on its bounced copy would no
// longer point at it.
func encodeSendClaims(in SendEmailInput, origin SendOrigin) ([]byte, error) {
	if in.Evidence.ActivityID.IsZero() {
		in.Evidence.ActivityID = origin.anchor.UUID
	}
	raw, err := json.Marshal(freezeClaims(in))
	if err != nil {
		return nil, fmt.Errorf("activities: keeping what the send stated: %w", err)
	}
	return raw, nil
}

// DecodeSendClaims reads back what a delivery kept. An empty value is a send
// that stated nothing.
func DecodeSendClaims(raw []byte) (SendClaims, error) {
	var stored sendClaims
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &stored); err != nil {
			return SendClaims{}, fmt.Errorf("activities: reading what the first send stated: %w", err)
		}
	}
	return stored.thaw()
}
