// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package search

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
)

// activitySnippetRunes is the length the activity branch cuts its excerpt to,
// and the length a rendered excerpt is cut to, so the two compare.
const activitySnippetRunes = 200

// renderSystemText names an activity hit Margince wrote in the reader's
// language, the way its History row names it. The branch SQL titles a hit with
// the stored subject, which is English for every task Margince created.
//
// Only a hit still showing the stored words is changed. A title that fell
// through to the channel or the kind stays as ranked. So does a snippet whose
// body the descriptor no longer matches.
func renderSystemText(ctx context.Context, tx pgx.Tx, hits []Hit) error {
	var activityIDs []ids.UUID
	for i := range hits {
		if hits[i].Type == hitTypeActivity {
			activityIDs = append(activityIDs, hits[i].ID)
		}
	}
	if len(activityIDs) == 0 {
		return nil
	}
	args := []any{activityIDs}
	rows, err := tx.Query(ctx, fmt.Sprintf(`SELECT id, coalesce(subject, ''), coalesce(body, ''), system_text
		FROM activity WHERE id = ANY($%d) AND system_text IS NOT NULL`, len(args)), args...)
	if err != nil {
		return fmt.Errorf("search: reading the system text of this page's activities: %w", err)
	}
	defer rows.Close()
	type written struct {
		subject, body string
		message       *systemtext.Message
	}
	byID := map[ids.UUID]written{}
	for rows.Next() {
		var id ids.UUID
		var row written
		if err := rows.Scan(&id, &row.subject, &row.body, &row.message); err != nil {
			return fmt.Errorf("search: reading the system text of this page's activities: %w", err)
		}
		byID[id] = row
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("search: reading the system text of this page's activities: %w", err)
	}
	reader := systemtext.Locale(ctx)
	for i := range hits {
		row, ok := byID[hits[i].ID]
		if hits[i].Type != hitTypeActivity || !ok {
			continue
		}
		if hits[i].Title == row.subject {
			hits[i].Title = row.message.SubjectIn(row.subject, reader)
		}
		if hits[i].Snippet != "" && hits[i].Snippet == firstRunes(row.body, activitySnippetRunes) {
			hits[i].Snippet = firstRunes(row.message.BodyIn(row.body, reader), activitySnippetRunes)
		}
	}
	return nil
}

func firstRunes(text string, limit int) string {
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	return string(runes[:limit])
}
