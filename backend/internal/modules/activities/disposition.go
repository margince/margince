// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// What a rep decided about a waiting message, made durable.
//
// The Worklist claims to be finite: work it and it empties. Until now a message
// the rep had already judged had no way to leave — the newsletter that is not a
// customer, the reply that belongs to a colleague, the one they will get to on
// Thursday — so it returned every morning and the count above it stayed wrong.
//
// Two decisions with two reaches, which is why there are two tables and two
// write paths rather than one with a flag:
//
//   - NOT SALES is a fact about the THREAD. Recognizing the procurement
//     newsletter settles what the message is, for everybody.
//   - SNOOZED and NOT MINE are one reader's own. A rep putting a reply down
//     until Thursday must not take it off the colleague who owns the deal, and
//     saying "this is not mine" is precisely saying it belongs to somebody who
//     still has to see it.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// The reader-scoped states. The table's CHECK holds the same set, so a value
// here the constraint refuses fails loudly on the write rather than silently
// storing a state no read looks for.
const (
	// dispositionField names what a judgement IS, in the audit after-image and
	// in the effect a system re-arm plans. One spelling, because the two are
	// read together when somebody asks what put a message back.
	dispositionField = "disposition"

	stateSnoozed = "snoozed"
	stateNotMine = "not_mine"
)

// The thread-scoped judgement and the two UNDO verbs, named for the reason the
// pair above are: the response metric counts what was put DOWN and must not
// count a reader taking it back, and a literal at each of those sites is a
// filter that silently stops matching.
const (
	stateNotSales   = "not_sales"
	stateSalesAgain = "sales_again"
	statePickedUp   = "picked_up"
)

// readerOrNobody is whose set-asides a read should apply.
//
// The nil UUID for a caller with no person behind it, which matches no
// reader_state row — so a system pass sees every waiting message, because a
// background job has set nothing aside. Returning the acting user of an agent
// or connector principal instead would apply a human's private snoozes to work
// that human never asked for.
func readerOrNobody(ctx context.Context) ids.UUID {
	actor, ok := principal.Actor(ctx)
	if !ok || actor.Type != principal.PrincipalHuman {
		return ids.UUID{}
	}
	return actor.UserID
}

