// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// Reading suggestions. Every reader — the list, the count, the board, the
// Worklist and the company page — composes suggestionVisibleClause, so none of
// them can show a suggestion another would hide.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// suggestionVisibleClause admits a suggestion only to a reader who may read
// its company and EVERY piece of its evidence: a meeting through the activity
// content gate, a signal through the signal scope and the content gate of each
// message it cites, an attachment through the content gate of the message it
// came on. Reading a meeting or a document needs the activity grant and
// reading a signal the signal grant, so a reader without one sees no
// suggestion citing that kind. Every cited record must still be live, and every
// evidence row the suggestion was written with must still exist — so an
// erasure hides a suggestion rather than leaving it to a wider audience than it
// had.
//
// Under the system principal it is the liveness rule alone, which is how the
// superseding pass reads it: one spelling of "this suggestion still stands".
//
// Every caller names deal_suggestion "s" in its outer query.
func suggestionVisibleClause(ctx context.Context, arg func(any) int) (string, error) {
	if err := requireSuggestionRead(ctx); err != nil {
		return "", err
	}
	company, err := auth.ScopeClauseFor(ctx, "company", "sc", arg)
	if err != nil {
		return "", err
	}
	meeting, err := auth.ActivityContentClause(ctx, "ea", arg)
	if err != nil {
		return "", err
	}
	signal, err := auth.SignalScopeClause(ctx, "es", arg)
	if err != nil {
		return "", err
	}
	cited, err := auth.ActivityContentClause(ctx, "ca", arg)
	if err != nil {
		return "", err
	}
	carrier, err := auth.ActivityContentClause(ctx, "aa", arg)
	if err != nil {
		return "", err
	}
	// A kind the reader holds no grant for admits no evidence of that kind.
	activityGranted := grantedSQL(ctx, "activity")
	signalGranted := grantedSQL(ctx, "signal")
	return fmt.Sprintf(`(EXISTS (SELECT 1 FROM company sc
	         WHERE sc.id = %[1]s.company_id AND sc.archived_at IS NULL AND %[2]s)
	 AND (SELECT count(*) FROM deal_suggestion_evidence e WHERE e.suggestion_id = %[1]s.id) = %[1]s.evidence_count
	 AND NOT EXISTS (SELECT 1 FROM deal_suggestion_evidence e
	   WHERE e.suggestion_id = %[1]s.id AND NOT (
	     (e.kind = 'meeting' AND %[7]s AND EXISTS (SELECT 1 FROM activity ea
	        WHERE ea.id = e.activity_id AND ea.archived_at IS NULL AND %[3]s))
	  OR (e.kind = 'signal' AND %[8]s AND %[7]s AND EXISTS (SELECT 1 FROM signal es
	        WHERE es.id = e.signal_id AND es.archived_at IS NULL AND %[4]s
	          AND NOT EXISTS (SELECT 1 FROM jsonb_array_elements(es.evidence) cite
	            WHERE cite->>'source_type' = 'activity' AND NOT EXISTS (SELECT 1 FROM activity ca
	              WHERE ca.id::text = cite->>'source_id' AND ca.archived_at IS NULL AND %[5]s))))
	  OR (e.kind = 'attachment' AND %[7]s AND EXISTS (SELECT 1 FROM attachment eat
	        JOIN activity aa ON aa.id = eat.activity_id
	        WHERE eat.id = e.attachment_id AND eat.archived_at IS NULL
	          AND aa.archived_at IS NULL AND %[6]s)))))`,
		"s", orTrue(company), meeting, orTrue(signal), cited, carrier, activityGranted, signalGranted), nil
}

// grantedSQL is the reader's object grant to read one kind, as a literal the
// clause can AND in.
func grantedSQL(ctx context.Context, object string) string {
	if auth.Allows(ctx, object, principal.ActionRead) {
		return predicateAlways
	}
	return "false"
}

// orTrue reads an empty clause — the unbounded answer — as a predicate.
func orTrue(clause string) string {
	if clause == "" {
		return predicateAlways
	}
	return clause
}

// requireSuggestionRead is the object gate every suggestion read owes: a
// suggestion proposes a deal about a company, so reading one needs both reads.
func requireSuggestionRead(ctx context.Context) error {
	if err := auth.Require(ctx, "deal", principal.ActionRead); err != nil {
		return err
	}
	return auth.Require(ctx, "company", principal.ActionRead)
}

