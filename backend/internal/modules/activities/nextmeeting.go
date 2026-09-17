// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// Is anything booked with this record, and when.
//
// WHY IT IS NOT A SCAN OF THE TIMELINE. The deal card used to answer this by
// walking the page of activities it had already read — 25 rows, newest first.
// That page is a summary, and an absence derived from a summary is not an
// absence: a deal with 25 rows of notes and mail newer than the meeting would
// report nothing booked while the meeting sat on page two, and the card would
// then tell a rep to arrange what they had already arranged.
//
// So the question is asked of the database, ordered the way the answer needs,
// bounded to one row. The same shape compose/contact360's next-meeting section
// already uses for a contact; this is its sibling for any linked record, which
// is why it lives in the module rather than in one of the two readers.
//
// A BOOKED MEETING IS ONE WITHOUT A CONTRARY STATUS. meeting_status NULL means
// nobody recorded one, which is the ordinary case for a meeting somebody typed
// in, and reading it as "not booked" would hide most of the calendar. Only an
// explicit non-booked status (cancelled, proposed) takes a meeting out of the
// answer.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// BookedMeeting is the next thing on the calendar with a record: enough to say
// that something is agreed and to name it, not the meeting's full row.
type BookedMeeting struct {
	ID       ids.UUID
	Subject  string
	StartsAt time.Time
}

// NextBookedMeetingInput narrows the question to one record.
type NextBookedMeetingInput struct {
	// EntityType and EntityID name the record the meeting must be linked to,
	// in the same vocabulary every other link read uses.
	EntityType string
	EntityID   ids.UUID
	// After is the instant a meeting must start later than — the caller's
	// clock, injected, so a test can pin it.
	After time.Time
}

// NextBookedMeeting answers the soonest booked meeting linked to a record,
// reporting whether there is one at all.
//
// No meeting is a real answer and not an error: "nothing is booked" is the
// finding a caller is asking for as often as the meeting itself.
func (s *Store) NextBookedMeeting(
	ctx context.Context, in NextBookedMeetingInput,
) (BookedMeeting, bool, error) {
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		return BookedMeeting{}, false, err
	}
	var meeting BookedMeeting
	var found bool
	err := s.tx(ctx, func(tx pgx.Tx) (err error) {
		meeting, found, err = nextBookedMeeting(ctx, tx, in)
		return err
	})
	return meeting, found, err
}

// NextBookedMeetingTx is NextBookedMeeting inside a caller-opened transaction,
// for a composite read whose sections must describe one instant. Same gate,
// same query; only the transaction is borrowed.
func (s *Store) NextBookedMeetingTx(
	ctx context.Context, tx pgx.Tx, in NextBookedMeetingInput,
) (BookedMeeting, bool, error) {
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		return BookedMeeting{}, false, err
	}
	return nextBookedMeeting(ctx, tx, in)
}

func nextBookedMeeting(
	ctx context.Context, tx pgx.Tx, in NextBookedMeetingInput,
) (BookedMeeting, bool, error) {
	column := linkColumn(in.EntityType)
	if column == "" {
		return BookedMeeting{}, false, &InvalidLinkTypeError{EntityType: in.EntityType}
	}
	// The record the question is narrowed to is gated before it is answered:
	// "is anything booked with this deal" tells the asker the deal exists, so
	// a record out of their scope answers not-found rather than "nothing".
	if err := auth.EnsureVisible(ctx, tx, in.EntityType, in.EntityID); err != nil {
		return BookedMeeting{}, false, err
	}
	args := []any{}
	arg := func(v any) int { args = append(args, v); return len(args) }
	// The timeline's own scope rule, asked of the meeting itself: an activity
	// carries no owner, so who may read one is decided by the records it links
	// to. Spelled through auth so this read and the timeline cannot disagree.
	scope, err := auth.ActivityContentClause(ctx, "a", arg)
	if err != nil {
		return BookedMeeting{}, false, err
	}
	where := []string{
		fmt.Sprintf("a.kind = $%d", arg(string(crmcontracts.ActivityKindMeeting))),
		activityLive,
		"(a.meeting_status IS NULL OR a.meeting_status = 'booked')",
		fmt.Sprintf("a.occurred_at > $%d", arg(in.After)),
		fmt.Sprintf(`EXISTS (SELECT 1 FROM activity_link l WHERE l.activity_id = a.id
			AND l.entity_type = $%d AND l.%s = $%d)`,
			arg(in.EntityType), column, arg(in.EntityID)),
	}
	if scope != "" {
		where = append(where, scope)
	}
	var meeting BookedMeeting
	var subject *string
	err = tx.QueryRow(ctx, `SELECT a.id, a.subject, a.occurred_at
		FROM activity a
		WHERE `+joinAnd(where)+`
		ORDER BY a.occurred_at, a.id
		LIMIT 1`, args...).Scan(&meeting.ID, &subject, &meeting.StartsAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return BookedMeeting{}, false, nil
	}
	if err != nil {
		return BookedMeeting{}, false, fmt.Errorf("read the next booked meeting: %w", err)
	}
	if subject != nil {
		meeting.Subject = *subject
	}
	return meeting, true, nil
}
