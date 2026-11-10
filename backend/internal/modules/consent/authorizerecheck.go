// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// The second look right before the provider call.
//
// AuthorizeTransmit answers before pacing, the attachment checks and the file
// read. A stop or a retraction committed in that time is in no ticket, so the
// dispatcher asks once more at the wire. The same transmitVerdictTx answers both
// times, so there is one decision code.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

// errRecheckComplete rolls the recheck's transaction back. It is not a failure.
var errRecheckComplete = errors.New("consent: the recheck reads and writes nothing")

// RecheckTransmit answers whether the issued ticket still holds.
//
// An equal fingerprint returns the issued ticket, and nothing is recorded, so
// an ordinary send keeps one decision set per attempt. Any change (a stop, a
// country, a rule pack, a category, a purpose, a mode) is decided and recorded
// again through AuthorizeTransmit. The new ticket and its set are what the
// dispatcher and the requirement check use.
func (g *Gate) RecheckTransmit(ctx context.Context, req commsauthz.TransmitRequest, issued commsauthz.TransmitTicket) (commsauthz.TransmitTicket, error) {
	fresh, err := g.readTransmitTicket(ctx, req, issued)
	if err != nil {
		return commsauthz.TransmitTicket{}, err
	}
	if fresh.Fingerprint != "" && fresh.Fingerprint == issued.Fingerprint {
		return issued, nil
	}
	return g.AuthorizeTransmit(ctx, req)
}

func (g *Gate) readTransmitTicket(ctx context.Context, req commsauthz.TransmitRequest, issued commsauthz.TransmitTicket) (commsauthz.TransmitTicket, error) {
	if err := transmittable(req); err != nil {
		return commsauthz.TransmitTicket{}, err
	}
	legacy := g.legacyTransmitAnswer(ctx, req, false)
	var fresh commsauthz.TransmitTicket
	err := g.readTransmitTx(ctx, func(tx pgx.Tx) error {
		var err error
		if fresh, _, err = g.transmitVerdictTx(ctx, tx, req, issued.DecisionSetID, legacy, false); err != nil {
			return err
		}
		return errRecheckComplete
	})
	if err != nil && !errors.Is(err, errRecheckComplete) {
		return commsauthz.TransmitTicket{}, err
	}
	return fresh, nil
}

// decisionFingerprint digests what a transmit ticket was decided on. The
// requirement check reads the categories and rule packs, so both are in it.
func decisionFingerprint(ticket commsauthz.TransmitTicket, set commsauthz.DecisionSet, countries []RecipientCountry) (string, error) {
	type recipientInput struct {
		Decision commsauthz.Decision
		Country  RecipientCountry
		Ruleset  rulesetStampValue
	}
	inputs := struct {
		Allowed, ConsentRefused bool
		Reason                  string
		Recipients              []recipientInput
	}{Allowed: ticket.Allowed, ConsentRefused: ticket.ConsentRefused, Reason: ticket.Reason}
	for i, d := range set.Decisions {
		in := recipientInput{Decision: d}
		if i < len(countries) {
			in.Country, in.Ruleset = countries[i], rulesetStamp(countries[i])
		}
		inputs.Recipients = append(inputs.Recipients, in)
	}
	encoded, err := json.Marshal(inputs)
	if err != nil {
		return "", fmt.Errorf("consent: fingerprinting the transmit decision: %w", err)
	}
	digest := sha256.Sum256(encoded)
	return hex.EncodeToString(digest[:]), nil
}
