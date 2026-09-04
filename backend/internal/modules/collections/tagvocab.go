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

// The three record types the product offers tags on. `taggable` admits lead
// and project too — the column has carried five since the baseline — but
// nothing in V1 shows or filters those, so counting them here would report a
// weight no screen can explain.
//
// Named rather than repeated because the list and the switch that reads it
// have to agree: a type counted in one and missing from the other reports zero
// for records that carry the tag.
// uqTagName is the uniqueness index whose violation means "another tag holds
// this name". It is spelled here rather than at each call site because it is
// the migration's identifier, not this package's: a rename there has to fail
// this compile.
const uqTagName = "uq_tag_name"

// intoTagIDField names the body field a merge's target rides in. It is the
// wire's spelling, quoted back to the caller in a refusal, so it belongs
// beside the code that refuses rather than retyped at each.
const intoTagIDField = "into_tag_id"

// nameField is the audit image's key for a tag's name. The three writers here
// all record it, and an image keyed differently in one of them would read as a
// different field to whoever queries the trail.
const nameField = "name"

const (
	typePerson       = "person"
	typeOrganization = "organization"
	typeDeal         = "deal"
)

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

// tagUsage counts, per advertised type, the tagged records THIS CALLER MAY SEE.
//
// One query per type rather than one grouped pass over `taggable`, because the
// visibility rule is per table: a rep may see every company and only their own
// people. Counting the link rows alone would be cheaper and would report how
// many records carry the word regardless of who is asking — which discloses
// the existence of rows the caller cannot open, and would make the number on
// screen one they cannot reconcile with the list behind it.
func tagUsage(ctx context.Context, tx pgx.Tx, id ids.TagID) (TagUsage, error) {
	var out TagUsage
	// The entity_type a tagging carries and the table it points at are the same
	// word for all three, which is why one name serves as both below.
	for _, c := range []struct {
		entityType string
		into       *int
	}{
		{typePerson, &out.People},
		{typeOrganization, &out.Companies},
		{typeDeal, &out.Deals},
	} {
		n, err := countVisibleTagged(ctx, tx, id, c.entityType)
		if errors.Is(err, apperrors.ErrPermissionDenied) {
			// No grant on that type is not "none of them carry it": the caller
			// may not read the type at all, and a number covering rows they
			// cannot list would tell them those rows exist. Zero is the honest
			// answer to a caller who is not admitted to look.
			continue
		}
		if err != nil {
			return TagUsage{}, err
		}
		*c.into = n
	}
	return out, nil
}

