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
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

// errRecheckComplete rolls the recheck's transaction back. It is not a failure.
var errRecheckComplete = errors.New("consent: the recheck reads and writes nothing")

// RecheckTransmit answers whether the issued ticket still holds.
//
// The same answer returns the issued ticket, and nothing is recorded, so an
// ordinary send keeps one decision set per attempt. A different answer is
// decided and recorded again through AuthorizeTransmit, and the new ticket is
// what the dispatcher acts on.
func (g *Gate) RecheckTransmit(ctx context.Context, req commsauthz.TransmitRequest, issued commsauthz.TransmitTicket) (commsauthz.TransmitTicket, error) {
	if err := transmittable(req); err != nil {
		return commsauthz.TransmitTicket{}, err
	}
	legacy := g.legacyTransmitAnswer(ctx, req)
	var fresh commsauthz.TransmitTicket
	err := g.store.db.Tx(ctx, func(tx pgx.Tx) error {
		var err error
		if fresh, _, err = g.transmitVerdictTx(ctx, tx, req, issued.DecisionSetID, legacy, false); err != nil {
			return err
		}
		return errRecheckComplete
	})
	if err != nil && !errors.Is(err, errRecheckComplete) {
		return commsauthz.TransmitTicket{}, err
	}
	if fresh.Allowed == issued.Allowed && fresh.ConsentRefused == issued.ConsentRefused {
		return issued, nil
	}
	return g.AuthorizeTransmit(ctx, req)
}
