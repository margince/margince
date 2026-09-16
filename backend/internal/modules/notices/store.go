// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package notices

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
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

// Notice is one durable line addressed to one contact.
type Notice struct {
	Origin    *crmcontracts.NoticeOrigin
	ID        ids.UUID
	Kind      string
	Subject   string
	Body      string
	Target    Target
	CreatedAt time.Time
}

// Target is the record a notice is about, when it is about one.
//
// Both halves or neither, which the table's own constraint holds: a type with
// no id names a KIND of thing and cannot be opened, and an id with no type
// cannot be routed to a screen. The zero value is a notice about no record —
// a capture backlog, a coach's word — and those are the majority.
type Target struct {
	Type string
	ID   ids.UUID
}

// Named reports whether this notice points at a record.
func (t Target) Named() bool { return t.Type != "" && !t.ID.IsZero() }

// NewNotice is one notice to record. It is a struct rather than a parameter
// list because the dedupe key is the fifth thing a caller might say about a
// notice, and five positional strings is where a caller starts passing the
// subject as the body.
type NewNotice struct {
	Origin    *crmcontracts.NoticeOrigin
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
	// Target is the record this notice is about, when it is about one. "A deal
	// you own changed stage" is true of every deal a rep owns, so the sentence
	// alone left a reader knowing something moved and having to find it.
	Target Target
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
//
// It is CreateTx in a transaction of its own, so the recipient's preference
// decides here too: a class this seat switched off writes nothing and answers
// the zero id, and CreateTx states that contract in full.
func (s *Store) Create(ctx context.Context, in NewNotice) (ids.UUID, error) {
	var id ids.UUID
	if err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		var txErr error
		id, txErr = s.CreateTx(ctx, tx, in)
		return txErr
	}); err != nil {
		return ids.Nil, err
	}
	return id, nil
}

// CreateTx is Create inside a transaction the caller already holds, for the
// producers whose notice has to commit with the change it announces: the
// lead-SLA fold writes the escalation task beside it, and an approval's notify
// leg writes the proposal. A notice landing in its own transaction can commit
// while theirs rolls back, which tells a reader about something that never
// happened.
//
// SUPPRESSION IS HERE AND NOT ON THE READ. A seat who switched a class off has
// decided the product may not raise it, and a row written anyway is still read
// by everything else that reads the table — the digest sweep, an export, a
// support query — so the decision would hold only where somebody remembered it.
// Nothing is written: no row, no ledger entry, no announcement. The answer is
// the zero id, which every caller already ignores.
//
// An unplaced kind is a plain error rather than a delivery under a default,
// because it is a programming defect: a producer spelling a kind ClassFor does
// not place would otherwise arrive under a setting nobody was ever shown.
func (s *Store) CreateTx(ctx context.Context, tx pgx.Tx, in NewNotice) (ids.UUID, error) {
	class, err := ClassFor(in.Kind)
	if err != nil {
		return ids.Nil, err
	}
	delivery, err := DeliveryFor(ctx, tx, in.Recipient, class)
	if err != nil {
		return ids.Nil, err
	}
	if delivery == DeliveryOff {
		// Unreachable for a colleague's coaching, which cannot be switched off
		// — SaveNotificationPreference refuses it — and guarded here anyway,
		// because the thing holding that is a rule in another file.
		return ids.Nil, nil
	}
	// No evidence: a system flow's authority is the engine's own, which the
	// audit row's actor already states.
	row, err := s.insertNoticeTx(ctx, tx, in, nil)
	return row.ID, err
}

// insertNotice is the write in a transaction of its own — the coaching path,
// whose notice is the whole of what its request changes.
//
// It carries no preference check: RaiseCoachNotice is the only caller, and a
// colleague's words are not the product's own housekeeping to suppress.
func (s *Store) insertNotice(ctx context.Context, in NewNotice, evidence map[string]any) (Notice, error) {
	var written Notice
	if err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		var txErr error
		written, txErr = s.insertNoticeTx(ctx, tx, in, evidence)
		return txErr
	}); err != nil {
		return Notice{}, err
	}
	return written, nil
}

