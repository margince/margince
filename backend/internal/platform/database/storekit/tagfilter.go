// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// The tag filter every list surface shares.
//
// People, companies and deals all narrow by tag, and the three predicates are
// the same shape over one polymorphic table. Written once here, in the layer
// each module already depends on, because three copies of a NOT EXISTS is
// three chances for one of them to mean something subtly different — and the
// mode that would drift first is `none`, the one nobody notices is wrong until
// a rep is looking at a record they filtered out.

// TagMode says how several tag ids combine.
type TagMode string

const (
	// TagModeAny selects a record carrying at least one of the named tags.
	TagModeAny TagMode = "any"
	// TagModeAll selects a record carrying every one of them.
	TagModeAll TagMode = "all"
	// TagModeNone selects a record carrying not one of them.
	TagModeNone TagMode = "none"
)

// ParseTagMode reads a wire value, defaulting an absent one to `any`.
//
// An unknown mode is refused rather than defaulted: silently treating a typo
// as `any` would hand back a wider slice than the caller asked for, and a
// filter that quietly widens is worse than one that fails.
func ParseTagMode(raw *string) (TagMode, error) {
	if raw == nil || *raw == "" {
		return TagModeAny, nil
	}
	switch TagMode(*raw) {
	case TagModeAny, TagModeAll, TagModeNone:
		return TagMode(*raw), nil
	default:
		return "", fmt.Errorf("tag_mode %q: want any, all or none", *raw)
	}
}

// TagFilterClause renders the predicate narrowing one entity's rows by tag,
// or "" when no tag was named.
//
// `taggableType` is the value `taggable.entity_type` stores, which is neither
// an RBAC object nor a table name even where all three read the same. The
// column has its own closed vocabulary and this names a member of it.
//
// `idColumn` is how the outer query names the record's own id — "person.id",
// "o.id" — because each list builds its own FROM and this has to attach to it.
//
// EXISTS rather than a join, in every mode: a record carries many tags, and a
// join would return it once per matching link — rows a keyset cursor would
// then page over as if they were distinct records.
//
// Archived tags are excluded. A word somebody retired is not in the picker, so
// a filter naming it selects a slice no reader can construct or explain; and
// after a merge releases a name, a re-coined word would otherwise drag along
// the records carrying the older retired tag of the same spelling.
func TagFilterClause(ctx context.Context, taggableType, idColumn string, tagIDs []ids.UUID, mode TagMode, arg func(any) int) string {
	if len(tagIDs) == 0 {
		return ""
	}
	// Filtering by a tag reads the vocabulary: a caller who cannot see the
	// words must not be able to learn which records carry one by watching a
	// page shrink. Rendering nothing would WIDEN the answer, so the refusal is
	// a predicate that selects no rows.
	if auth.Require(ctx, "tag", principal.ActionRead) != nil {
		return "FALSE"
	}
	switch mode {
	case TagModeAll:
		// One EXISTS per tag: a single subquery cannot say "carries all of
		// these" without counting, and counting reads worse than the shape a
		// reader can check tag by tag.
		parts := make([]string, 0, len(tagIDs))
		for _, id := range tagIDs {
			parts = append(parts, tagExists(taggableType, idColumn, []ids.UUID{id}, arg))
		}
		return "(" + strings.Join(parts, " AND ") + ")"
	case TagModeNone:
		return "NOT " + tagExists(taggableType, idColumn, tagIDs, arg)
	default:
		return tagExists(taggableType, idColumn, tagIDs, arg)
	}
}

