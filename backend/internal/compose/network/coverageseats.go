// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package network

// Both sides of a coverage view as a reader counts them. Our side has one entry
// per colleague, and each seat on theirs says when it last wrote to us.

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// SeatInbound is when one seat last wrote to us, and how precisely that is
// known: "day" for an entry that names only its date.
type SeatInbound struct {
	At        time.Time
	Precision string
}

// OurColleagues folds the edges into one entry per colleague, summing Count90d
// and keeping the first, warmest score. The concentration rule and every wire
// shape read it, so a count of colleagues and a finding about one agree.
func OurColleagues(edges []ColleagueEdge) []ColleagueEdge {
	at := make(map[ids.UUID]int, len(edges))
	out := make([]ColleagueEdge, 0, len(edges))
	for _, edge := range edges {
		if index, seen := at[edge.UserID]; seen {
			out[index].Count90d += edge.Count90d
			continue
		}
		at[edge.UserID] = len(out)
		edge.ContactID = ids.UUID{}
		out = append(out, edge)
	}
	return out
}

// readSeatsInbound dates each seat's last message to us through the contact
// strength fold, the one the contact page reads. A caller without the activity
// or contact grant gets nil: an absent date says nothing about the seat.
func readSeatsInbound(
	ctx context.Context, tx pgx.Tx, seats []deals.DealStakeholder, now time.Time,
) (map[ids.UUID]SeatInbound, error) {
	if len(seats) == 0 {
		return nil, nil //nolint:nilnil // no seats is no dates to give, which the wire reads as absent
	}
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		return nil, deniedAsNone(err)
	}
	contactIDs := make([]ids.ContactID, 0, len(seats))
	for _, seat := range seats {
		contactIDs = append(contactIDs, ids.From[ids.ContactKind](seat.ContactID))
	}
	strengths, err := contacts.StrengthForContacts(ctx, tx, contactIDs, now)
	if err != nil {
		return nil, deniedAsNone(err)
	}
	out := make(map[ids.UUID]SeatInbound, len(strengths))
	for _, seat := range strengths {
		if last := seat.Strength.LastInbound; last != nil {
			out[seat.ContactID.UUID] = SeatInbound{
				At: *last, Precision: seat.Strength.LastInboundPrecision,
			}
		}
	}
	return out, nil
}

// deniedAsNone turns a refused grant into no answer and passes any other error on.
func deniedAsNone(err error) error {
	if errors.Is(err, apperrors.ErrPermissionDenied) {
		return nil
	}
	return err
}