// countVisibleTagged counts one type's tagged records under that type's own
// row-scope predicate.
func countVisibleTagged(ctx context.Context, tx pgx.Tx, id ids.TagID, entityType string) (int, error) {
	// The OBJECT grant first. ScopeClauseFor renders a row predicate and is not
	// a gate: a seat holding tag.read and no person.read would otherwise be
	// counted the people it may not list, and a count of rows a caller cannot
	// open discloses that they exist.
	if err := auth.Require(ctx, entityType, principal.ActionRead); err != nil {
		return 0, err
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	tagPos, typePos := arg(id), arg(entityType)
	scope, err := auth.ScopeClauseFor(ctx, entityType, "r", arg)
	if err != nil {
		return 0, fmt.Errorf("collections: scoping %s tag usage: %w", entityType, err)
	}
	if scope == "" {
		// An unbounded caller sees every row, and the empty clause is how the
		// helper says so — not a missing predicate to be defaulted open.
		scope = "TRUE"
	}
	var n int
	query := fmt.Sprintf(`
		SELECT count(*)
		  FROM taggable tg
		  JOIN %s r ON r.id = tg.entity_id
		 WHERE tg.tag_id = $%d AND tg.entity_type = $%d
		   AND r.archived_at IS NULL
		   AND (%s)`, pgx.Identifier{entityType}.Sanitize(), tagPos, typePos, scope)
	if err := tx.QueryRow(ctx, query, args...).Scan(&n); err != nil {
		return 0, fmt.Errorf("collections: counting %s tag usage: %w", entityType, err)
	}
	return n, nil
}

// CountTagReachBatch totals, per tag, the records THIS caller may see carrying
// it — one statement per record type for the whole page rather than three per
// hit, which is what a search page of tag hits would otherwise cost.
//
// The per-type structure is kept, not flattened: each type carries its own
// object grant and its own row-scope predicate, and a single query across
// types could honour neither. What is batched is the tags, not the rules.
//
// A tag absent from the returned map is one nothing visible carries; the
// caller decides whether that reads as zero or as unknown.
func CountTagReachBatch(ctx context.Context, tx pgx.Tx, tagIDs []ids.TagID) (map[ids.TagID]int, error) {
	if err := auth.Require(ctx, "tag", principal.ActionRead); err != nil {
		return nil, err
	}
	out := make(map[ids.TagID]int, len(tagIDs))
	if len(tagIDs) == 0 {
		return out, nil
	}
	for _, entityType := range []string{typePerson, typeOrganization, typeDeal} {
		counts, err := countVisibleTaggedBatch(ctx, tx, tagIDs, entityType)
		if errors.Is(err, apperrors.ErrPermissionDenied) {
			// Same rule as tagUsage: a type this caller may not read
			// contributes nothing rather than disclosing that its rows exist.
			continue
		}
		if err != nil {
			return nil, err
		}
		for tagID, n := range counts {
			out[tagID] += n
		}
	}
	return out, nil
}

// countVisibleTaggedBatch is countVisibleTagged over many tags at once, under
// the same object grant and the same row-scope predicate.
func countVisibleTaggedBatch(ctx context.Context, tx pgx.Tx, tagIDs []ids.TagID, entityType string) (map[ids.TagID]int, error) {
	if err := auth.Require(ctx, entityType, principal.ActionRead); err != nil {
		return nil, err
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	idsPos, typePos := arg(tagIDs), arg(entityType)
	scope, err := auth.ScopeClauseFor(ctx, entityType, "r", arg)
	if err != nil {
		return nil, fmt.Errorf("collections: scoping %s tag usage: %w", entityType, err)
	}
	if scope == "" {
		scope = "TRUE"
	}
	query := fmt.Sprintf(`
		SELECT tg.tag_id, count(*)
		  FROM taggable tg
		  JOIN %s r ON r.id = tg.entity_id
		 WHERE tg.tag_id = ANY($%d) AND tg.entity_type = $%d
		   AND r.archived_at IS NULL
		   AND (%s)
		 GROUP BY tg.tag_id`, pgx.Identifier{entityType}.Sanitize(), idsPos, typePos, scope)
	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("collections: counting %s tag usage: %w", entityType, err)
	}
	defer rows.Close()
	out := make(map[ids.TagID]int, len(tagIDs))
	for rows.Next() {
		var tagID ids.TagID
		var n int
		if err := rows.Scan(&tagID, &n); err != nil {
			return nil, fmt.Errorf("collections: reading %s tag usage: %w", entityType, err)
		}
		out[tagID] = n
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("collections: reading %s tag usage: %w", entityType, err)
	}
	return out, nil
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
		// Lock before reading: the version check below and the UPDATE that
		// follows it are two statements, and without the lock another writer
		// can land between them — the caller's If-Match would pass against a
		// version their write then overwrites.
		if _, err := storekit.LockRow(ctx, tx, "tag", id.UUID, storekit.IncludeArchived); err != nil {
			return err
		}
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
			       description = CASE WHEN $5::boolean THEN $6 ELSE description END
			 WHERE id = $1
			RETURNING `+tagColumns,
			id, name,
			in.Color != nil, derefOrNil(in.Color),
			in.Description != nil, derefOrNil(in.Description))
		if out, err = scanTag(row); err != nil {
			if constraint, ok := storekit.UniqueViolation(err); ok && constraint == uqTagName {
				return fmt.Errorf("a tag already holds that name: %w", apperrors.ErrConflict)
			}
			return err
		}
		_, err = storekit.Audit(ctx, tx, "update", "tag", id.UUID,
			map[string]any{nameField: before.Name},
			map[string]any{nameField: out.Name})
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
		// Lock first: two restores racing would both read an archived row and
		// both report success, and the audit trail would show the word coming
		// back twice.
		if _, err := storekit.LockRow(ctx, tx, "tag", id.UUID, storekit.IncludeArchived); err != nil {
			return err
		}
		// Read the archived instant: the audit row for an update has to say
		// what the field held, and after the UPDATE it holds NULL.
		before, err := scanTag(tx.QueryRow(ctx, "SELECT "+tagColumns+" FROM tag WHERE id = $1", id))
		if errors.Is(err, pgx.ErrNoRows) {
			return apperrors.ErrNotFound
		}
		if err != nil {
			return err
		}
		row := tx.QueryRow(ctx, `
			UPDATE tag SET archived_at = NULL
			 WHERE id = $1 AND archived_at IS NOT NULL
			RETURNING `+tagColumns, id)
		if out, err = scanTag(row); errors.Is(err, pgx.ErrNoRows) {
			// The tag exists but was never archived: there is nothing here to
			// restore, and saying so is more useful than reporting success.
			return apperrors.ErrNotFound
		} else if err != nil {
			if constraint, ok := storekit.UniqueViolation(err); ok && constraint == uqTagName {
				return fmt.Errorf(
					"a live tag already holds this name; rename it before restoring this one: %w",
					apperrors.ErrConflict)
			}
			return err
		}
		_, err = storekit.Audit(ctx, tx, "update", "tag", id.UUID,
			map[string]any{"archived_at": before.ArchivedAt},
			map[string]any{"archived_at": nil, nameField: out.Name})
		return err
	})
	return out, err
}
