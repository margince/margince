// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package collections

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// The vocabulary's own verbs — read one word with its weight, rename it,
// bring it back, fold two into one. Split from tags.go, which holds the
// apply/remove half, because the two answer different questions and the file
// ceiling is the reminder that they do.

// TagUsage counts how much of the workspace carries one tag, per advertised
// record type.
type TagUsage struct {
	People    int
	Companies int
	Deals     int
}

// advertisedTagTypes are the three record types the product offers tags on.
// `taggable` admits lead and project too — the column has carried five since
// the baseline — but nothing in V1 shows or filters those, so counting them
// here would report a weight no screen can explain.
var advertisedTagTypes = []string{"person", "organization", "deal"}

// GetTag reads one tag and how much of the workspace carries it.
//
// The counts are the reader's own: an admin deciding whether to retire a word
// needs to know what retiring it costs, and a number including rows they
// cannot see is one they cannot act on.
func (s *Store) GetTag(ctx context.Context, id ids.TagID) (tagRow, TagUsage, error) {
	if err := auth.Require(ctx, "tag", principal.ActionRead); err != nil {
		return tagRow{}, TagUsage{}, err
	}
	var out tagRow
	var usage TagUsage
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		row := tx.QueryRow(ctx, "SELECT "+tagColumns+" FROM tag WHERE id = $1", id)
		var scanErr error
		if out, scanErr = scanTag(row); errors.Is(scanErr, pgx.ErrNoRows) {
			return apperrors.ErrNotFound
		} else if scanErr != nil {
			return scanErr
		}
		var usageErr error
		usage, usageErr = tagUsage(ctx, tx, id)
		return usageErr
	})
	return out, usage, err
}

// tagUsage counts the taggings per advertised type in one grouped pass.
//
// It counts `taggable` rows and does NOT join the record tables, which means
// it does not apply each type's row scope. That is a deliberate limit and the
// reason the contract calls this a weight rather than a list: the counts tell
// an admin how much retiring a word would touch, and the records themselves
// come from the list endpoints, which are row-scoped. A count that leaked
// which rows exist would be a different problem — this one leaks only how many
// carry a word the caller can already read.
func tagUsage(ctx context.Context, tx pgx.Tx, id ids.TagID) (TagUsage, error) {
	rows, err := tx.Query(ctx, `
		SELECT entity_type, count(*)
		  FROM taggable
		 WHERE tag_id = $1 AND entity_type = ANY($2)
		 GROUP BY entity_type`, id, advertisedTagTypes)
	if err != nil {
		return TagUsage{}, fmt.Errorf("collections: counting tag usage: %w", err)
	}
	defer rows.Close()
	var out TagUsage
	for rows.Next() {
		var entityType string
		var n int
		if scanErr := rows.Scan(&entityType, &n); scanErr != nil {
			return TagUsage{}, scanErr
		}
		switch entityType {
		case "person":
			out.People = n
		case "organization":
			out.Companies = n
		case "deal":
			out.Deals = n
		}
	}
	return out, rows.Err()
}

// TagUpdate is the partial a rename carries. A nil field is left alone; a
// non-nil one holding a nil pointer clears the column.
type TagUpdate struct {
	Name        *string
	Color       **string
	Description **string
}

// UpdateTag renames, recolours or describes a tag.
//
// `expectedVersion` is the If-Match precondition: zero means the caller did
// not send one and accepts last-write-wins, which the contract discourages but
// admits. A mismatch is version skew, not a missing row — the difference
// matters to a client deciding whether to re-read or give up.
func (s *Store) UpdateTag(ctx context.Context, id ids.TagID, in TagUpdate, expectedVersion int64) (tagRow, error) {
	if err := auth.Require(ctx, "tag", principal.ActionUpdate); err != nil {
		return tagRow{}, err
	}
	var name *string
	if in.Name != nil {
		normalized, err := ValidateTagName(*in.Name)
		if err != nil {
			return tagRow{}, err
		}
		name = &normalized
	}
	var out tagRow
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		before, err := scanTag(tx.QueryRow(ctx, "SELECT "+tagColumns+" FROM tag WHERE id = $1", id))
		if errors.Is(err, pgx.ErrNoRows) {
			return apperrors.ErrNotFound
		}
		if err != nil {
			return err
		}
		if expectedVersion != 0 && before.Version != expectedVersion {
			return apperrors.ErrVersionSkew
		}
		row := tx.QueryRow(ctx, `
			UPDATE tag
			   SET name        = COALESCE($2, name),
			       color       = CASE WHEN $3::boolean THEN $4 ELSE color END,
			       description = CASE WHEN $5::boolean THEN $6 ELSE description END,
			       version     = version + 1,
			       updated_at  = now()
			 WHERE id = $1
			RETURNING `+tagColumns,
			id, name,
			in.Color != nil, derefOrNil(in.Color),
			in.Description != nil, derefOrNil(in.Description))
		if out, err = scanTag(row); err != nil {
			if constraint, ok := storekit.UniqueViolation(err); ok && constraint == "uq_tag_name" {
				return fmt.Errorf("a tag already holds that name: %w", apperrors.ErrConflict)
			}
			return err
		}
		_, err = storekit.Audit(ctx, tx, "update", "tag", id.UUID,
			map[string]any{"name": before.Name},
			map[string]any{"name": out.Name})
		return err
	})
	return out, err
}

// derefOrNil unwraps the outer pointer of a clearable field. The two levels
// are the difference between "leave it" and "clear it", which one level cannot
// express.
func derefOrNil(v **string) *string {
	if v == nil {
		return nil
	}
	return *v
}

