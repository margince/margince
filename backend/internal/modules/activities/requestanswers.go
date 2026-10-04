// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// OffThreadAnswer is one answer of ours to a request that sits outside its
// thread, in the fields the settlement model reads.
type OffThreadAnswer struct {
	ID ids.UUID
	// Kind is email for our same-subject mail, or call or meeting.
	Kind    string
	Subject string
	Body    string
	At      time.Time
}

// OffThreadAnswersTx reads the answers to a request that sit outside its
// thread, oldest first, as of asOf, with each body cut to bodyLimit runes.
// They are the answers answered.go recognises off the thread, so a request the
// waiting lane counts as answered is judged on the same evidence.
//
// System-only: the bodies go to the settlement model.
func OffThreadAnswersTx(ctx context.Context, tx pgx.Tx, request ids.UUID, asOf time.Time,
	bodyLimit, limit int,
) ([]OffThreadAnswer, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `
		SELECT answer.id, answer.kind, coalesce(answer.subject, ''),
		       coalesce(left(answer.body, $3), ''), answer.occurred_at
		  FROM activity a
		  CROSS JOIN LATERAL (`+offThreadAnswersSQL("a", "$2")+`) found
		  JOIN activity answer ON answer.id = found.id
		 WHERE a.id = $1
		   AND answer.archived_at IS NULL AND answer.restricted_at IS NULL
		   AND answer.audience = 'workspace'
		 ORDER BY answer.occurred_at, answer.id
		 LIMIT $4`, request, asOf, bodyLimit, limit)
	if err != nil {
		return nil, fmt.Errorf("activities: reading the answers off the request's thread: %w", err)
	}
	defer rows.Close()
	var out []OffThreadAnswer
	for rows.Next() {
		var answer OffThreadAnswer
		if err := rows.Scan(&answer.ID, &answer.Kind, &answer.Subject, &answer.Body, &answer.At); err != nil {
			return nil, err
		}
		out = append(out, answer)
	}
	return out, rows.Err()
}
