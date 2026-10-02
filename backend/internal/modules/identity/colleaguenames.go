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

// ColleagueNamesAmong answers which of the given names belong to a live human
// colleague of the caller — by display name, its first word or its last word,
// case-insensitive. The caller is never among them: a name somebody writes in
// their own document is not a quotation of themselves.
//
// It reads the same seats Colleagues lists, so a name answered here is a
// colleague the roster would show.
func (s *Service) ColleagueNamesAmong(ctx context.Context, names []string) ([]string, error) {
	if err := auth.RequireMember(ctx); err != nil {
		return nil, err
	}
	if len(names) == 0 {
		return nil, nil
	}
	actor, ok := principal.Actor(ctx)
	if !ok {
		return nil, apperrors.ErrPermissionDenied
	}
	var found []string
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT n
			  FROM unnest($1::text[]) WITH ORDINALITY AS wanted(n, at)
			 WHERE EXISTS (
			       SELECT 1
			         FROM app_user u, LATERAL (SELECT lower(btrim(regexp_replace(u.display_name, '\s+', ' ', 'g'))) AS name) d
			        WHERE `+LiveMemberSQL("u")+`
			          AND NOT u.is_agent
			          AND u.id <> $2
			          AND lower(btrim(regexp_replace(wanted.n, '\s+', ' ', 'g')))
			              IN (d.name, split_part(d.name, ' ', 1), regexp_replace(d.name, '^.* ', '')))
			 ORDER BY at`, names, actor.UserID)
		if err != nil {
			return err
		}
		found, err = pgx.CollectRows(rows, pgx.RowTo[string])
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("identity: matching colleague names: %w", err)
	}
	return found, nil
}
