// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package collections

// Reading tag suggestions. The Worklist lane, its count, the card and both
// decisions all start from visibleTagSuggestionsQuery. So none of them can show
// a suggestion another would hide.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// TagSuggestion is one suggestion as a reader sees it.
type TagSuggestion struct {
	ID         ids.UUID
	State      string
	TagID      ids.TagID
	TagName    string
	TagColor   *string
	EntityType string
	EntityID   ids.UUID
	EntityName string
	CreatedAt  time.Time
	Evidence   []TagSuggestionEvidenceItem
}

// TagSuggestionEvidenceItem is one cited activity with what a reader
// recognises it by. Subject is nil when the activity has none or it was
// redacted.
type TagSuggestionEvidenceItem struct {
	ActivityID ids.UUID  `json:"activity_id"`
	Kind       string    `json:"kind"`
	Subject    *string   `json:"subject"`
	OccurredAt time.Time `json:"occurred_at"`
}

const tagSuggestionMaxLimit = 200

// tagSuggestionRead says which suggestions a read admits and what it returns.
type tagSuggestionRead struct {
	// openOnly admits only an open suggestion that still stands. A decision
	// reads without it, so it can tell a decided suggestion (409) from one the
	// caller may not see (404).
	openOnly bool
	// evidence returns the cited activities in the same statement.
	evidence bool
}

// visibleTagSuggestionsQuery selects the suggestions a reader may see: one who
// may read the live record and every activity it cites. A suggestion built
// from mail only its owner can read is therefore shown to that owner alone. A
// cited activity that is gone, archived or restricted hides the suggestion.
// The evidence, when asked for, is read in this same statement, so it is the
// evidence this statement authorized.
func visibleTagSuggestionsQuery(ctx context.Context, arg func(any) int, read tagSuggestionRead) (string, error) {
	if err := auth.Require(ctx, "tag", principal.ActionRead); err != nil {
		return "", err
	}
	contact, err := recordArm(ctx, auth.ReadGranted(ctx, "contact"), "contact", "vc", arg)
	if err != nil {
		return "", err
	}
	company, err := recordArm(ctx, auth.ReadGranted(ctx, "company"), "company", "vco", arg)
	if err != nil {
		return "", err
	}
	content, err := auth.ActivityContentClause(ctx, "va", arg)
	if err != nil {
		return "", err
	}
	if !auth.ReadGranted(ctx, "activity") {
		content = "false"
	}
	evidence := `'[]'::jsonb`
	if read.evidence {
		evidence = tagSuggestionEvidenceJSON
	}
	query := `SELECT s.id, s.state, s.tag_id, t.name, t.color,
	       CASE WHEN s.contact_id IS NOT NULL THEN 'contact' ELSE 'company' END,
	       coalesce(s.contact_id, s.company_id), coalesce(vc.full_name, vco.display_name, ''), s.created_at,
	       ` + evidence + `
	  FROM tag_suggestion s JOIN tag t ON t.id = s.tag_id AND t.archived_at IS NULL
	  LEFT JOIN contact vc ON vc.id = s.contact_id AND vc.archived_at IS NULL AND ` + contact + `
	  LEFT JOIN company vco ON vco.id = s.company_id AND vco.archived_at IS NULL AND ` + company + `
	 WHERE (vc.id IS NOT NULL OR vco.id IS NOT NULL)
	   AND (SELECT count(*) FROM tag_suggestion_evidence vn WHERE vn.suggestion_id = s.id) = s.evidence_count
	   AND NOT EXISTS (SELECT 1 FROM tag_suggestion_evidence ve JOIN activity va ON va.id = ve.activity_id
	         WHERE ve.suggestion_id = s.id
	           AND NOT (va.archived_at IS NULL AND ` + auth.ActivityAvailableClause("va") + ` AND ` + content + `))`
	if read.openOnly {
		query += ` AND s.state = 'open' AND ` + tagSuggestionStandsClause("s")
	}
	return query, nil
}

