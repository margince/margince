// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/recordmerge"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// MergeLink preserves relationship identity for review before its endpoint changes.
type MergeLink struct {
	Kind      string
	ID        ids.UUID
	Collapsed bool
	Note      *string
}

func previewMergeLinks(ctx context.Context, tx pgx.Tx, kind string, source, target ids.UUID) ([]MergeLink, error) {
	_, activitySource, activityDuplicate, err := recordmerge.ActivityPredicates(kind)
	if err != nil {
		return nil, err
	}
	args := pgx.NamedArgs{mergeSourceKey: source, mergeTargetKey: target, recordKindParam: kind}
	var out []MergeLink
	queries := []struct{ kind, sql string }{
		{"activity", "SELECT a.activity_id, " + activityDuplicate + ", NULL::text FROM activity_link a WHERE " + activitySource + " ORDER BY a.activity_id LIMIT 501"},
		{"list", `SELECT a.list_id, ` + recordmerge.ListDuplicate + `, CASE WHEN NOT ` + recordmerge.ListDuplicate + ` OR EXISTS (SELECT 1 FROM list_member b WHERE b.list_id=a.list_id AND ` + recordmerge.MembershipTarget + ` AND ` + recordmerge.ListNoteFill + `) THEN a.note END FROM list_member a WHERE ` + recordmerge.MembershipSource + ` ORDER BY a.list_id LIMIT 501`},
		{"tag", `SELECT a.tag_id, ` + recordmerge.TagDuplicate + `, NULL::text FROM taggable a WHERE ` + recordmerge.MembershipSource + ` ORDER BY a.tag_id LIMIT 501`},
	}
	for _, query := range queries {
		if err := auth.Require(ctx, query.kind, principal.ActionRead); err != nil {
			return nil, err
		}
	}
	for _, query := range queries {
		rows, err := tx.Query(ctx, query.sql, args)
		if err != nil {
			return nil, err
		}
		links, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (MergeLink, error) {
			link := MergeLink{Kind: query.kind}
			err := row.Scan(&link.ID, &link.Collapsed, &link.Note)
			return link, err
		})
		if err != nil {
			return nil, err
		}
		if len(links) > 500 {
			return nil, apperrors.ErrInvalidArgument
		}
		for _, link := range links {
			switch link.Kind {
			case "activity":
				if err := auth.EnsureActivityContentVisible(ctx, tx, link.ID); err != nil {
					return nil, err
				}
			case "list":
				if err := auth.EnsureVisibleExisting(ctx, tx, "list", link.ID); err != nil {
					return nil, err
				}
			}
		}
		out = append(out, links...)
	}
	return out, nil
}
