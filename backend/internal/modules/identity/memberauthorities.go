// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/authz"
)

// MemberAuthority is one live member's grants, teams and seat.
type MemberAuthority struct {
	RBAC authz.RBAC
	Seat principal.SeatType
}

// LiveMemberAuthorities is EffectiveAuthority for many members in a fixed
// number of reads, for an answer that judges every member at once. A member
// who is not live is absent from the result, which is EffectiveAuthority's
// ErrNotFound for them. It reads through loadGrantsFor, the loader every
// session resolves through, and at REPEATABLE READ for the reason liveUserTx
// gives.
//
// Gated on membership: the answer says what colleagues may do, and only a
// colleague asks.
func (s *Service) LiveMemberAuthorities(ctx context.Context, users []ids.UUID) (map[ids.UUID]MemberAuthority, error) {
	if err := auth.RequireMember(ctx); err != nil {
		return nil, err
	}
	out := make(map[ids.UUID]MemberAuthority, len(users))
	err := s.db.TxIsolated(ctx, pgx.RepeatableRead, func(tx pgx.Tx) error {
		seats, err := liveSeats(ctx, tx, users)
		if err != nil {
			return err
		}
		live := make([]ids.UUID, 0, len(seats))
		for user := range seats {
			live = append(live, user)
		}
		grants, err := loadGrantsFor(ctx, tx, live)
		if err != nil {
			return err
		}
		for _, user := range live {
			g := grants[user]
			out[user] = MemberAuthority{
				RBAC: authz.RBAC{Permissions: g.perms, TeamIDs: rawTeamIDs(g.teams)},
				Seat: seats[user],
			}
		}
		return nil
	})
	return out, err
}

func liveSeats(ctx context.Context, tx pgx.Tx, users []ids.UUID) (map[ids.UUID]principal.SeatType, error) {
	rows, err := tx.Query(ctx,
		`SELECT id, seat_type FROM app_user WHERE id = ANY($1) AND `+LiveMemberSQL(""), users)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[ids.UUID]principal.SeatType{}
	for rows.Next() {
		var user ids.UUID
		var seat string
		if err := rows.Scan(&user, &seat); err != nil {
			return nil, err
		}
		out[user] = principal.SeatType(seat)
	}
	return out, rows.Err()
}
