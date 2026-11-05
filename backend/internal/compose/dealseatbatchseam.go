// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/dealstatus"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func dealSeatBatchReader(poolStore *contacts.Store) dealstatus.SeatBatchReader {
	return func(ctx context.Context, tx pgx.Tx, dealIDs []ids.UUID, _ time.Time) (map[ids.UUID][]dealstatus.Seat, error) {
		seats, err := deals.StakeholderSeats(ctx, tx, dealIDs)
		if errors.Is(err, apperrors.ErrPermissionDenied) {
			return map[ids.UUID][]dealstatus.Seat{}, nil
		}
		if err != nil {
			return nil, err
		}
		var all []deals.DealStakeholder
		for _, dealSeats := range seats {
			all = append(all, dealSeats...)
		}
		names, err := seatNamesForCard(ctx, tx, poolStore, all)
		if err != nil {
			return nil, err
		}
		attachable, err := seatsThisReaderMayFileAgainst(ctx, tx, all)
		if err != nil {
			return nil, err
		}
		out := make(map[ids.UUID][]dealstatus.Seat, len(seats))
		for dealID, dealSeats := range seats {
			for _, seat := range dealSeats {
				out[dealID] = append(out[dealID], dealstatus.Seat{
					Role: seat.Role, Name: names[seat.ContactID], ContactID: seat.ContactID,
					Attachable: attachable[seat.ContactID],
				})
			}
		}
		return out, nil
	}
}
