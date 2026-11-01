// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// NoticeSourceReviewsTx keeps pending source work visible to authorized contact readers.
func (s *Store) NoticeSourceReviewsTx(ctx context.Context, tx pgx.Tx, contactID ids.ContactID) ([]ids.UUID, error) {
	if err := requireDSRAdmin(ctx, principal.ActionRead); err != nil {
		return nil, err
	}
	if err := auth.Require(ctx, "contact", principal.ActionRead); err != nil {
		return nil, err
	}
	if err := auth.EnsureVisibleExisting(ctx, tx, "contact", contactID.UUID); err != nil {
		return nil, err
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	where := storekit.SQLf("contact_id = $%d AND (", arg(contactID)) + reviewStateSQL(arg) + storekit.SQLf(") = $%d", arg(string(NoticeSourceReview)))
	query := "SELECT id FROM privacy_notice_case WHERE " + where + storekit.SQLf(" ORDER BY id LIMIT $%d", arg(noticeCaseListMax))
	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
}

// UnverifiedNoticeSourceSQL keeps unproven deadlines off the agenda while corrections are pending.
func UnverifiedNoticeSourceSQL(caseAlias, acquisitionAlias string) string {
	notice := pgx.Identifier{caseAlias}.Sanitize()
	acquisition := pgx.Identifier{acquisitionAlias}.Sanitize()
	return notice + ".source_kind IS NULL AND (" + acquisition + ".kind = 'unknown_legacy')"
}

func noticeLiveOwnerSQL(alias string) string {
	owner := pgx.Identifier{alias}.Sanitize() + ".owner_user_id"
	return "CASE WHEN EXISTS (SELECT 1 FROM app_user u WHERE u.id = " + owner + " AND u.removed_at IS NOT NULL) THEN NULL ELSE " + owner + " END"
}
