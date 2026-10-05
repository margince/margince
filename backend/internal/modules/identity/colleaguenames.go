// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// SeatNamesAmong answers which of the given names belong to a seat — by display
// name, its first word or its last word, case-insensitive — split by whose.
// Colleagues are the live human seats Colleagues lists, so a name answered there
// is one the roster would show; own are the names that are the caller's own,
// because a name somebody writes in their own document is not a quotation of
// themselves.
func (s *Service) SeatNamesAmong(ctx context.Context, names []string) (colleagues, own []string, err error) {
	if err := auth.RequireMember(ctx); err != nil {
		return nil, nil, err
	}
	if len(names) == 0 {
		return nil, nil, nil
	}
	actor, ok := principal.Actor(ctx)
	if !ok {
		return nil, nil, apperrors.ErrPermissionDenied
	}
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT wanted.n,
			       bool_or(u.id <> $2 AND NOT u.is_agent AND u.locked_until IS NULL),
			       bool_or(u.id = $2)
			  FROM unnest($1::text[]) WITH ORDINALITY AS wanted(n, at)
			  JOIN app_user u ON `+LiveMemberSQL("u")+`
			  CROSS JOIN LATERAL (SELECT lower(btrim(regexp_replace(u.display_name, '\s+', ' ', 'g'))) AS name) d
			 WHERE lower(btrim(regexp_replace(wanted.n, '\s+', ' ', 'g')))
			       IN (d.name, split_part(d.name, ' ', 1), regexp_replace(d.name, '^.* ', ''))
			 GROUP BY wanted.n, wanted.at
			 ORDER BY wanted.at`, names, actor.UserID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var name string
			var colleague, caller bool
			if err := rows.Scan(&name, &colleague, &caller); err != nil {
				return err
			}
			if colleague {
				colleagues = append(colleagues, name)
			}
			if caller {
				own = append(own, name)
			}
		}
		return rows.Err()
	})
	if err != nil {
		return nil, nil, fmt.Errorf("identity: matching seat names: %w", err)
	}
	return colleagues, own, nil
}