// RestoreTag brings an archived word back.
//
// It refuses when a LIVE tag has taken the name meanwhile. uq_tag_name binds
// live rows only, so the archived row and the new one coexist happily — the
// collision appears exactly at the moment of restoring, and the constraint is
// what catches it. The refusal says which situation the caller is in, because
// "conflict" alone does not tell them they have to rename one of two words
// they can both see.
func (s *Store) RestoreTag(ctx context.Context, id ids.TagID) (tagRow, error) {
	if err := auth.Require(ctx, "tag", principal.ActionUpdate); err != nil {
		return tagRow{}, err
	}
	var out tagRow
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		// Read the archived instant first: the audit row for an update has to
		// say what the field held, and after the UPDATE it holds NULL.
		before, err := scanTag(tx.QueryRow(ctx, "SELECT "+tagColumns+" FROM tag WHERE id = $1", id))
		if errors.Is(err, pgx.ErrNoRows) {
			return apperrors.ErrNotFound
		}
		if err != nil {
			return err
		}
		row := tx.QueryRow(ctx, `
			UPDATE tag SET archived_at = NULL, version = version + 1, updated_at = now()
			 WHERE id = $1 AND archived_at IS NOT NULL
			RETURNING `+tagColumns, id)
		if out, err = scanTag(row); errors.Is(err, pgx.ErrNoRows) {
			// The tag exists but was never archived: there is nothing here to
			// restore, and saying so is more useful than reporting success.
			return apperrors.ErrNotFound
		} else if err != nil {
			if constraint, ok := storekit.UniqueViolation(err); ok && constraint == "uq_tag_name" {
				return fmt.Errorf(
					"a live tag already holds this name; rename it before restoring this one: %w",
					apperrors.ErrConflict)
			}
			return err
		}
		_, err = storekit.Audit(ctx, tx, "update", "tag", id.UUID,
			map[string]any{"archived_at": before.ArchivedAt},
			map[string]any{"archived_at": nil, "name": out.Name})
		return err
	})
	return out, err
}

// MergeResult is what a merge did, in the two numbers that differ.
type MergeResult struct {
	Moved     int
	Collapsed int
}

// MergeTags folds one tag into another: every record carrying the source ends
// up carrying the target, the source is archived, and its name is released.
//
// Moved and collapsed are counted separately because they are different facts.
// A record that carried only the source is MOVED — the target gains it. One
// that already carried both COLLAPSES — its duplicate tagging is dropped and
// the target gains nothing. An admin reading "12 moved, 3 collapsed" knows the
// target grew by 12; a single total of 15 would tell them the wrong number.
//
// The source's NAME IS RELEASED rather than kept as an alias. That is a
// deliberate product decision and it has a cost the caller has to be told
// about: links to the old tag stop working, searching its name finds nothing,
// and somebody can coin the same word again next month.
func (s *Store) MergeTags(ctx context.Context, source, target ids.TagID) (MergeResult, error) {
	if err := auth.Require(ctx, "tag", principal.ActionUpdate); err != nil {
		return MergeResult{}, err
	}
	if source == target {
		return MergeResult{}, &BadInputError{Field: "into_tag_id", Reason: "must name a different tag"}
	}
	var out MergeResult
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		if err := requireLiveTag(ctx, tx, target, "into_tag_id"); err != nil {
			return err
		}
		sourceRow, err := scanTag(tx.QueryRow(ctx, "SELECT "+tagColumns+" FROM tag WHERE id = $1", source))
		if errors.Is(err, pgx.ErrNoRows) {
			return apperrors.ErrNotFound
		}
		if err != nil {
			return err
		}
		// Count the overlap BEFORE moving anything: once the update runs, a
		// record that carried both is indistinguishable from one that carried
		// only the source.
		if err := tx.QueryRow(ctx, `
			SELECT count(*) FROM taggable s
			 WHERE s.tag_id = $1
			   AND EXISTS (SELECT 1 FROM taggable t
			                WHERE t.tag_id = $2 AND t.entity_type = s.entity_type
			                  AND t.entity_id = s.entity_id)`,
			source, target).Scan(&out.Collapsed); err != nil {
			return err
		}
		moved, err := tx.Exec(ctx, `
			UPDATE taggable SET tag_id = $2
			 WHERE tag_id = $1
			   AND NOT EXISTS (SELECT 1 FROM taggable t
			                    WHERE t.tag_id = $2 AND t.entity_type = taggable.entity_type
			                      AND t.entity_id = taggable.entity_id)`, source, target)
		if err != nil {
			return err
		}
		out.Moved = int(moved.RowsAffected())
		// The duplicates the update skipped: the record already carries the
		// target, so the source's row is redundant rather than movable.
		if _, err := tx.Exec(ctx, `DELETE FROM taggable WHERE tag_id = $1`, source); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			UPDATE tag SET archived_at = now(), version = version + 1, updated_at = now()
			 WHERE id = $1 AND archived_at IS NULL`, source); err != nil {
			return err
		}
		_, err = storekit.Audit(ctx, tx, "update", "tag", source.UUID,
			map[string]any{"name": sourceRow.Name},
			map[string]any{
				"merged_into": target.UUID,
				"moved":       out.Moved,
				"collapsed":   out.Collapsed,
			})
		return err
	})
	return out, err
}

// requireLiveTag refuses a tag that is missing or retired, naming the field
// that carried it so the caller knows which id to fix.
func requireLiveTag(ctx context.Context, tx pgx.Tx, id ids.TagID, field string) error {
	var archived *string
	err := tx.QueryRow(ctx, `SELECT archived_at::text FROM tag WHERE id = $1`, id).Scan(&archived)
	if errors.Is(err, pgx.ErrNoRows) {
		return &BadInputError{Field: field, Reason: "no such tag"}
	}
	if err != nil {
		return err
	}
	if archived != nil {
		return &BadInputError{Field: field, Reason: "names a tag that was archived"}
	}
	return nil
}