// insertNoticeTx is the write itself, returning the whole row.
//
// Create keeps its id-only signature because every system flow that calls it
// wants nothing else, and widening it would make four call sites carry a value
// none of them reads. The coach path does read it: it answers 201 with the
// notice, and the created_at on that answer has to be the row's own.
func (s *Store) insertNoticeTx(ctx context.Context, tx pgx.Tx, in NewNotice, evidence map[string]any) (Notice, error) {
	if in.Kind == "" || in.Subject == "" {
		return Notice{}, errors.New("notices: a notice names its kind and its subject")
	}
	capturedBy, err := storekit.CapturedBy(ctx)
	if err != nil {
		return Notice{}, err
	}
	subject := truncate(in.Subject, subjectBound)
	body := truncate(in.Body, bodyBound)
	id := ids.NewV7()
	var createdAt time.Time
	var dedupe *string
	if in.DedupeKey != "" {
		dedupe = &in.DedupeKey
	}
	// Nil rather than the zero value on both halves, so the table's paired
	// constraint sees a notice about no record as two NULLs.
	var targetType *string
	var targetID *ids.UUID
	if in.Target.Named() {
		targetType, targetID = &in.Target.Type, &in.Target.ID
	}
	// The INDEX is the guard, not a read-then-write check: two deliveries
	// racing both pass a check and only one may pass the index.
	//
	// RETURNING tells the two cases apart. No row back means the key was
	// already there, and everything below — the audit row, the event — belongs
	// to a notice that was already recorded once.
	//
	// created_at is the column's own default, so the insert returns it rather
	// than the caller stamping a second clock beside it.
	args := []any{id, in.Recipient, in.Kind, subject, body, capturedBy, dedupe, targetType, targetID, in.Origin}
	holders := make([]string, len(args))
	for i := range args {
		holders[i] = "$" + strconv.Itoa(i+1)
	}
	placeholders := strings.Join(holders, ", ")
	row := tx.QueryRow(ctx, `
		INSERT INTO notice (id, recipient_user_id, kind, subject, body, captured_by,
		                    dedupe_key, target_type, target_id, origin)
		VALUES (`+placeholders+`)
		ON CONFLICT (recipient_user_id, dedupe_key) WHERE dedupe_key IS NOT NULL
		DO NOTHING
		RETURNING created_at`,
		args...)
	switch scanErr := row.Scan(&createdAt); {
	case errors.Is(scanErr, pgx.ErrNoRows):
		return storedNotice(ctx, tx, in.Recipient, in.DedupeKey)
	case scanErr != nil:
		return Notice{}, fmt.Errorf("notices: recording the notice: %w", scanErr)
	}
	auditID, err := storekit.AuditEventWithEvidence(ctx, tx, "create", "notice", id,
		map[string]any{"kind": in.Kind, "recipient_user_id": in.Recipient.String()}, evidence)
	if err != nil {
		return Notice{}, err
	}
	recipientID := in.Recipient.UUID
	if err := storekit.EmitEvent(ctx, tx, auditID, recipientID, crmcontracts.PublicEventNoticeCreated{
		NoticeId:        openapi_types.UUID(id),
		RecipientUserId: openapi_types.UUID(recipientID),
		Kind:            in.Kind,
	}); err != nil {
		return Notice{}, err
	}
	return Notice{
		ID: id, Kind: in.Kind, Subject: subject, Body: body,
		Target: in.Target, CreatedAt: createdAt, Origin: in.Origin,
	}, nil
}

// storedNotice answers the notice a repeat delivery collided with — ALL of it,
// not its id with this call's text pinned beside it.
//
// The two need not agree. A dedupe key names the EVENT, and a second delivery
// can carry a subject reworded since, or a kind a later version spells
// differently. Returning the stored id with the replay's words would put a
// notice on screen that the reader cannot find anywhere, and that nothing in the
// database says. The target comes back with them for the same reason: a second
// delivery can name a different entity — an event replayed after the automation
// was repointed — and answering the stored notice's id with this call's target
// would send a reader to a record the notice they can see says nothing about.
func storedNotice(ctx context.Context, tx pgx.Tx, recipient ids.UserID, dedupeKey string) (Notice, error) {
	var stored Notice
	var storedType *string
	var storedID *ids.UUID
	if err := tx.QueryRow(ctx, `
		SELECT id, kind, subject, body, target_type, target_id, created_at, origin FROM notice
		 WHERE recipient_user_id = $1 AND dedupe_key = $2`,
		recipient, dedupeKey,
	).Scan(&stored.ID, &stored.Kind, &stored.Subject, &stored.Body,
		&storedType, &storedID, &stored.CreatedAt, &stored.Origin); err != nil {
		return Notice{}, fmt.Errorf("notices: reading the notice that already stands: %w", err)
	}
	if storedType != nil && storedID != nil {
		stored.Target = Target{Type: *storedType, ID: *storedID}
	}
	return stored, nil
}

