// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dealrooms

// Writing to the conversation. The transaction bodies are shared by both
// sides — an author is either a participant or a user, and the SQL does not
// care which — while the entry points differ: the seller's take the seat
// gates, the buyer's (store_public_threads.go) take the session.

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// threadAuthor is who is writing: exactly one of the two is set, which the
// schema's one-author CHECK also holds.
type threadAuthor struct {
	participantID *ids.DealRoomParticipantID
	userID        *ids.UUID
}

func (a threadAuthor) side() string {
	if a.participantID != nil {
		return sideBuyer
	}
	return sideSeller
}

// OpenThreadInput is the validated shape both sides open a thread from.
type OpenThreadInput struct {
	DocumentID     *ids.UUID
	Body           string
	RequiredChange bool
	Source         string
	// RequestID is the client's own id for this attempt, so a repeated delivery
	// lands once. Nil means the caller asked for no protection.
	RequestID *ids.UUID
}

// cleanBody trims a comment and refuses an empty one.
func cleanBody(body string) (string, error) {
	trimmed := strings.TrimSpace(body)
	if trimmed == "" {
		return "", &fieldError{field: "body", code: codeRequired, msg: "a comment needs some words"}
	}
	if len(trimmed) > commentLimit {
		return "", &fieldError{field: "body", code: codeTooLong, msg: fmt.Sprintf("a comment is at most %d characters", commentLimit)}
	}
	return trimmed, nil
}

// commentLimit bounds one comment. Long enough for a real question, short
// enough that the public edge is not a place to store documents.
const commentLimit = 8000

// OpenThread is the seller's side opening a thread.
func (s *Store) OpenThread(ctx context.Context, roomID ids.DealRoomID, in OpenThreadInput) (crmcontracts.DealRoomThread, error) {
	if err := auth.Require(ctx, roomObject, principal.ActionUpdate); err != nil {
		return crmcontracts.DealRoomThread{}, err
	}
	userID, err := writingUser(ctx)
	if err != nil {
		return crmcontracts.DealRoomThread{}, err
	}
	var out crmcontracts.DealRoomThread
	err = s.tx(ctx, func(tx pgx.Tx) error {
		room, err := openRoomForContent(ctx, tx, roomID)
		if err != nil {
			return err
		}
		out, err = openThreadTx(ctx, tx, room, in, threadAuthor{userID: &userID})
		return err
	})
	return out, err
}

// writingUser names the seat behind a seller-side write. A principal with no
// user (an agent acting for nobody, the system) cannot be an author.
func writingUser(ctx context.Context) (ids.UUID, error) {
	p, ok := principal.Actor(ctx)
	if !ok || p.UserID == ids.Nil {
		return ids.Nil, apperrors.ErrPermissionDenied
	}
	return p.UserID, nil
}

