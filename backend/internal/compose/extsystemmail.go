// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The system read of the emails linked to one deal. A job tick or a bus
// delivery has no caller, so no grant narrows it. What a user limited to
// themselves may not see is left out here instead. The read answers only mail
// the whole workspace may see. It gives only the facts a unit needs to tell
// whether a mail went out and whether anyone answered. No subject, body, address or
// attachment leaves the core.

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

//nolint:ireturn // the published port, as Deals.
func (r *callRuntime) Activities() extension.Activities { return systemActivities{rt: r} }

type systemActivities struct{ rt *callRuntime }

func (a systemActivities) Mail(ctx context.Context, dealID string, q extension.MailQuery) (extension.Page[extension.MailSummary], error) {
	ctx, err := a.rt.systemRead(ctx, extension.SystemReadActivities)
	if err != nil {
		return extension.Page[extension.MailSummary]{}, err
	}
	deal, err := unitID(dealID, "deal")
	if err != nil {
		return extension.Page[extension.MailSummary]{}, err
	}
	limit, err := mailPageLimit(q.Limit)
	if err != nil {
		return extension.Page[extension.MailSummary]{}, err
	}
	after, err := parseMailCursor(q.Cursor)
	if err != nil {
		return extension.Page[extension.MailSummary]{}, err
	}
	page, err := readDealMail(ctx, a.rt.deps.pool, deal, after, limit, q.IncludeArchived)
	if err != nil && !errors.Is(err, extension.ErrNotFound) {
		return extension.Page[extension.MailSummary]{}, a.rt.systemReadFault(ctx, err)
	}
	return page, err
}

// defaultMailPage is how many emails a page holds when the call names no limit.
const defaultMailPage = 50

func mailPageLimit(limit int) (int, error) {
	switch {
	case limit < 0:
		return 0, fmt.Errorf("%w: limit cannot be negative", extension.ErrInvalid)
	case limit == 0:
		return defaultMailPage, nil
	}
	return min(limit, extension.MaxReadPage), nil
}

// mailPosition is where the page before ended. Mail runs newest first, and the
// id breaks a tie between two sent in the same instant.
type mailPosition struct {
	at time.Time
	id ids.UUID
}

func (p mailPosition) cursor() string {
	return base64.RawURLEncoding.EncodeToString([]byte(p.at.UTC().Format(time.RFC3339Nano) + "|" + p.id.String()))
}

func parseMailCursor(cursor string) (*mailPosition, error) {
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
	return &mailPosition{at: when, id: parsed}, nil
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

func readDealMail(ctx context.Context, pool *pgxpool.Pool, deal ids.UUID, after *mailPosition, limit int, archived bool) (extension.Page[extension.MailSummary], error) {
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
			page.NextCursor = mailPosition{at: last.OccurredAt, id: id}.cursor()
		}
		page.Items = mail
		return nil
	})
	if err != nil && !errors.Is(err, extension.ErrNotFound) {
		return extension.Page[extension.MailSummary]{}, fmt.Errorf("compose: reading the mail of deal %s for an extension: %w", deal, err)
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
