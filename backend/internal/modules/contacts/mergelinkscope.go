// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const (
	mergeMembershipSource = "a.entity_type = @kind AND a.entity_id = @source"
	mergeMembershipTarget = "b.entity_type = @kind AND b.entity_id = @target"
	mergeListDuplicate    = "EXISTS (SELECT 1 FROM list_member b WHERE b.list_id = a.list_id AND " + mergeMembershipTarget + ")"
	mergeTagDuplicate     = "EXISTS (SELECT 1 FROM taggable b WHERE b.tag_id = a.tag_id AND " + mergeMembershipTarget + ")"
	mergeListNoteFill     = "b.note IS NULL AND a.note IS NOT NULL"
)

type MergeLink struct {
	Kind      string
	ID        ids.UUID
	Collapsed bool
	Note      *string
}

func mergeActivityPredicates(kind string) (column, source, duplicate string, err error) {
	switch kind {
	case "contact":
		column = "contact_id"
	case "company":
		column = "company_id"
	default:
		return "", "", "", apperrors.ErrInvalidArgument
	}
	source = "a." + column + " = @source"
	duplicate = "EXISTS (SELECT 1 FROM activity_link b WHERE b.activity_id = a.activity_id AND b." + column + " = @target)"
	return column, source, duplicate, nil
}

func previewMergeLinks(ctx context.Context, tx pgx.Tx, kind string, source, target ids.UUID) ([]MergeLink, error) {
	_, activitySource, activityDuplicate, err := mergeActivityPredicates(kind)
	if err != nil {
		return nil, err
	}
	args := pgx.NamedArgs{"source": source, "target": target, "kind": kind}
	var out []MergeLink
	for _, query := range []struct{ kind, sql string }{
		{"activity", "SELECT a.activity_id, " + activityDuplicate + ", NULL::text FROM activity_link a WHERE " + activitySource + " ORDER BY a.activity_id LIMIT 501"},
		{"list", `SELECT a.list_id, ` + mergeListDuplicate + `, CASE WHEN NOT ` + mergeListDuplicate + ` OR EXISTS (SELECT 1 FROM list_member b WHERE b.list_id=a.list_id AND ` + mergeMembershipTarget + ` AND ` + mergeListNoteFill + `) THEN a.note END FROM list_member a WHERE ` + mergeMembershipSource + ` ORDER BY a.list_id LIMIT 501`},
		{"tag", `SELECT a.tag_id, ` + mergeTagDuplicate + `, NULL::text FROM taggable a WHERE ` + mergeMembershipSource + ` ORDER BY a.tag_id LIMIT 501`},
	} {
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
			if err := auth.Require(ctx, link.Kind, principal.ActionRead); err != nil {
				return nil, err
			}
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