// openThreadTx inserts the thread and its first comment, and announces the
// comment. A document thread is pinned to the version the room's manifest
// currently names; a room-level thread has no document and no version.
func openThreadTx(ctx context.Context, tx pgx.Tx, room crmcontracts.DealRoom, in OpenThreadInput, by threadAuthor) (crmcontracts.DealRoomThread, error) {
	capturedBy, err := storekit.CapturedBy(ctx)
	if err != nil {
		return crmcontracts.DealRoomThread{}, err
	}
	roomID := ids.From[ids.DealRoomKind](ids.UUID(room.Id))
	var attachmentID *ids.UUID
	if in.DocumentID != nil {
		id, err := currentVersionOf(ctx, tx, roomID, *in.DocumentID)
		if err != nil {
			return crmcontracts.DealRoomThread{}, err
		}
		attachmentID = &id
	} else if in.RequiredChange {
		return crmcontracts.DealRoomThread{}, &fieldError{field: "required_change", code: "needs_document", msg: "only a thread on a document can require a change"}
	}
	threadID := ids.NewV7()
	// Conflict-first, because the alternative loses the race. A read-then-insert
	// pair lets two deliveries of one attempt both find nothing and both write —
	// which is exactly the double-click this exists to stop, arriving on two
	// connections instead of one. The index decides, not a prior read.
	//
	// The inference list mirrors uq_deal_room_thread_request, predicate included:
	// Postgres matches a partial index only when the statement repeats its WHERE,
	// and a caller that sent no id is outside the index and so conflicts with
	// nobody.
	inserted, err := tx.Exec(ctx,
		`INSERT INTO deal_room_thread (id, room_id, document_id, attachment_id, required_change, author_participant_id, author_user_id, source, captured_by, request_id)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		 ON CONFLICT (room_id, request_id) WHERE request_id IS NOT NULL DO NOTHING`,
		threadID, roomID, in.DocumentID, attachmentID, in.RequiredChange, by.participantID, by.userID, in.Source, capturedBy, in.RequestID)
	if err != nil {
		return crmcontracts.DealRoomThread{}, fmt.Errorf("insert deal room thread: %w", err)
	}
	// Nothing written means a previous delivery of this attempt opened the thread.
	// Returning here is what keeps the audit row and the first comment from
	// happening twice for something the buyer did once.
	if inserted.RowsAffected() == 0 {
		return threadOfRequest(ctx, tx, roomID, in.RequestID)
	}
	if _, err := storekit.Audit(ctx, tx, "create", threadObject, threadID, nil,
		map[string]any{fieldRoomID: roomID.UUID, "document_id": in.DocumentID, "required_change": in.RequiredChange}); err != nil {
		return crmcontracts.DealRoomThread{}, fmt.Errorf("audit deal room thread: %w", err)
	}
	// NO key on the opening comment. The thread row above already carries this
	// attempt's id and both writes are in one transaction, so the thread's own index
	// settles the whole opening — while a second copy of the id on the comment table
	// would collide with a reply that used the same id earlier, which a client is
	// free to do and which would fail a post that is perfectly legitimate.
	first := commentPlacement{threadID: threadID, documentID: in.DocumentID, opensThread: true, requiredChange: in.RequiredChange}
	// Carries no key, so its insert cannot be the one that does nothing.
	if err := postCommentTx(ctx, tx, room, first, in.Body, in.Source, by); err != nil {
		return crmcontracts.DealRoomThread{}, err
	}
	return readThread(ctx, tx, roomID, threadID)
}

// threadOfRequest answers with the thread a previous delivery of this attempt
// opened. Only reached when the insert above conflicted, so the row is there.
//
// Keyed the way uq_deal_room_thread_request is keyed, because it answers the
// conflict that index raised: if the two disagreed about what "the same attempt"
// means, this would miss the row the insert was refused for.
func threadOfRequest(
	ctx context.Context, tx pgx.Tx, roomID ids.DealRoomID, requestID *ids.UUID,
) (crmcontracts.DealRoomThread, error) {
	var threadID ids.UUID
	if err := tx.QueryRow(ctx, `
		SELECT id FROM deal_room_thread WHERE room_id = $1 AND request_id = $2`,
		roomID, requestID).Scan(&threadID); err != nil {
		return crmcontracts.DealRoomThread{}, fmt.Errorf("read the thread a replayed request opened: %w", err)
	}
	return readThread(ctx, tx, roomID, threadID)
}

// currentVersionOf is the attachment the room currently shows for a document.
// Absent for a document of another room, or one removed.
func currentVersionOf(ctx context.Context, tx pgx.Tx, roomID ids.DealRoomID, documentID ids.UUID) (ids.UUID, error) {
	var attachmentID ids.UUID
	err := tx.QueryRow(ctx,
		`SELECT attachment_id FROM deal_room_document WHERE id = $1 AND room_id = $2 AND archived_at IS NULL`,
		documentID, roomID).Scan(&attachmentID)
	if err != nil {
		if err == pgx.ErrNoRows {
			return ids.Nil, apperrors.ErrNotFound
		}
		return ids.Nil, fmt.Errorf("read deal room document version: %w", err)
	}
	return attachmentID, nil
}