// Suggestion is one open suggestion as a reader sees it.
type Suggestion struct {
	ID          ids.UUID
	Kind        string
	State       string
	CompanyID   ids.UUID
	CompanyName string
	PipelineID  ids.UUID
	StageID     ids.UUID
	NameHint    string
	AmountMinor *int64
	Currency    *string
	CloseDate   *time.Time
	Confidence  float64
	CreatedAt   time.Time
	Evidence    []SuggestionEvidenceItem
}

// SuggestionEvidenceItem is one cited item with the words a reader recognises
// it by: a meeting's subject, a signal's summary, a document's file name.
type SuggestionEvidenceItem struct {
	SuggestionEvidence
	Title string
	// carrier is the message a document came on; nil for the other kinds.
	carrier *ids.UUID
}

// SuggestionQuery filters one page of open suggestions.
type SuggestionQuery struct {
	CompanyID  *ids.UUID
	PipelineID *ids.UUID
	StageID    *ids.UUID
	Cursor     string
	Limit      int
}

const (
	suggestionDefaultLimit = 50
	suggestionMaxLimit     = 200
)

// suggestionCursor is the list's keyset: newest first, id as the tiebreak.
type suggestionCursor struct {
	CreatedAt time.Time `json:"c"`
	ID        ids.UUID  `json:"id"`
}

const suggestionColumns = `s.id, s.kind, s.state, s.company_id, c.display_name, s.pipeline_id,
	       s.proposed_stage_id, s.name_hint, s.proposed_amount_minor, s.currency,
	       s.proposed_close_date, s.confidence::float8, s.created_at`

func scanSuggestion(row pgx.Row) (Suggestion, error) {
	var s Suggestion
	err := row.Scan(&s.ID, &s.Kind, &s.State, &s.CompanyID, &s.CompanyName, &s.PipelineID,
		&s.StageID, &s.NameHint, &s.AmountMinor, &s.Currency, &s.CloseDate, &s.Confidence, &s.CreatedAt)
	return s, err
}

// ListSuggestions pages the open suggestions the caller may see.
func (s *Store) ListSuggestions(ctx context.Context, in SuggestionQuery) ([]Suggestion, string, error) {
	if err := requireSuggestionRead(ctx); err != nil {
		return nil, "", err
	}
	switch {
	case in.Limit <= 0:
		in.Limit = suggestionDefaultLimit
	case in.Limit > suggestionMaxLimit:
		in.Limit = suggestionMaxLimit
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	visible, err := suggestionVisibleClause(ctx, arg)
	if err != nil {
		return nil, "", err
	}
	query := `SELECT ` + suggestionColumns + ` FROM deal_suggestion s JOIN company c ON c.id = s.company_id
	 WHERE s.state = 'open' AND ` + visible
	for _, filter := range []struct {
		column string
		value  *ids.UUID
	}{{"s.company_id", in.CompanyID}, {"s.pipeline_id", in.PipelineID}, {"s.proposed_stage_id", in.StageID}} {
		if filter.value != nil {
			query += fmt.Sprintf(" AND %s = $%d", filter.column, arg(*filter.value))
		}
	}
	if in.Cursor != "" {
		cur, err := storekit.DecodeOpaque[suggestionCursor](in.Cursor)
		if err != nil || cur.ID.IsZero() {
			return nil, "", &storekit.MalformedCursorError{}
		}
		query += fmt.Sprintf(" AND (s.created_at, s.id) < ($%d, $%d)", arg(cur.CreatedAt), arg(cur.ID))
	}
	query += fmt.Sprintf(" ORDER BY s.created_at DESC, s.id DESC LIMIT $%d", arg(in.Limit+1))

	var out []Suggestion
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, query, args...)
		if err != nil {
			return err
		}
		out, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (Suggestion, error) { return scanSuggestion(row) })
		if err != nil {
			return err
		}
		return attachEvidence(ctx, tx, out)
	})
	if err != nil {
		return nil, "", fmt.Errorf("deals: listing suggestions: %w", err)
	}
	next := ""
	if len(out) > in.Limit {
		out = out[:in.Limit]
		last := out[len(out)-1]
		if next, err = storekit.EncodeOpaque(suggestionCursor{CreatedAt: last.CreatedAt, ID: last.ID}); err != nil {
			return nil, "", fmt.Errorf("deals: encoding the suggestion cursor: %w", err)
		}
	}
	return out, next, nil
}