// notTheReadersOwnStageMove is true of a notice the reader was ever shown: it
// declines the stage changes the recipient made themselves.
//
// Delivery declines owner-made stage changes in automation.stageChangeNotify,
// and this is the SQL mirror of that decision. It excludes rather than deletes:
// the history stays, and nothing pretends the recipient acknowledged it.
//
// ONE spelling for every reader of the table — the attention lane's unread
// query, the notification centre's history and badge, and the count
// MarkAllRead answers with. Each of those would have to change together, and a
// second copy would make one of them the place a rep's own stage moves
// reappear.
//
// A boolean expression over `notice` columns only, so it composes into a WHERE
// arm and into RETURNING alike.
const notTheReadersOwnStageMove = `NOT coalesce(
	kind = 'automation' AND target_type = 'deal'
	AND (origin ? 'stage_change' OR starts_with(dedupe_key, 'stage_change_notify:'))
	AND origin->>'actor_type' = 'human'
	AND lower(origin->>'actor_id') IN
	  (recipient_user_id::text, 'human:' || recipient_user_id::text), false)`

// UnreadFor answers the CALLING contact's own unread notices, newest first,
// bounded. The contact comes from the bound principal and is not a parameter
// — another contact's notices cannot be expressed — and a caller with no
// contact behind it is refused with the permission sentinel, which the
// attention feed renders as a withheld lane.
func (s *Store) UnreadFor(ctx context.Context, limit int) ([]Notice, error) {
	seat, err := actingSeat(ctx, "reading your notices")
	if err != nil {
		return nil, err
	}
	var unread []Notice
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		rows, txErr := tx.Query(ctx, `
			SELECT id, kind, subject, body, target_type, target_id, created_at, origin
			  FROM notice
			 WHERE recipient_user_id = $1 AND read_at IS NULL
			   AND `+notTheReadersOwnStageMove+`
			 ORDER BY created_at DESC, id DESC
			 LIMIT $2`, seat, limit)
		if txErr != nil {
			return txErr
		}
		defer rows.Close()
		unread = []Notice{}
		for rows.Next() {
			var n Notice
			// Both halves are nullable and the table pairs them, so either
			// arriving alone is a row the constraint should have refused.
			var targetType *string
			var targetID *ids.UUID
			if scanErr := rows.Scan(&n.ID, &n.Kind, &n.Subject, &n.Body,
				&targetType, &targetID, &n.CreatedAt, &n.Origin); scanErr != nil {
				return scanErr
			}
			if targetType != nil && targetID != nil {
				n.Target = Target{Type: *targetType, ID: *targetID}
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
// the statement — another contact's notice reads as absent (404), so its
// existence stays hidden — and idempotent: marking a read notice again is a
// no-op success, because the reader's goal state already holds. The
// read-state flip is a mutation and carries the write shape.
func (s *Store) MarkRead(ctx context.Context, id ids.UUID) error {
	seat, err := actingSeat(ctx, "marking a notice read")
	if err != nil {
		return err
	}
	return s.db.Tx(ctx, func(tx pgx.Tx) error {
		// The write claims the unread row alone, so two concurrent settles
		// race for one row lock and exactly one announces; the loser's
		// existence probe below turns "no unread row" into either the
		// idempotent success or the stranger's 404.
		tag, err := tx.Exec(ctx, `
			UPDATE notice SET read_at = now()
			 WHERE id = $1 AND recipient_user_id = $2 AND read_at IS NULL`,
			id, seat)
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
				id, seat).Scan(&one); err != nil {
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
		// entity, and the notice's lifecycle is that one contact's to hear.
		return storekit.EmitEvent(ctx, tx, auditID, seat, crmcontracts.PublicEventNoticeRead{
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