// commentPlacement says where a comment lands: which thread, on which
// document, and whether it is the thread's first word (which carries the
// thread's required-change flag into the event).
type commentPlacement struct {
	threadID       ids.UUID
	documentID     *ids.UUID
	opensThread    bool
	requiredChange bool
	// requestID is the client's own id for this attempt. Carried on the row so a
	// repeated delivery is recognised rather than appended.
	requestID *ids.UUID
}

// postCommentTx inserts one comment and announces it. The caller has settled
// that the thread is open and the room live.
func postCommentTx(ctx context.Context, tx pgx.Tx, room crmcontracts.DealRoom, at commentPlacement, body, source string, by threadAuthor) error {
	capturedBy, err := storekit.CapturedBy(ctx)
	if err != nil {
		return err
	}
	commentID := ids.NewV7()
	// Conflict-first for the same reason the thread insert is: two deliveries of
	// one attempt race, and the index is the only thing that sees both.
	inserted, err := tx.Exec(ctx,
		`INSERT INTO deal_room_comment (id, room_id, thread_id, body, author_participant_id, author_user_id, source, captured_by, request_id)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		 ON CONFLICT (room_id, request_id) WHERE request_id IS NOT NULL DO NOTHING`,
		commentID, ids.UUID(room.Id), at.threadID, body, by.participantID, by.userID, source, capturedBy, at.requestID)
	if err != nil {
		return fmt.Errorf("insert deal room comment: %w", err)
	}
	// A previous delivery already posted it, which only a RACE reaches now — the
	// caller settles an ordinary replay before it gets here. Returning without the
	// audit row and the announcement below is what keeps one message from ringing
	// twice.
	if inserted.RowsAffected() == 0 {
		return nil
	}
	auditID, err := storekit.Audit(ctx, tx, "create", commentObject, commentID, nil,
		map[string]any{fieldRoomID: ids.UUID(room.Id), "thread_id": at.threadID, fieldSide: by.side()})
	if err != nil {
		return fmt.Errorf("audit deal room comment: %w", err)
	}
	var docID *openapi_types.UUID
	if at.documentID != nil {
		u := openapi_types.UUID(*at.documentID)
		docID = &u
	}
	// Who spoke and what about, read HERE and carried on the event: a name can
	// change and a document can be retitled or removed, and the timeline entry
	// this event writes has to keep saying what was true when it happened.
	authorName, named, err := commentAuthorName(ctx, tx, by)
	if err != nil {
		return err
	}
	docTitle, titled, err := commentDocumentTitle(ctx, tx, at.documentID)
	if err != nil {
		return err
	}
	posted := crmcontracts.PublicEventDealRoomCommentPosted{
		DealId:         room.DealId,
		ThreadId:       openapi_types.UUID(at.threadID),
		CommentId:      openapi_types.UUID(commentID),
		DocumentId:     docID,
		Side:           by.side(),
		OpensThread:    at.opensThread,
		RequiredChange: &at.requiredChange,
	}
	if named {
		posted.AuthorName = &authorName
	}
	if titled {
		posted.DocumentTitle = &docTitle
	}
	if err := storekit.EmitEvent(ctx, tx, auditID, ids.UUID(room.Id), posted); err != nil {
		return fmt.Errorf("emit deal_room.comment_posted: %w", err)
	}
	return nil
}

// commentAuthorName names whoever spoke: the buyer's seat carries their own
// name, and the seller's is their user record. Nil rather than a placeholder
// when the row has gone — a note that named "Unknown" would be asserting
// something about the contact instead of admitting the record is thin.
// The bool is "there is a name", which is a real answer and not a failure —
// hence (string, bool, error) rather than a nil pointer standing for both.
func commentAuthorName(ctx context.Context, tx pgx.Tx, by threadAuthor) (string, bool, error) {
	var name string
	var err error
	switch {
	case by.participantID != nil:
		err = tx.QueryRow(ctx,
			`SELECT full_name FROM deal_room_participant WHERE id = $1`, *by.participantID).Scan(&name)
	case by.userID != nil:
		err = tx.QueryRow(ctx,
			`SELECT display_name FROM app_user WHERE id = $1`, *by.userID).Scan(&name)
	default:
		return "", false, nil
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("read the comment author's name: %w", err)
	}
	return name, name != "", nil
}

