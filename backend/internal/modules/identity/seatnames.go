// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

// Naming a seat.
//
// Its own file rather than a paragraph in users.go, which owns the seat
// LIFECYCLE — invite, reactivate, deactivate, re-role, each a governed write
// with an actor and an audit row behind it. This is a directory read: it admits
// on membership alone and changes nothing. The two share a table and nothing
// else.

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// SeatNames names the colleagues in a set, as a human would recognize them.
//
// It carries no row scope, because a seat is not a record: app_user holds no
// owner_id and no capture privacy, and the surfaces that already name
// colleagues — who_knows, company_coverage — name them to any member. A
// row-scope clause here would be inventing a rule this table has never had.
//
// What bounds it is membership, asked for below. A seat the installation does
// not hold is simply absent from the answer, which the caller renders as the id
// it already has.
func (s *Service) SeatNames(ctx context.Context, seats []ids.UserID) (map[ids.UUID]string, error) {
	if err := auth.RequireMember(ctx); err != nil {
		return nil, err
	}
	names := map[ids.UUID]string{}
	if len(seats) == 0 {
		return names, nil
	}
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx,
			`SELECT id, display_name FROM app_user WHERE id = ANY($1) AND archived_at IS NULL`, seats)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var id ids.UUID
			var name string
			if err := rows.Scan(&id, &name); err != nil {
				return err
			}
			names[id] = name
		}
		return rows.Err()
	})
	return names, err
}

// DepartedAmong answers which of seats left: deactivated and not archived. A
// naming read marks them, so a record a departed colleague still owns says it
// needs a new owner wherever their name appears.
func (s *Service) DepartedAmong(ctx context.Context, seats []ids.UserID) (map[ids.UUID]bool, error) {
	if err := auth.RequireMember(ctx); err != nil {
		return nil, err
	}
	departed := map[ids.UUID]bool{}
	if len(seats) == 0 {
		return departed, nil
	}
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx,
			`SELECT id FROM app_user WHERE id = ANY($1) AND `+DepartedMemberSQL(""), seats)
		if err != nil {
			return err
		}
		gone, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
		for _, id := range gone {
			departed[id] = true
		}
		return err
	})
	return departed, err
}
