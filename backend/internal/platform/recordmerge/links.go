// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package recordmerge

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The named parameters every merge statement here binds.
const (
	SourceKey = "source"
	TargetKey = "target"
	KindKey   = "kind"
)

// The predicates over the list and tag tables, which name a record by kind and id.
const (
	MembershipSource = "a.entity_type = @kind AND a.entity_id = @source"
	MembershipTarget = "b.entity_type = @kind AND b.entity_id = @target"
	ListDuplicate    = "EXISTS (SELECT 1 FROM list_member b WHERE b.list_id = a.list_id AND " + MembershipTarget + ")"
	TagDuplicate     = "EXISTS (SELECT 1 FROM taggable b WHERE b.tag_id = a.tag_id AND " + MembershipTarget + ")"
	ListNoteFill     = "b.note IS NULL AND a.note IS NOT NULL"
)

// activityLinkColumns names the activity_link column that holds each mergeable
// record kind.
var activityLinkColumns = map[string]string{
	"contact": "contact_id",
	"company": "company_id",
	"deal":    "deal_id",
}

// ActivityPredicates answers the activity_link column for a record kind. It
// also answers the predicate that selects the retired record's links and the
// one that finds a link the survivor holds on the same activity.
func ActivityPredicates(kind string) (column, source, duplicate string, err error) {
	column, ok := activityLinkColumns[kind]
	if !ok {
		return "", "", "", apperrors.ErrInvalidArgument
	}
	source = "a." + column + " = @source"
	duplicate = "EXISTS (SELECT 1 FROM activity_link b WHERE b.activity_id = a.activity_id AND b." + column + " = @target)"
	return column, source, duplicate, nil
}

// RelinkLinkRows re-homes the pure link tables (activity_link,
// list_member, taggable): a link the survivor already holds stands, and the
// retired copy is dropped. activity_link has no provenance; the list note is
// kept, and the earlier adder or tagger of a duplicate is not.
func RelinkLinkRows(ctx context.Context, tx pgx.Tx, entityType string, sourceID, targetID ids.UUID) (int64, error) {
	column, source, duplicate, err := ActivityPredicates(entityType)
	if err != nil {
		return 0, err
	}
	args := pgx.NamedArgs{SourceKey: sourceID, TargetKey: targetID, KindKey: entityType}
	if _, err := tx.Exec(ctx, `DELETE FROM activity_link a
 WHERE `+source+` AND `+duplicate, args); err != nil {
		return 0, err
	}
	tag, err := tx.Exec(ctx, `UPDATE activity_link a SET `+column+` = @target WHERE `+source, args)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), RelinkMemberships(ctx, tx, entityType, sourceID, targetID)
}

// RelinkMemberships moves the merged-away record's list memberships and tags
// onto the survivor. A membership or tag the survivor already holds stands,
// and the retired copy is dropped. The lead merge, which moves its timeline on
// its own, calls this alone.
func RelinkMemberships(ctx context.Context, tx pgx.Tx, entityType string, sourceID, targetID ids.UUID) error {
	args := pgx.NamedArgs{SourceKey: sourceID, TargetKey: targetID, KindKey: entityType}
	// Spelled out per table: the company-merge coverage census reads these
	// statements, and a table name assembled at runtime is invisible to it.
	for _, stmt := range []string{
		`UPDATE list_member b SET note = a.note FROM list_member a
 WHERE ` + MembershipSource + ` AND ` + MembershipTarget + ` AND b.list_id = a.list_id AND ` + ListNoteFill,
		`DELETE FROM list_member a
 WHERE ` + MembershipSource + ` AND ` + ListDuplicate,
		`UPDATE list_member a SET entity_id = @target WHERE ` + MembershipSource,
		`DELETE FROM taggable a
 WHERE ` + MembershipSource + ` AND ` + TagDuplicate,
		`UPDATE taggable a SET entity_id = @target WHERE ` + MembershipSource,
	} {
		if _, err := tx.Exec(ctx, stmt, args); err != nil {
			return fmt.Errorf("move the merged-away record's lists and tags: %w", err)
		}
	}
	return nil
}