// commentDocumentTitle titles the document a thread is about. Nil for a
// room-level exchange, which is not a missing title but a different kind of
// thread, and the note says so in its own words.
func commentDocumentTitle(ctx context.Context, tx pgx.Tx, documentID *ids.UUID) (string, bool, error) {
	if documentID == nil {
		return "", false, nil
	}
	var title string
	err := tx.QueryRow(ctx,
		`SELECT title FROM deal_room_document WHERE id = $1`, *documentID).Scan(&title)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("read the thread's document title: %w", err)
	}
	return title, title != "", nil
}

// errThreadResolved refuses a reply to a settled thread: reopening is a new
// thread, so the record of what was settled when stays readable.
var errThreadResolved = &stateError{
	code:    "deal_room_thread_resolved",
	current: threadResolved,
	wanted:  "this thread is resolved; open a new one to continue",
}

// replyTx appends to an open thread of the room.
func replyTx(ctx context.Context, tx pgx.Tx, room crmcontracts.DealRoom, threadID ids.UUID, body, source string, by threadAuthor, requestID *ids.UUID) (crmcontracts.DealRoomThread, error) {
	roomID := ids.From[ids.DealRoomKind](ids.UUID(room.Id))
	// Before the state check, because a comment that is ALREADY STORED is not a new
	// post and the thread's state cannot make it one. A buyer whose first delivery
	// landed and whose retry arrives after the seller resolved the thread would
	// otherwise be told their message failed, with no way to send it again.
	//
	// A found row is proof of a replay; an absent one proves nothing, because a
	// concurrent first delivery may not have committed yet. That half stays with the
	// insert's ON CONFLICT — this read answers only the case it can settle.
	if landed, err := commentOfRequest(ctx, tx, roomID, threadID, requestID); err != nil || landed {
		return readThread(ctx, tx, roomID, threadID)
	}
	if _, err := storekit.LockRow(ctx, tx, threadObject, threadID, storekit.NoArchiveColumn); err != nil {
		return crmcontracts.DealRoomThread{}, err
	}
	current, err := readThread(ctx, tx, roomID, threadID)
	if err != nil {
		return crmcontracts.DealRoomThread{}, err
	}
	if current.State != threadOpen {
		return crmcontracts.DealRoomThread{}, errThreadResolved
	}
	var documentID *ids.UUID
	if current.DocumentId != nil {
		u := ids.UUID(*current.DocumentId)
		documentID = &u
	}
	// A replay answers with the thread as it stands. Nothing further to do: the
	// comment is already in it, from the delivery that got here first.
	if err := postCommentTx(ctx, tx, room, commentPlacement{threadID: threadID, documentID: documentID, requestID: requestID}, body, source, by); err != nil {
		return crmcontracts.DealRoomThread{}, err
	}
	return readThread(ctx, tx, roomID, threadID)
}

// Reply is the seller's side answering in a thread.
func (s *Store) Reply(ctx context.Context, roomID ids.DealRoomID, threadID ids.UUID, body, source string, requestID *ids.UUID) (crmcontracts.DealRoomThread, error) {
	if err := auth.Require(ctx, roomObject, principal.ActionUpdate); err != nil {
		return crmcontracts.DealRoomThread{}, err
	}
	userID, err := writingUser(ctx)
	if err != nil {
		return crmcontracts.DealRoomThread{}, err
	}
	var out crmcontracts.DealRoomThread
	err = s.tx(ctx, func(tx pgx.Tx) error {
		room, err := openRoomForContent(ctx, tx, roomID)
		if err != nil {
			return err
		}
		out, err = replyTx(ctx, tx, room, threadID, body, source, threadAuthor{userID: &userID}, requestID)
		return err
	})
	return out, err
}

