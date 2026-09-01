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

// NewNotice is one notice to record. It is a struct rather than a parameter
// list because the dedupe key is the fifth thing a caller might say about a
// notice, and five positional strings is where a caller starts passing the
// subject as the body.
type NewNotice struct {
	Recipient ids.UserID
	Kind      string
	Subject   string
	Body      string
	// DedupeKey is the caller's natural key for the EVENT this notice is
	// about. A second Create carrying the same key for the same recipient
	// answers the first notice's id and writes nothing.
	//
	// Empty where the caller has no such key, and most do not: a notice
	// addressed by a human act has no event to key on. Empty means "not
	// claiming one", and those rows dedupe against nothing — which is the only
	// honest answer, since an invented key would silently collapse two real
	// notices into one.
	DedupeKey string
}

// Create records one notice for recipient — the whole of delivery on this
// transport. captured_by comes from the authenticated principal (the engine
// writes as the system), never from a caller-supplied field, and the write
// lands with its audit row and event in one transaction like every other
// mutation. The recipient's existence is held by the row's own FK: a notice
// to nobody fails loudly rather than landing unread forever.
//
// AT-LEAST-ONCE IS THE CALLER'S PROBLEM AND THIS IS WHERE IT IS SOLVED.
// workflow.Handler.Apply is documented as idempotent on IdempotencyKey(ev),
// and a handler that is only idempotent while the QUEUE is has its correctness
// somewhere the reader cannot see. A notice carrying a dedupe key lands as ONE
// ROW PER RECIPIENT however many times its event is delivered; the second call
// answers the first id and emits nothing, because a second event about a
// notice that already exists would put the same line on the same Worklist
// twice by another route.
func (s *Store) Create(ctx context.Context, in NewNotice) (ids.UUID, error) {
	// No evidence: a system flow's authority is the engine's own, which the
	// audit row's actor already states.
	row, err := s.insertNotice(ctx, in, nil)
	return row.ID, err
}

// insertNotice is the write itself, returning the whole row.
//
// Create keeps its id-only signature because every system flow that calls it
// wants nothing else, and widening it would make four call sites carry a value
// none of them reads. The coach path does read it: it answers 201 with the
// notice, and the created_at on that answer has to be the row's own.
func (s *Store) insertNotice(ctx context.Context, in NewNotice, evidence map[string]any) (Notice, error) {
	if in.Kind == "" || in.Subject == "" {
		return Notice{}, errors.New("notices: a notice names its kind and its subject")
	}
	capturedBy, err := storekit.CapturedBy(ctx)
	if err != nil {
		return Notice{}, err
	}
	// Locals rather than in.* from here down: on the duplicate path below these
	// are replaced by what the stored notice actually says, and a field read
	// straight off the argument would quietly disagree with the row.
	kind := in.Kind
	subject := truncate(in.Subject, subjectBound)
	body := truncate(in.Body, bodyBound)
	id := ids.NewV7()
	var createdAt time.Time
	var dedupe *string
	if in.DedupeKey != "" {
		dedupe = &in.DedupeKey
	}
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		// The INDEX is the guard, not a read-then-write check: two deliveries
		// racing both pass a check and only one may pass the index.
		//
		// RETURNING tells the two cases apart. No row back means the key was
		// already there, and everything below — the audit row, the event —
		// belongs to a notice that was already recorded once.
		//
		// created_at is the column's own default, so the insert returns it
		// rather than the caller stamping a second clock beside it.
		row := tx.QueryRow(ctx, `
			INSERT INTO notice (id, recipient_user_id, kind, subject, body, captured_by, dedupe_key)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
			ON CONFLICT (recipient_user_id, dedupe_key) WHERE dedupe_key IS NOT NULL
			DO NOTHING
			RETURNING created_at`,
			id, in.Recipient, in.Kind, subject, body, capturedBy, dedupe)
		switch err := row.Scan(&createdAt); {
		case errors.Is(err, pgx.ErrNoRows):
			// Already recorded. Answer the notice that STANDS — all of it, not
			// its id with this call's text pinned beside it.
			//
			// The two need not agree. A dedupe key names the EVENT, and a
			// second delivery can carry a subject reworded since, or a kind a
			// later version spells differently. Returning the stored id with
			// the replay's words would put a notice on screen that the reader
			// cannot find anywhere, and that nothing in the database says.
			return tx.QueryRow(ctx, `
				SELECT id, kind, subject, body, created_at FROM notice
				 WHERE recipient_user_id = $1 AND dedupe_key = $2`,
				in.Recipient, in.DedupeKey).Scan(&id, &kind, &subject, &body, &createdAt)
		case err != nil:
			return fmt.Errorf("notices: recording the notice: %w", err)
		}
		auditID, err := storekit.AuditEventWithEvidence(ctx, tx, "create", "notice", id,
			map[string]any{"kind": in.Kind, "recipient_user_id": in.Recipient.String()}, evidence)
		if err != nil {
			return err
		}
		recipientID := in.Recipient.UUID
		return storekit.EmitEvent(ctx, tx, auditID, recipientID, crmcontracts.PublicEventNoticeCreated{
			NoticeId:        openapi_types.UUID(id),
			RecipientUserId: openapi_types.UUID(recipientID),
			Kind:            in.Kind,
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
