// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package systemmail

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/relstrength"
	"github.com/margince/margince/backend/pkg/extension"
)

// defaultPage is how many emails a page holds when the call names no limit.
const defaultPage = 50

// Query is a MailQuery whose limit and cursor were read.
type Query struct {
	limit    int
	after    *position
	archived bool
}

// ParseQuery refuses a negative limit and a cursor this read did not give.
func ParseQuery(q extension.MailQuery) (Query, error) {
	limit, err := PageLimit(q.Limit)
	if err != nil {
		return Query{}, err
	}
	after, err := parseCursor(q.Cursor)
	if err != nil {
		return Query{}, err
	}
	return Query{limit: limit, after: after, archived: q.IncludeArchived}, nil
}

// PageLimit reads a page size a unit asked for: none is the default, and more
// than extension.MaxReadPage is cut to it.
func PageLimit(limit int) (int, error) {
	switch {
	case limit < 0:
		return 0, fmt.Errorf("%w: limit cannot be negative", extension.ErrInvalid)
	case limit == 0:
		return defaultPage, nil
	}
	return min(limit, extension.MaxReadPage), nil
}

// position is where the page before ended. Mail runs newest first, and the
// id breaks a tie between two sent in the same instant.
type position struct {
	at time.Time
	id ids.UUID
}

func (p position) cursor() string {
	return base64.RawURLEncoding.EncodeToString([]byte(p.at.UTC().Format(time.RFC3339Nano) + "|" + p.id.String()))
}

func parseCursor(cursor string) (*position, error) {
	if cursor == "" {
		return nil, nil //nolint:nilnil // no cursor is the first page: a nil position IS that answer
	}
	bad := fmt.Errorf("%w: the cursor is not one this read gave", extension.ErrInvalid)
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return nil, bad
	}
	at, id, ok := strings.Cut(string(raw), "|")
	if !ok {
		return nil, bad
	}
	when, err := time.Parse(time.RFC3339Nano, at)
	if err != nil {
		return nil, bad
	}
	parsed, err := ids.Parse(id)
	if err != nil {
		return nil, bad
	}
	return &position{at: when, id: parsed}, nil
}

// dealMailQuery reads one page of a deal's emails. Shared means the audience
// every colleague may see, and a row held under a retention obligation is not
// readable by anyone. A mail excluded from capture is never stored, so it is
// absent without a clause of its own.
const dealMailQuery = `
	SELECT a.id, a.direction, a.occurred_at, a.kind, a.origin, a.calendar_method, a.sender_is_machine
	  FROM activity a
	 WHERE a.kind = 'email'
	   AND a.audience = 'workspace'
	   AND %s
	   AND EXISTS (SELECT 1 FROM activity_link l WHERE l.activity_id = a.id AND l.deal_id = $1)
	   %s
	 ORDER BY a.occurred_at DESC, a.id DESC
	 LIMIT $2`

// ReadDeal reads one page of a deal's emails as the system, the whole
// workspace's mail only. A deal no row holds answers extension.ErrNotFound.
func ReadDeal(ctx context.Context, pool *pgxpool.Pool, deal ids.UUID, q Query) (extension.Page[extension.MailSummary], error) {
	after, limit, archived := q.after, q.limit, q.archived
	var page extension.Page[extension.MailSummary]
	err := database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error {
		var exists bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM deal WHERE id = $1)`, deal).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return extension.ErrNotFound
		}
		args := []any{deal, limit + 1}
		extra := ""
		if !archived {
			extra += " AND a.archived_at IS NULL"
		}
		if after != nil {
			args = append(args, after.at, after.id)
			extra += " AND (a.occurred_at, a.id) < ($3, $4)"
		}
		rows, err := tx.Query(ctx, fmt.Sprintf(dealMailQuery, auth.ActivityAvailableClause("a"), extra), args...)
		if err != nil {
			return err
		}
		mail, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (extension.MailSummary, error) {
			return scanMailSummary(row, deal)
		})
		if err != nil {
			return err
		}
		if len(mail) > limit {
			mail = mail[:limit]
			last := mail[limit-1]
			id, _ := ids.Parse(last.ID)
			page.NextCursor = position{at: last.OccurredAt, id: id}.cursor()
		}
		page.Items = mail
		return nil
	})
	if err != nil && !errors.Is(err, extension.ErrNotFound) {
		return extension.Page[extension.MailSummary]{}, fmt.Errorf("systemmail: reading the mail of deal %s for an extension: %w", deal, err)
	}
	return page, err
}

func scanMailSummary(row pgx.CollectableRow, deal ids.UUID) (extension.MailSummary, error) {
	var (
		id        ids.UUID
		direction *string
		at        time.Time
		evidence  relstrength.Evidence
		method    *string
	)
	if err := row.Scan(&id, &direction, &at, &evidence.Kind, &evidence.Origin, &method, &evidence.SenderIsMachine); err != nil {
		return extension.MailSummary{}, err
	}
	if method != nil {
		evidence.CalendarMethod = *method
	}
	out := extension.MailSummary{
		ID: id.String(), OccurredAt: at, DealID: deal.String(), AutomaticKind: string(evidence.Automatic()),
	}
	if direction != nil {
		out.Direction = *direction
	}
	return out, nil
}