// CountOpenSuggestions is how many open suggestions THIS caller can see. A
// count is a read, so it carries the list's own visibility clause.
func (s *Store) CountOpenSuggestions(ctx context.Context) (int, error) {
	if err := requireSuggestionRead(ctx); err != nil {
		return 0, err
	}
	var args []any
	visible, err := suggestionVisibleClause(ctx,
		func(v any) int { args = append(args, v); return len(args) })
	if err != nil {
		return 0, err
	}
	var open int
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`SELECT count(*) FROM deal_suggestion s WHERE s.state = 'open' AND `+visible, args...).Scan(&open)
	})
	if err != nil {
		return 0, fmt.Errorf("deals: counting suggestions: %w", err)
	}
	return open, nil
}

// GetSuggestion reads one suggestion in any state, when the caller may see it.
func (s *Store) GetSuggestion(ctx context.Context, id ids.UUID) (Suggestion, error) {
	if err := requireSuggestionRead(ctx); err != nil {
		return Suggestion{}, err
	}
	var out Suggestion
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		var err error
		out, err = readVisibleSuggestionTx(ctx, tx, id, false)
		return err
	})
	return out, err
}

// readVisibleSuggestionTx reads one suggestion, in any state, when the caller
// may see it; otherwise not-found, so existence stays hidden. lock takes the
// row for the rest of the transaction.
func readVisibleSuggestionTx(ctx context.Context, tx pgx.Tx, id ids.UUID, lock bool) (Suggestion, error) {
	args := []any{id}
	visible, err := suggestionVisibleClause(ctx,
		func(v any) int { args = append(args, v); return len(args) })
	if err != nil {
		return Suggestion{}, err
	}
	query := `SELECT ` + suggestionColumns + ` FROM deal_suggestion s JOIN company c ON c.id = s.company_id
	 WHERE s.id = $1 AND ` + visible
	if lock {
		query += ` FOR UPDATE OF s`
	}
	out, err := scanSuggestion(tx.QueryRow(ctx, query, args...))
	if errors.Is(err, pgx.ErrNoRows) {
		return Suggestion{}, apperrors.ErrNotFound
	}
	if err != nil {
		return Suggestion{}, fmt.Errorf("deals: reading a suggestion: %w", err)
	}
	one := []Suggestion{out}
	if err := attachEvidence(ctx, tx, one); err != nil {
		return Suggestion{}, err
	}
	return one[0], nil
}

// attachEvidence fills each suggestion's evidence in one statement, with the
// message a document came on. It asks the visibility clause again rather than
// trusting its caller's list: the titles are content, and only a reader who
// may see every piece of a suggestion's evidence is shown any of it.
func attachEvidence(ctx context.Context, tx pgx.Tx, list []Suggestion) error {
	if len(list) == 0 {
		return nil
	}
	index := make(map[ids.UUID]int, len(list))
	suggestionIDs := make([]ids.UUID, 0, len(list))
	for i, s := range list {
		index[s.ID] = i
		suggestionIDs = append(suggestionIDs, s.ID)
	}
	args := []any{suggestionIDs}
	visible, err := suggestionVisibleClause(ctx,
		func(v any) int { args = append(args, v); return len(args) })
	if err != nil {
		return err
	}
	rows, err := tx.Query(ctx, `
		SELECT e.suggestion_id, e.kind, e.activity_id, e.signal_id, e.attachment_id, e.occurred_at,
		       coalesce(a.subject, sg.summary, at.filename, ''), at.activity_id
		  FROM deal_suggestion_evidence e
		  JOIN deal_suggestion s ON s.id = e.suggestion_id
		  LEFT JOIN activity a ON a.id = e.activity_id
		  LEFT JOIN signal sg ON sg.id = e.signal_id
		  LEFT JOIN attachment at ON at.id = e.attachment_id
		 WHERE e.suggestion_id = ANY($1) AND `+visible+`
		 ORDER BY e.occurred_at DESC, e.id`, args...)
	if err != nil {
		return fmt.Errorf("deals: reading suggestion evidence: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var owner ids.UUID
		var item SuggestionEvidenceItem
		if err := rows.Scan(&owner, &item.Kind, &item.ActivityID, &item.SignalID, &item.AttachmentID,
			&item.OccurredAt, &item.Title, &item.carrier); err != nil {
			return fmt.Errorf("deals: scanning suggestion evidence: %w", err)
		}
		list[index[owner]].Evidence = append(list[index[owner]].Evidence, item)
	}
	return rows.Err()
}
