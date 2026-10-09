// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package collections

// Reading tag suggestions. The Worklist lane, its count, the card and both
// decisions all start from visibleTagSuggestionsQuery. So none of them can show
// a suggestion another would hide.

import (
	"context"
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
	ActivityID ids.UUID
	Kind       string
	Subject    *string
	OccurredAt time.Time
}

const tagSuggestionMaxLimit = 200

// visibleTagSuggestionsQuery selects the suggestions a reader may see: one who
// may read the record and every activity it cites. A suggestion built from mail
// only its owner can read is therefore shown to that owner alone. An open
// suggestion must also still stand (tagSuggestionStandsClause). The record's
// name is selected only through the same record grant and row scope.
func visibleTagSuggestionsQuery(ctx context.Context, arg func(any) int) (string, error) {
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
	return `SELECT s.id, s.state, s.tag_id, t.name, t.color,
	       CASE WHEN s.contact_id IS NOT NULL THEN 'contact' ELSE 'company' END,
	       coalesce(s.contact_id, s.company_id), coalesce(vc.full_name, vco.display_name, ''), s.created_at
	  FROM tag_suggestion s JOIN tag t ON t.id = s.tag_id
	  LEFT JOIN contact vc ON vc.id = s.contact_id AND ` + contact + `
	  LEFT JOIN company vco ON vco.id = s.company_id AND ` + company + `
	 WHERE (s.state <> 'open' OR ` + tagSuggestionStandsClause("s") + `)
	   AND (vc.id IS NOT NULL OR vco.id IS NOT NULL)
	   AND NOT EXISTS (SELECT 1 FROM tag_suggestion_evidence ve JOIN activity va ON va.id = ve.activity_id
	         WHERE ve.suggestion_id = s.id AND NOT (` + content + `))`, nil
}

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
	return "true", nil
}

func scanTagSuggestion(row pgx.Row) (TagSuggestion, error) {
	var s TagSuggestion
	err := row.Scan(&s.ID, &s.State, &s.TagID, &s.TagName, &s.TagColor,
		&s.EntityType, &s.EntityID, &s.EntityName, &s.CreatedAt)
	return s, err
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
		visible, err := visibleTagSuggestionsQuery(ctx, arg)
		if err != nil {
			return err
		}
		rows, err := tx.Query(ctx, visible+` AND s.state = 'open'
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
		visible, err := visibleTagSuggestionsQuery(ctx, func(v any) int { args = append(args, v); return len(args) })
		if err != nil {
			return err
		}
		return tx.QueryRow(ctx, `SELECT count(*) FROM (`+visible+` AND s.state = 'open') open`, args...).Scan(&n)
	})
	return n, err
}

// GetTagSuggestion reads one suggestion this reader may see, with its evidence.
func (s *Store) GetTagSuggestion(ctx context.Context, id ids.UUID) (TagSuggestion, error) {
	var out TagSuggestion
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		var err error
		out, err = readVisibleTagSuggestionTx(ctx, tx, id, false)
		return err
	})
	return out, err
}

// readVisibleTagSuggestionTx reads one suggestion under the caller's
// visibility, locking it when lock is set. One the caller may not see answers
// ErrNotFound, so its existence stays hidden.
func readVisibleTagSuggestionTx(ctx context.Context, tx pgx.Tx, id ids.UUID, lock bool) (TagSuggestion, error) {
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	visible, err := visibleTagSuggestionsQuery(ctx, arg)
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
	rows, err := tx.Query(ctx, `
		SELECT e.activity_id, a.kind,
		       CASE WHEN 'subject' = ANY(a.redacted_fields) THEN NULL ELSE a.subject END, e.occurred_at
		  FROM tag_suggestion_evidence e JOIN activity a ON a.id = e.activity_id
		 WHERE e.suggestion_id = $1
		 ORDER BY e.occurred_at DESC, e.activity_id`, id)
	if err != nil {
		return TagSuggestion{}, fmt.Errorf("collections: reading tag suggestion evidence: %w", err)
	}
	out.Evidence, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (TagSuggestionEvidenceItem, error) {
		var e TagSuggestionEvidenceItem
		return e, row.Scan(&e.ActivityID, &e.Kind, &e.Subject, &e.OccurredAt)
	})
	if err != nil {
		return TagSuggestion{}, fmt.Errorf("collections: reading tag suggestion evidence: %w", err)
	}
	return out, nil
}
