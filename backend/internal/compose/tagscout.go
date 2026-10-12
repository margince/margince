// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The tag scout proposes a suggestible tag on the contact or company that a
// captured mail, meeting, note or call is filed under. It fires on a phrase
// from the tag's description (split on commas, semicolons and line breaks).
// A phrase match is cheap and explainable, and it never mints a word. Private
// mail counts, because collections shows each suggestion only to a reader who
// may read every activity it cites.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/collections"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const (
	// tagScoutWindow is how far back evidence counts.
	tagScoutWindow = 30 * 24 * time.Hour
	// tagScoutSuggestionCap bounds how many suggestions one pass raises; a
	// suggestion raised leaves the candidate set, so the next pass reaches the
	// rest.
	tagScoutSuggestionCap = 200
	// tagScoutItemCap bounds the activities one suggestion cites.
	tagScoutItemCap = 5
)

// TagScoutPass is what one pass did, for the log line.
type TagScoutPass struct {
	Superseded, Considered, Raised int
}

// tagScoutRow is one cited activity for one tag on one record.
type tagScoutRow struct {
	tagID      ids.TagID
	entityType string
	record     ids.UUID
	evidence   collections.TagSuggestionEvidence
}

// RunTagScout runs one pass inside the caller's transaction, as the system
// principal the job binds.
func RunTagScout(ctx context.Context, tx pgx.Tx, now time.Time) (TagScoutPass, error) {
	var pass TagScoutPass
	var err error
	if pass.Superseded, err = collections.SupersedeStaleTagSuggestionsTx(ctx, tx); err != nil {
		return pass, err
	}
	query, args := tagScoutSQL(now.Add(-tagScoutWindow), now)
	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return pass, fmt.Errorf("tag scout: reading the evidence: %w", err)
	}
	found, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (tagScoutRow, error) {
		var r tagScoutRow
		err := row.Scan(&r.tagID, &r.entityType, &r.record, &r.evidence.ActivityID, &r.evidence.OccurredAt)
		return r, err
	})
	if err != nil {
		return pass, fmt.Errorf("tag scout: scanning the evidence: %w", err)
	}
	for _, draft := range tagScoutDrafts(found) {
		if pass.Considered == tagScoutSuggestionCap {
			break
		}
		pass.Considered++
		raised, err := collections.RecordTagSuggestionTx(ctx, tx, draft)
		if err != nil {
			return pass, err
		}
		if raised {
			pass.Raised++
		}
	}
	return pass, nil
}

// tagScoutDrafts groups the rows, which arrive ordered by tag and record.
func tagScoutDrafts(found []tagScoutRow) []collections.TagSuggestionDraft {
	var out []collections.TagSuggestionDraft
	for _, r := range found {
		if n := len(out); n > 0 && out[n-1].TagID == r.tagID &&
			out[n-1].EntityType == r.entityType && out[n-1].EntityID == r.record {
			out[n-1].Evidence = append(out[n-1].Evidence, r.evidence)
			continue
		}
		out = append(out, collections.TagSuggestionDraft{
			TagID: r.tagID, EntityType: r.entityType, EntityID: r.record,
			Evidence: []collections.TagSuggestionEvidence{r.evidence},
		})
	}
	return out
}

// tagScoutSQL is the pass's one read. It pairs each suggestible tag's phrases
// with the captured activities in the window that mention them. It keeps
// records the tag may be proposed on, and evidence newer than the last decision.
func tagScoutSQL(since, now time.Time) (string, []any) {
	args := []any{since, now, tagScoutItemCap}
	candidate := func(entityType string) string {
		return `(ev.entity_type = '` + entityType + `' AND ` +
			collections.SuggestableTagClause(entityType, "ev.tag_id", "ev.record") +
			` AND ev.occurred_at > coalesce(` + collections.TagSuggestionFloorExpr(entityType, "ev.tag_id", "ev.record") +
			`, '-infinity'::timestamptz))`
	}
	query := `
	WITH phrases AS (
	  SELECT DISTINCT t.id AS tag_id, lower(btrim(phrase)) AS phrase
	    FROM tag t, regexp_split_to_table(t.description, '[,;\n]') AS phrase
	   WHERE t.archived_at IS NULL AND t.suggestible AND char_length(btrim(phrase)) >= 3
	), ev AS (
	  SELECT DISTINCT p.tag_id, l.entity_type, coalesce(l.contact_id, l.company_id) AS record,
	         a.id AS activity_id, a.occurred_at
	    FROM activity a
	    JOIN activity_link l ON l.activity_id = a.id AND l.entity_type IN ('contact', 'company')
	    JOIN phrases p ON position(p.phrase IN lower(coalesce(a.subject, '') || ' ' || coalesce(a.body, ''))) > 0
	   WHERE a.kind IN ('email', 'meeting', 'note', 'call')
	     AND a.archived_at IS NULL AND a.restricted_at IS NULL
	     AND a.capture_label IS DISTINCT FROM 'noise'
	     AND a.occurred_at > $1 AND a.occurred_at <= $2
	), ranked AS (
	  SELECT ev.*, row_number() OVER (PARTITION BY ev.tag_id, ev.entity_type, ev.record
	                                  ORDER BY ev.occurred_at DESC, ev.activity_id) AS rank
	    FROM ev
	   WHERE ` + candidate("contact") + ` OR ` + candidate("company") + `
	)
	SELECT tag_id, entity_type, record, activity_id, occurred_at
	  FROM ranked WHERE rank <= $3
	 ORDER BY tag_id, entity_type, record, occurred_at DESC, activity_id`
	return query, args
}