// ResolveThread closes a thread. Human-only: resolving a required-change
// thread is what unblocks the buyer's confirmation, so a contact stands behind
// it. Already resolved answers the thread unchanged.
func (s *Store) ResolveThread(ctx context.Context, roomID ids.DealRoomID, threadID ids.UUID) (crmcontracts.DealRoomThread, error) {
	if err := auth.Require(ctx, roomObject, principal.ActionUpdate); err != nil {
		return crmcontracts.DealRoomThread{}, err
	}
	if err := auth.RequireHuman(ctx); err != nil {
		return crmcontracts.DealRoomThread{}, err
	}
	userID, err := writingUser(ctx)
	if err != nil {
		return crmcontracts.DealRoomThread{}, err
	}
	var out crmcontracts.DealRoomThread
	err = s.tx(ctx, func(tx pgx.Tx) error {
		room, err := openRoomForContent(ctx, tx, roomID)
		if err != nil {
			return err
		}
		if _, err := storekit.LockRow(ctx, tx, threadObject, threadID, storekit.NoArchiveColumn); err != nil {
			return err
		}
		current, err := readThread(ctx, tx, roomID, threadID)
		if err != nil {
			return err
		}
		if current.State == threadResolved {
			out = current
			return nil
		}
		p := storekit.NewPatch()
		p.Set("state", threadOpen, threadResolved)
		p.Set("resolved_at", nil, time.Now().UTC())
		p.Set("resolved_by_user_id", nil, userID)
		// No If-Match: a thread carries no version column, so there is no
		// precondition to compare. The row lock is what serializes two contacts
		// resolving at once, and it is taken by name — a nil version reads as a
		// caller who had one and declined it.
		lock, err := storekit.LockRow(ctx, tx, threadObject, threadID, storekit.NoArchiveColumn)
		if err != nil {
			return err
		}
		if err := p.ApplyLocked(ctx, tx, lock); err != nil {
			return err
		}
		auditID, err := storekit.Audit(ctx, tx, "resolve", threadObject, threadID, p.Before(), p.After())
		if err != nil {
			return fmt.Errorf("audit deal room thread resolve: %w", err)
		}
		resolved := crmcontracts.PublicEventDealRoomThreadResolved{
			DealId:     room.DealId,
			ThreadId:   openapi_types.UUID(threadID),
			DocumentId: current.DocumentId,
		}
		if err := storekit.EmitEvent(ctx, tx, auditID, ids.UUID(room.Id), resolved); err != nil {
			return fmt.Errorf("emit deal_room.thread_resolved: %w", err)
		}
		out, err = readThread(ctx, tx, roomID, threadID)
		return err
	})
	return out, err
}

// errRequestIDNamesAnotherThread says an attempt id has already been used for a
// comment somewhere else in this room.
var errRequestIDNamesAnotherThread = fmt.Errorf(
	"%w: this request_id was already used for a comment on another thread", apperrors.ErrConflict)

// commentOfRequest reports whether a previous delivery of this attempt already left
// a comment on this thread.
//
// Keyed the way uq_deal_room_comment_request is keyed — (room_id, request_id) — so
// the row it finds is the row the insert would conflict with. The thread is then
// checked SEPARATELY: the index is scoped to the room, so the same id used on a
// different thread is a client reusing an id for two different posts, and answering
// with this thread would silently drop the second one.
func commentOfRequest(
	ctx context.Context, tx pgx.Tx, roomID ids.DealRoomID, threadID ids.UUID, requestID *ids.UUID,
) (bool, error) {
	if requestID == nil {
		return false, nil
	}
	var storedOn ids.UUID
	err := tx.QueryRow(ctx, `
		SELECT thread_id FROM deal_room_comment WHERE room_id = $1 AND request_id = $2`,
		roomID, *requestID).Scan(&storedOn)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read the comment a replayed request left: %w", err)
	}
	if storedOn != threadID {
		return false, errRequestIDNamesAnotherThread
	}
	return true, nil
}