// tagExists renders one EXISTS over the taggable link for these ids.
//
// An ARCHIVED id matches nothing here, and that reads differently per mode:
// `any` and `all` narrow to nothing, `none` keeps every record. Both follow
// from one rule — a retired word is not part of the vocabulary a filter can
// name — and the alternative is worse. Honouring an archived id would let a
// saved view keep selecting by a word an admin retired precisely to stop
// people selecting by it, and after a merge releases a name the re-coined word
// would drag the old tag's records along.
//
// The picker never offers a retired word, so reaching this state means a saved
// view outlived the tag it names. That view then answers with nothing rather
// than with a slice its reader cannot explain.
func tagExists(taggableType, idColumn string, tagIDs []ids.UUID, arg func(any) int) string {
	return fmt.Sprintf(`EXISTS (
		SELECT 1 FROM taggable tg
		  JOIN tag t ON t.id = tg.tag_id
		WHERE tg.entity_type = $%d AND tg.entity_id = %s
		  AND tg.tag_id = ANY($%d) AND t.archived_at IS NULL)`,
		arg(taggableType), idColumn, arg(tagIDs))
}

// RowTag is one tag as a list row carries it — the word and its colour, and
// nothing about who applied it. A page of fifty rows draws fifty chips and
// needs no assignments to do it.
type RowTag struct {
	TagID ids.UUID
	Name  string
	Color *string
}

// rowTagCap bounds how many tags one row carries back.
//
// A chip strip shows a few and says "+N", so the row does not need every word
// to draw one — and a record somebody tagged forty times would otherwise make
// one page of fifty rows into two thousand joined rows. The count a strip
// needs beyond the cap comes from the record's own tags read, which is the
// screen that has room for it.
const rowTagCap = 5

// AttachRowTags loads the live tags for one page of records, in ONE query, and
// hands each row its own.
//
// Batched deliberately: the obvious per-row read is N queries for a page of N,
// which is the shape that makes a list feel slow and shows up nowhere in a
// unit test. `taggableType` is the value taggable.entity_type stores.
//
// Archived tags are omitted. A retired word is not drawn on a list, and a row
// showing one would send a reader to a picker that no longer offers it.
func AttachRowTags[T any](
	ctx context.Context, tx pgx.Tx, taggableType string,
	rows []T, id func(T) ids.UUID, set func(*T, []RowTag),
) error {
	if len(rows) == 0 {
		return nil
	}
	// The vocabulary is its own grant. A caller who may read these RECORDS and
	// not the words gets rows with no chips — the same withholding the
	// record's own tags read performs, which a list that handed the words out
	// anyway would make pointless.
	if auth.Require(ctx, "tag", principal.ActionRead) != nil {
		return nil
	}
	recordIDs := make([]ids.UUID, len(rows))
	for i := range rows {
		recordIDs[i] = id(rows[i])
	}
	// The window is what keeps the cap per RECORD rather than per page: a
	// plain LIMIT would hand every tag to the first row and none to the rest.
	found, err := tx.Query(ctx, `
		SELECT entity_id, tag_id, name, color FROM (
			SELECT g.entity_id, t.id AS tag_id, t.name, t.color,
			       row_number() OVER (PARTITION BY g.entity_id ORDER BY t.name, t.id) AS rank
			  FROM taggable g
			  JOIN tag t ON t.id = g.tag_id
			 WHERE g.entity_type = $1 AND g.entity_id = ANY($2) AND t.archived_at IS NULL
		) ranked
		 WHERE rank <= $3`, taggableType, recordIDs, rowTagCap)
	if err != nil {
		return fmt.Errorf("storekit: reading row tags for %s: %w", taggableType, err)
	}
	defer found.Close()

	byRecord := make(map[ids.UUID][]RowTag, len(rows))
	for found.Next() {
		var recordID ids.UUID
		var tag RowTag
		if err := found.Scan(&recordID, &tag.TagID, &tag.Name, &tag.Color); err != nil {
			return fmt.Errorf("storekit: scanning a row tag: %w", err)
		}
		byRecord[recordID] = append(byRecord[recordID], tag)
	}
	if err := found.Err(); err != nil {
		return err
	}
	for i := range rows {
		if tags := byRecord[id(rows[i])]; len(tags) > 0 {
			set(&rows[i], tags)
		}
	}
	return nil
}
