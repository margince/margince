// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// ListNoticeCases retains the published vocabulary for established disclosure duties.
func (s *Store) ListNoticeCases(ctx context.Context, states []NoticeState, limit int, cursor string) ([]NoticeCase, storekit.Page, error) {
	return s.readNoticeCasePage(ctx, states, limit, cursor, false)
}

// ListNoticeCaseReviews includes source work without an established legal deadline.
func (s *Store) ListNoticeCaseReviews(ctx context.Context, states []NoticeState, limit int, cursor string) ([]NoticeCase, storekit.Page, error) {
	return s.readNoticeCasePage(ctx, states, limit, cursor, true)
}

func (s *Store) readNoticeCasePage(ctx context.Context, states []NoticeState, limit int, cursor string, includeReviews bool) ([]NoticeCase, storekit.Page, error) {
	if err := requireDSRAdmin(ctx, principal.ActionRead); err != nil {
		return nil, storekit.Page{}, err
	}
	if limit <= 0 || limit > noticeCaseListMax {
		limit = noticeCaseListMax
	}
	wanted := make([]string, 0, len(states))
	for _, st := range states {
		if !knownNoticeState(st) {
			return nil, storekit.Page{}, &ValidationError{Field: fieldState, Reason: "not a notice-case state"}
		}
		wanted = append(wanted, string(st))
	}
	if len(wanted) == 0 {
		wanted = unresolvedNoticeStates()
	}
	var out []NoticeCase
	var page storekit.Page
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		sql, args, err := noticeCaseListQuery(wanted, cursor, limit, includeReviews)
		if err != nil {
			return err
		}
		rows, err := tx.Query(ctx, sql, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			c, err := scanNoticeCase(rows)
			if err != nil {
				return err
			}
			out = append(out, c)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		if err := attachAcquisitions(ctx, tx, out); err != nil {
			return err
		}
		if len(out) <= limit {
			return nil
		}
		out = out[:limit]
		last := out[limit-1]
		token, err := storekit.EncodeOpaque(noticeCaseCursor{DueAt: noticeQueueTime(last), ID: last.ID})
		if err != nil {
			return err
		}
		page = storekit.Page{HasMore: true, NextCursor: token}
		return nil
	})
	return out, page, err
}

type noticeCaseCursor struct {
	DueAt time.Time `json:"due"`
	ID    ids.UUID  `json:"id"`
}

// The cursor uses the same effective queue time as sorting, including pending source corrections.
func noticeCaseListQuery(states []string, cursor string, limit int, includeReviews bool) (string, []any, error) {
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	stateColumn := fieldState
	orderColumn := "coalesce(due_at, created_at)"
	if includeReviews {
		stateColumn, orderColumn = reviewStateSQL(arg), reviewQueueTimeSQL(arg)
	}
	sql := "SELECT" + noticeCaseColumns + storekit.SQLf(" FROM privacy_notice_case WHERE ("+stateColumn+") = ANY($%d)", arg(states))
	if !includeReviews {
		sql += " AND " + noticeDutyClause(arg)
	}
	if cursor != "" {
		after, err := storekit.DecodeOpaque[noticeCaseCursor](cursor)
		// Other queue cursors can decode with a zero deadline. Reject them before paging.
		if err != nil || after.DueAt.IsZero() || after.ID.IsZero() {
			return "", nil, &storekit.MalformedCursorError{}
		}
		sql += storekit.SQLf(" AND ("+orderColumn+", id) > ($%d, $%d)", arg(after.DueAt), arg(after.ID))
	}
	sql += storekit.SQLf(" ORDER BY "+orderColumn+", id LIMIT $%d", arg(limit+1))
	return sql, args, nil
}

func noticeQueueTime(c NoticeCase) time.Time {
	if c.needsSourceReview() {
		return c.CreatedAt
	}
	if c.DueAt != nil {
		return *c.DueAt
	}
	return c.CreatedAt
}
