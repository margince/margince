// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package notices

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// subjectBound and bodyBound cap what a producer can put in front of a
// reader: a notice is a line and a paragraph, not a document, and the lane
// that carries it ships bounded strings to every open tab.
const (
	subjectBound = 200
	bodyBound    = 2000
)

// Store owns the notice table.
type Store struct {
	db *database.DB
}

// NewStore binds the store to db.
func NewStore(db *database.DB) *Store {
	return &Store{db: db}
}

// Notice is one durable line addressed to one person.
type Notice struct {
	ID        ids.UUID
	Kind      string
	Subject   string
	Body      string
	CreatedAt time.Time
}

// Create records one notice for recipient — the whole of delivery on this
// transport. captured_by comes from the authenticated principal (the engine
// writes as the system), never from a caller-supplied field, and the write
// lands with its audit row and event in one transaction like every other
// mutation. The recipient's existence is held by the row's own FK: a notice
// to nobody fails loudly rather than landing unread forever.
func (s *Store) Create(ctx context.Context, recipient ids.UserID, kind, subject, body string) (ids.UUID, error) {
	row, err := s.create(ctx, recipient, kind, subject, body)
	return row.ID, err
}

// create is the write itself, returning the whole row.
//
// Create keeps its id-only signature because every system flow that calls it
// wants nothing else, and widening it would make four call sites carry a value
// none of them reads. The coach path does read it: it answers 201 with the
// notice, and the created_at on that answer has to be the row's own.
func (s *Store) create(ctx context.Context, recipient ids.UserID, kind, subject, body string) (Notice, error) {
	if kind == "" || subject == "" {
		return Notice{}, errors.New("notices: a notice names its kind and its subject")
	}
	capturedBy, err := storekit.CapturedBy(ctx)
	if err != nil {
		return Notice{}, err
	}
	var createdAt time.Time
	subject = truncate(subject, subjectBound)
	body = truncate(body, bodyBound)
	id := ids.NewV7()
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		// created_at is the column's own default, so the insert returns it
		// rather than the caller stamping a second clock beside it.
		if err := tx.QueryRow(ctx, `
			INSERT INTO notice (id, recipient_user_id, kind, subject, body, captured_by)
			VALUES ($1, $2, $3, $4, $5, $6)
			RETURNING created_at`,
			id, recipient, kind, subject, body, capturedBy).Scan(&createdAt); err != nil {
			return fmt.Errorf("notices: recording the notice: %w", err)
		}
		auditID, err := storekit.AuditEvent(ctx, tx, "create", "notice", id,
			map[string]any{"kind": kind, "recipient_user_id": recipient.String()})
		if err != nil {
			return err
		}
		recipientID := recipient.UUID
		return storekit.EmitEvent(ctx, tx, auditID, recipientID, crmcontracts.PublicEventNoticeCreated{
			NoticeId:        openapi_types.UUID(id),
			RecipientUserId: openapi_types.UUID(recipientID),
			Kind:            kind,
		})
	})
	if err != nil {
		return Notice{}, err
	}
	return Notice{ID: id, Kind: kind, Subject: subject, Body: body, CreatedAt: createdAt}, nil
}

// UnreadFor answers the CALLING person's own unread notices, newest first,
// bounded. The person comes from the bound principal and is not a parameter
// — another person's notices cannot be expressed — and a caller with no
// person behind it is refused with the permission sentinel, which the
// attention feed renders as a withheld lane.
func (s *Store) UnreadFor(ctx context.Context, limit int) ([]Notice, error) {
	actor, ok := principal.Actor(ctx)
	if !ok || actor.Type != principal.PrincipalHuman || actor.UserID.IsZero() {
		// The PERSON, not merely a user id: an agent or system principal
		// can carry a human's id, and reading — like settling — a notice
		// is that human's own act.
		return nil, fmt.Errorf("notices: reading your notices needs an authenticated person: %w", apperrors.ErrPermissionDenied)
	}
	var unread []Notice
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		rows, txErr := tx.Query(ctx, `
			SELECT id, kind, subject, body, created_at
			  FROM notice
			 WHERE recipient_user_id = $1 AND read_at IS NULL
			 ORDER BY created_at DESC, id DESC
			 LIMIT $2`, actor.UserID, limit)
		if txErr != nil {
			return txErr
		}
		defer rows.Close()
		unread = []Notice{}
		for rows.Next() {
			var n Notice
			if scanErr := rows.Scan(&n.ID, &n.Kind, &n.Subject, &n.Body, &n.CreatedAt); scanErr != nil {
				return scanErr
			}
			unread = append(unread, n)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("notices: listing unread notices: %w", err)
	}
	return unread, nil
}

// MarkRead settles one notice for its own recipient. Scoped by recipient in
// the statement — another person's notice reads as absent (404), so its
// existence stays hidden — and idempotent: marking a read notice again is a
// no-op success, because the reader's goal state already holds. The
// read-state flip is a mutation and carries the write shape.
func (s *Store) MarkRead(ctx context.Context, id ids.UUID) error {
	actor, ok := principal.Actor(ctx)
	if !ok || actor.Type != principal.PrincipalHuman || actor.UserID.IsZero() {
		// The same person-not-id rule UnreadFor states: acknowledgment is
		// the recipient's own act, never a principal acting as them.
		return fmt.Errorf("notices: marking a notice read needs an authenticated person: %w", apperrors.ErrPermissionDenied)
	}
	return s.db.Tx(ctx, func(tx pgx.Tx) error {
		// The write claims the unread row alone, so two concurrent settles
		// race for one row lock and exactly one announces; the loser's
		// existence probe below turns "no unread row" into either the
		// idempotent success or the stranger's 404.
		tag, err := tx.Exec(ctx, `
			UPDATE notice SET read_at = now()
			 WHERE id = $1 AND recipient_user_id = $2 AND read_at IS NULL`,
			id, actor.UserID)
		if err != nil {
			return fmt.Errorf("notices: marking the notice read: %w", err)
		}
		if tag.RowsAffected() == 0 {
			// An unclaimed row means either the stranger's absent notice or
			// a settle that already happened — and a row that exists here
			// necessarily carries read_at, so its existence alone decides.
			var one int
			if err := tx.QueryRow(ctx,
				`SELECT 1 FROM notice WHERE id = $1 AND recipient_user_id = $2`,
				id, actor.UserID).Scan(&one); err != nil {
				if errors.Is(err, pgx.ErrNoRows) {
					return apperrors.ErrNotFound
				}
				return fmt.Errorf("notices: probing the notice: %w", err)
			}
			// The reader's goal state already holds: a replayed mark writes
			// no second audit row and announces nothing.
			return nil
		}
		auditID, err := storekit.AuditEvent(ctx, tx, "update", "notice", id,
			map[string]any{"read": true})
		if err != nil {
			return err
		}
		// The event's subject is the RECIPIENT, like the created event's:
		// the self-only delivery rule compares the subscription owner to the
		// entity, and the notice's lifecycle is that one person's to hear.
		return storekit.EmitEvent(ctx, tx, auditID, actor.UserID, crmcontracts.PublicEventNoticeRead{
			NoticeId: openapi_types.UUID(id),
		})
	})
}

// truncate bounds s to at most n runes.
func truncate(s string, n int) string {
	if runes := []rune(s); len(runes) > n {
		return string(runes[:n])
	}
	return s
}