// SetThreadNotSales records that a conversation is not sales work.
//
// The judgement is written against the THREAD the message belongs to, so the
// next reply on it arrives already judged. Keyed on one activity id it would be
// undone by that reply: the newer inbound is a different row and becomes the
// thread's waiting candidate.
//
// Idempotent: saying it twice is the same statement, and the second records who
// last said it. The row's presence IS the judgement — there is no false to
// store — so clearing it is a delete rather than a flag.
func (s *Store) SetThreadNotSales(ctx context.Context, id ids.ActivityID) error {
	return s.judgeMessage(ctx, id, func(ctx context.Context, tx pgx.Tx, capturedBy string) error {
		thread, err := threadOf(ctx, tx, id)
		if err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO activity_sales_state (thread_key, kind, channel_provider, set_by)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT (thread_key, kind, channel_provider)
			DO UPDATE SET set_by = EXCLUDED.set_by, set_at = now()`,
			thread.key, thread.kind, thread.provider, capturedBy); err != nil {
			return fmt.Errorf("activities: recording the thread as not sales: %w", err)
		}
		return s.recordDisposition(ctx, tx, id, stateNotSales, nil, "")
	})
}

// ClearThreadNotSales takes back the judgement — the conversation is sales work
// after all, and belongs in the queue again.
func (s *Store) ClearThreadNotSales(ctx context.Context, id ids.ActivityID) error {
	return s.judgeMessage(ctx, id, func(ctx context.Context, tx pgx.Tx, _ string) error {
		thread, err := threadOf(ctx, tx, id)
		if err != nil {
			return err
		}
		tag, err := tx.Exec(ctx, `
			DELETE FROM activity_sales_state
			 WHERE thread_key = $1 AND kind = $2 AND channel_provider = $3`,
			thread.key, thread.kind, thread.provider)
		if err != nil {
			return fmt.Errorf("activities: clearing the not-sales judgement: %w", err)
		}
		if tag.RowsAffected() == 0 {
			// Nothing was withdrawn, so nothing is announced. The caller's goal
			// state already holds — which is why this is a success — but an
			// audit row and a public event would both say somebody took back a
			// decision that was never made.
			return nil
		}
		return s.recordDisposition(ctx, tx, id, stateSalesAgain, nil, "")
	})
}

// threadIdentity is what makes two messages the same conversation.
//
// The same triple the waiting query matches replies by, and for its reason: a
// mail thread key comes from headers the sender controls, and channel keys
// share the flat namespace with them, so key alone would let a crafted
// References value silence an unrelated conversation.
type threadIdentity struct {
	key      string
	kind     string
	provider string
}

// threadOf reads the conversation one message belongs to, and refuses anything
// that is not an inbound message in a thread.
//
// WHAT A DISPOSITION IS ABOUT bounds what may carry one. Every verb here says
// something about an unanswered inbound conversation — "not sales", "not mine",
// "later" — and none of them means anything on a note, a task, a meeting or the
// workspace's own outbound reply. Without this check any discoverable activity
// could be given durable state plus an audit row and a public event, and the
// rows would sit in two tables that no read ever consults.
//
// A message with NO thread key is refused for its own reason: there would be
// nothing to hold the judgement against, and an empty key would silence every
// other unthreaded message of the same kind at once — the failure the waiting
// query already avoids by excluding them.
func threadOf(ctx context.Context, tx pgx.Tx, id ids.ActivityID) (threadIdentity, error) {
	var thread threadIdentity
	var key, provider *string
	var direction string
	if err := tx.QueryRow(ctx,
		`SELECT thread_key, kind, channel_provider, direction FROM activity WHERE id = $1`,
		id).Scan(&key, &thread.kind, &provider, &direction); err != nil {
		return threadIdentity{}, fmt.Errorf("activities: reading the message's thread: %w", err)
	}
	if thread.kind != kindEmail && thread.kind != kindMessage {
		return threadIdentity{}, &values.ParseError{
			Field: "id", Code: "not_a_message",
			Message: "only an email or a channel message carries a disposition",
		}
	}
	if direction != directionInbound {
		return threadIdentity{}, &values.ParseError{
			Field: "id", Code: "not_inbound",
			Message: "a disposition answers a message somebody sent US",
		}
	}
	if key == nil || *key == "" {
		return threadIdentity{}, &values.ParseError{
			Field: "id", Code: "message_has_no_thread",
			Message: "this message belongs to no conversation, so there is no thread to judge",
		}
	}
	thread.key = *key
	if provider != nil {
		thread.provider = *provider
	}
	return thread, nil
}

// The other two halves of what the waiting lane reads. kindEmail is already
// spelled in this package (pipelinefacts.go) and is reused rather than given a
// second spelling here.
const (
	kindMessage      = "message"
	directionInbound = "inbound"
)

// SnoozeMessage puts one message down for this reader until its reopen
// condition is met.
//
// A `time` snooze names a moment, and one already past is refused rather than
// stored: it would write a row that hides nothing and read to the rep as a
// snooze that did not take. Judged against the STORE's clock, which the
// scheduling suites inject — a test driving a snooze past its moment must not
// need the wall clock to reach it.
//
// `reply` and `meeting` name no moment at all. They wait on rows the waiting
// query already reads, so the condition is stored and evaluated there rather
// than turned into a guessed date here.
func (s *Store) SnoozeMessage(
	ctx context.Context, id ids.ActivityID, on values.ReopenCondition,
	until *time.Time, ref *ids.UUID,
) error {
	if on.WantsInstant() {
		if until == nil {
			return &values.ParseError{
				Field: fieldSnoozedUntil, Code: "snooze_needs_a_moment",
				Message: "a snooze that waits on the clock names the moment it lifts",
			}
		}
		if !until.After(s.now()) {
			return &values.ParseError{
				Field: fieldSnoozedUntil, Code: "snooze_in_the_past",
				Message: "a snooze names a moment still ahead",
			}
		}
	} else if until != nil {
		return &values.ParseError{
			Field: fieldSnoozedUntil, Code: "snooze_has_no_moment",
			Message: "a snooze waiting on a reply or a meeting lifts when that happens, not on a date",
		}
	}
	if on.NeedsReference() != (ref != nil) {
		return &values.ParseError{
			Field: "reopen_ref", Code: "reopen_ref_shape",
			Message: "only a snooze waiting on a meeting names the meeting it waits for",
		}
	}
	return s.setReaderState(ctx, id, stateSnoozed, until, on, ref)
}

// SetMessageNotMine records that this reader is not the person to answer.
//
// It carries no moment. "This is not my work" does not become false on a
// Thursday — what makes it stop applying is the record changing hands, which
// the reader of these rows judges against the owner of the day.
func (s *Store) SetMessageNotMine(ctx context.Context, id ids.ActivityID) error {
	return s.setReaderState(ctx, id, stateNotMine, nil, "", nil)
}

// ClearMessageDisposition picks a message back up — the undo behind every
// set-aside verb.
func (s *Store) ClearMessageDisposition(ctx context.Context, id ids.ActivityID) error {
	return s.judgeMessage(ctx, id, func(ctx context.Context, tx pgx.Tx, _ string) error {
		reader := readerOrNobody(ctx)
		if reader.IsZero() {
			return apperrors.ErrPermissionDenied
		}
		tag, err := tx.Exec(ctx,
			`DELETE FROM activity_reader_state WHERE activity_id = $1 AND reader_id = $2`,
			id, reader)
		if err != nil {
			return fmt.Errorf("activities: picking the message back up: %w", err)
		}
		if tag.RowsAffected() == 0 {
			// The reader had set nothing aside. A success, for the reason the
			// sibling above states, and silent for the same one.
			return nil
		}
		return s.recordDisposition(ctx, tx, id, statePickedUp, nil, "")
	})
}

// setReaderState writes one reader's own judgement about one message.
func (s *Store) setReaderState(
	ctx context.Context, id ids.ActivityID, state string, until *time.Time,
	on values.ReopenCondition, ref *ids.UUID,
) error {
	return s.judgeMessage(ctx, id, func(ctx context.Context, tx pgx.Tx, capturedBy string) error {
		reader := readerOrNobody(ctx)
		if reader.IsZero() {
			// A reader-scoped judgement needs a reader. An agent or system
			// principal has no queue of its own to clear, and writing one under
			// the acting human's id would set aside work on their behalf.
			return apperrors.ErrPermissionDenied
		}
		// The meeting a snooze names must exist, be a meeting, and be one this
		// reader may read. Checked inside the transaction so it cannot be
		// archived between the check and the row that waits on it.
		if ref != nil {
			if err := EnsureMeetingReference(ctx, tx, *ref); err != nil {
				return err
			}
		}
		// NULL rather than the empty string when the judgement is not a
		// snooze: the column's CHECK pairs its presence with the snoozed
		// state, and an empty string is present.
		var storedOn *string
		if on != "" {
			raw := string(on)
			storedOn = &raw
		}
		// set_at comes from the STORE's clock, not from now() in SQL.
		//
		// A reply snooze lifts on mail that arrived after the rep put the
		// message down, so set_at is no longer just a record of when — it is
		// one side of that comparison. A wall-clock stamp against injected
		// instants makes every seeded reply look older than the snooze, which
		// is a snooze that never lifts.
		if _, err := tx.Exec(ctx, `
			INSERT INTO activity_reader_state (activity_id, reader_id, state, snoozed_until, reopen_on, reopen_ref, set_by, set_at)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
			ON CONFLICT (activity_id, reader_id) DO UPDATE
			   SET state = EXCLUDED.state,
			       snoozed_until = EXCLUDED.snoozed_until,
			       reopen_on = EXCLUDED.reopen_on,
			       reopen_ref = EXCLUDED.reopen_ref,
			       set_by = EXCLUDED.set_by,
			       set_at = EXCLUDED.set_at`,
			id, reader, state, until, storedOn, ref, capturedBy, s.now().UTC()); err != nil {
			return fmt.Errorf("activities: setting the message aside: %w", err)
		}
		return s.recordDisposition(ctx, tx, id, state, until, on)
	})
}

// judgeMessage is the gate every disposition write takes before it writes.
//
// READING THE CONVERSATION is the licence to judge it, and the content gate is
// what says so — not the discover one. A disposition is a statement about what
// a conversation IS: `not_sales` hides it from the whole workspace, and a
// caller who may know a message exists without reading a word of it has no
// standing to make that claim. readActivity is deliberately discover-gated
// (its own doc says so), which admits exactly that caller.
//
// The object grant is `read`, not `update`. Nothing here changes the message —
// the row is exactly as the customer sent it — and demanding update authority
// would refuse a reader whose whole relationship to the thread is reading it,
// which is the person this queue is built for.
//
// An id the caller cannot read answers not-found, the same as one that does not
// exist, so nothing here confirms a message is there.
func (s *Store) judgeMessage(
	ctx context.Context, id ids.ActivityID, write func(context.Context, pgx.Tx, string) error,
) error {
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		return err
	}
	capturedBy, err := storekit.CapturedBy(ctx)
	if err != nil {
		return err
	}
	return s.tx(ctx, func(tx pgx.Tx) error {
		if err := auth.EnsureActivityContentVisibleLive(ctx, tx, id.UUID); err != nil {
			return err
		}
		return write(ctx, tx, capturedBy)
	})
}

// recordDisposition is the write shape's second half: the audit row and the
// announcement, in the same transaction as the judgement itself.
func (s *Store) recordDisposition(
	ctx context.Context, tx pgx.Tx, id ids.ActivityID, state string, until *time.Time,
	on values.ReopenCondition,
) error {
	after := map[string]any{dispositionField: state}
	if until != nil {
		after["snoozed_until"] = until.UTC()
	}
	if on != "" {
		after["reopen_on"] = string(on)
	}
	auditID, err := storekit.AuditEvent(ctx, tx, "update", "activity", id.UUID, after)
	if err != nil {
		return err
	}
	return storekit.EmitEvent(ctx, tx, auditID, id.UUID, crmcontracts.PublicEventActivityDispositionRecorded{
		ActivityId:  openapi_types.UUID(id.UUID),
		Disposition: crmcontracts.PublicEventActivityDispositionRecordedDisposition(state),
	})
}
