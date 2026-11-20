// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// legacyTransmitAnswer is the legacy gate's answer, carried into the
// transaction as a value, with the recipients' countries it was taken under.
// The engine decides under the same countries, so the two verdicts that
// Effective weighs never judge one recipient under two markets.
type legacyTransmitAnswer struct {
	allowed, unanswerable bool
	err                   error
	countries             []RecipientCountry
}

func (g *Gate) legacyTransmitAnswer(ctx context.Context, req commsauthz.TransmitRequest, record bool) (legacyTransmitAnswer, error) {
	countries, err := g.transmitCountries(ctx, req, record)
	if err != nil {
		return legacyTransmitAnswer{}, err
	}
	// The legacy gate's answer is recorded beside the engine's on every row, so
	// a disagreement is readable in the record.
	//
	// It decides only when no recipient's category is enforced. The shipped
	// setting never does that; an operator moving a category back to observe
	// does, and the rollback depends on it.
	legacyErr := g.checkGrantedRecipients(ctx, req.Recipients, req.PurposeKey, record, countries)
	// Three outcomes: granted, not granted, and no answer. No answer is not a
	// refusal. Whether it matters depends on the modes, which are read inside
	// the transaction, so it is carried there as a value.
	legacyUnanswerable := legacyErr != nil && !errors.Is(legacyErr, apperrors.ErrConsentNotGranted)
	legacyAllowed := legacyErr == nil

	return legacyTransmitAnswer{allowed: legacyAllowed, unanswerable: legacyUnanswerable, err: legacyErr, countries: countries}, nil
}

// transmitCountries resolves each recipient's country once, from what staging
// stored about them, including a country the sender stated.
func (g *Gate) transmitCountries(ctx context.Context, req commsauthz.TransmitRequest, record bool) ([]RecipientCountry, error) {
	transact := g.store.db.Tx
	if !record {
		transact = g.readTransmitTx
	}
	var out []RecipientCountry
	err := transact(ctx, func(tx pgx.Tx) error {
		claims, err := stagedClaims(ctx, tx, req.DeliveryID)
		if err != nil {
			return err
		}
		// Only the staged evidence picks a country; the thread and the links
		// do not, so neither is read here.
		out, err = g.store.recipientCountries(ctx, tx, req.Recipients, func(r connector.Recipient) commsauthz.Request {
			return stagedRequestFor(req, r, claims, "", nil)
		})
		return err
	})
	return out, err
}
