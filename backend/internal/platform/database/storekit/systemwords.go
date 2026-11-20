// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
)

// SystemWords is an activity's stored subject and body with the descriptor
// Margince wrote them with.
type SystemWords struct {
	Subject, Body string
	Message       *systemtext.Message
}

// SystemWrittenActivities reads the words of those activities that carry a
// template descriptor. A read that admitted them can then name them in the
// reader's language. The caller's statement chose the ids; this read asks
// the content gate again, so it returns nothing the caller may not read. A
// prose body, such as a sent mail, is not loaded.
func SystemWrittenActivities(ctx context.Context, tx pgx.Tx, activityIDs []ids.UUID) (map[ids.UUID]SystemWords, error) {
	out := map[ids.UUID]SystemWords{}
	if len(activityIDs) == 0 {
		return out, nil
	}
	var args []any
	arg := func(value any) int { args = append(args, value); return len(args) }
	idsAt, template := arg(activityIDs), arg(string(systemtext.Template))
	readable, err := auth.ActivityContentClause(ctx, "a", arg)
	if err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, fmt.Sprintf(`SELECT a.id, coalesce(a.subject, ''),
		CASE WHEN a.system_text->'body'->>'origin' = $%[2]d THEN coalesce(a.body, '') ELSE '' END, a.system_text
		FROM activity a WHERE a.id = ANY($%[1]d) AND %[3]s
		  AND (a.system_text->'subject'->>'origin' = $%[2]d OR a.system_text->'body'->>'origin' = $%[2]d)`, idsAt, template, readable), args...)
	if err != nil {
		return nil, fmt.Errorf("read the system text of these activities: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var id ids.UUID
		var words SystemWords
		if err := rows.Scan(&id, &words.Subject, &words.Body, &words.Message); err != nil {
			return nil, fmt.Errorf("read the system text of these activities: %w", err)
		}
		out[id] = words
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read the system text of these activities: %w", err)
	}
	return out, nil
}