// tagSuggestionEvidenceJSON is a suggestion's cited activities, newest first.
// A redacted subject reads as absent.
const tagSuggestionEvidenceJSON = `(SELECT coalesce(jsonb_agg(jsonb_build_object(
	         'activity_id', ee.activity_id, 'kind', ea.kind, 'occurred_at', ee.occurred_at,
	         'subject', CASE WHEN 'subject' = ANY(ea.redacted_fields) THEN NULL ELSE ea.subject END)
	         ORDER BY ee.occurred_at DESC, ee.activity_id), '[]'::jsonb)
	    FROM tag_suggestion_evidence ee JOIN activity ea ON ea.id = ee.activity_id
	   WHERE ee.suggestion_id = s.id)`

// recordArm is the reader's row scope over one record type as a join
// condition, or false when they hold no read grant on it.
func recordArm(ctx context.Context, granted bool, table, alias string, arg func(any) int) (string, error) {
	if !granted {
		return "false", nil
	}
	clause, err := auth.ScopeClauseFor(ctx, table, alias, arg)
	if err != nil || clause != "" {
		return clause, err
	}
	return predicateAlways, nil
}

func scanTagSuggestion(row pgx.Row) (TagSuggestion, error) {
	var s TagSuggestion
	var evidence []byte
	if err := row.Scan(&s.ID, &s.State, &s.TagID, &s.TagName, &s.TagColor,
		&s.EntityType, &s.EntityID, &s.EntityName, &s.CreatedAt, &evidence); err != nil {
		return s, err
	}
	return s, json.Unmarshal(evidence, &s.Evidence)
}

// OpenTagSuggestions reads up to limit open suggestions this reader may see,
// newest first.
func (s *Store) OpenTagSuggestions(ctx context.Context, limit int) ([]TagSuggestion, error) {
	if limit <= 0 || limit > tagSuggestionMaxLimit {
		limit = tagSuggestionMaxLimit
	}
	var out []TagSuggestion
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		var args []any
		arg := func(v any) int { args = append(args, v); return len(args) }
		visible, err := visibleTagSuggestionsQuery(ctx, arg, tagSuggestionRead{openOnly: true})
		if err != nil {
			return err
		}
		rows, err := tx.Query(ctx, visible+`
			 ORDER BY s.created_at DESC, s.id DESC
			 LIMIT $`+fmt.Sprint(arg(limit)), args...)
		if err != nil {
			return fmt.Errorf("collections: reading open tag suggestions: %w", err)
		}
		out, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (TagSuggestion, error) {
			return scanTagSuggestion(row)
		})
		return err
	})
	return out, err
}

// CountOpenTagSuggestions counts what OpenTagSuggestions would show without a
// limit.
func (s *Store) CountOpenTagSuggestions(ctx context.Context) (int, error) {
	var n int
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		var args []any
		visible, err := visibleTagSuggestionsQuery(ctx, func(v any) int { args = append(args, v); return len(args) },
			tagSuggestionRead{openOnly: true})
		if err != nil {
			return err
		}
		return tx.QueryRow(ctx, `SELECT count(*) FROM (`+visible+`) open`, args...).Scan(&n)
	})
	return n, err
}

// GetTagSuggestion reads one open, standing suggestion this reader may see,
// with its evidence.
func (s *Store) GetTagSuggestion(ctx context.Context, id ids.UUID) (TagSuggestion, error) {
	var out TagSuggestion
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		var err error
		out, err = readVisibleTagSuggestionTx(ctx, tx, id, tagSuggestionRead{openOnly: true, evidence: true}, false)
		return err
	})
	return out, err
}

// readVisibleTagSuggestionTx reads one suggestion and its evidence in one
// statement under the caller's visibility, locking it when lock is set. One
// the caller may not see answers ErrNotFound, so its existence stays hidden.
func readVisibleTagSuggestionTx(ctx context.Context, tx pgx.Tx, id ids.UUID, read tagSuggestionRead, lock bool) (TagSuggestion, error) {
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	visible, err := visibleTagSuggestionsQuery(ctx, arg, read)
	if err != nil {
		return TagSuggestion{}, err
	}
	query := visible + ` AND s.id = $` + fmt.Sprint(arg(id))
	if lock {
		query += ` FOR UPDATE OF s`
	}
	out, err := scanTagSuggestion(tx.QueryRow(ctx, query, args...))
	if errors.Is(err, pgx.ErrNoRows) {
		return TagSuggestion{}, apperrors.ErrNotFound
	}
	if err != nil {
		return TagSuggestion{}, fmt.Errorf("collections: reading a tag suggestion: %w", err)
	}
	return out, nil
}
