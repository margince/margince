// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// Done on a message we sent: "I need no answer to it". The contact page offers
// it on the card that waits for their reply, and the disposition route takes
// it. Both ask ownWaitingMessageSQL, so a page cannot offer a Done the writer
// refuses, and the route cannot take one the page would never offer.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// OwnMessageDone is the reader's standing on one message we sent.
type OwnMessageDone struct {
	// SetAside: the reader marked the message done or not theirs, or snoozed
	// it and the snooze holds, by the predicate every waiting list applies.
	SetAside bool
	// CanMark: MarkMessageHandled would store Done on it for this reader.
	CanMark bool
}

// OwnMessageDoneState reads the reader's standing on message id as of asOf.
// A caller with no colleague behind it has neither: it sets nothing aside and
// may mark nothing.
func (s *Store) OwnMessageDoneState(ctx context.Context, tx pgx.Tx, id ids.ActivityID, asOf time.Time) (OwnMessageDone, error) {
	reader := readerOrNobody(ctx)
	if reader.IsZero() {
		return OwnMessageDone{}, nil
	}
	setAside, err := readerSetAside(ctx, tx, id, reader, asOf)
	if err != nil {
		return OwnMessageDone{}, err
	}
	canMark, err := s.mayMarkOwnMessage(ctx, tx, id, reader)
	if err != nil {
		return OwnMessageDone{}, err
	}
	return OwnMessageDone{SetAside: setAside, CanMark: canMark}, nil
}

func readerSetAside(ctx context.Context, tx pgx.Tx, id ids.ActivityID, reader ids.UUID, asOf time.Time) (bool, error) {
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	backContent, err := auth.ActivityContentClause(ctx, "back", arg)
	if err != nil {
		return false, err
	}
	wake := messageSnoozeLiftedSQL(fmt.Sprintf("$%d::timestamptz", arg(asOf)), backContent)
	readerPos, idPos := arg(reader), arg(id)
	query := fmt.Sprintf(`SELECT %s FROM activity a WHERE a.id = $%d`,
		readerSetAsideSQL(fmt.Sprintf("$%d", readerPos), wake), idPos)
	var setAside bool
	err = tx.QueryRow(ctx, query, args...).Scan(&setAside)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("activities: reading whether the reader set the message aside: %w", err)
	}
	return setAside, nil
}

// mayMarkOwnMessage asks what judgeMessage and ensureDoneMessage ask before the
// write, and answers false where either would refuse.
func (s *Store) mayMarkOwnMessage(ctx context.Context, tx pgx.Tx, id ids.ActivityID, reader ids.UUID) (bool, error) {
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		return refusedOrFault(err)
	}
	if err := auth.EnsureActivityContentVisibleLive(ctx, tx, id.UUID); err != nil {
		return refusedOrFault(err)
	}
	if err := s.ensureOwnWaitingMessage(ctx, tx, id, reader); err != nil {
		return refusedOrFault(err)
	}
	return true, nil
}

// refusedOrFault turns a refusal into "may not" and keeps every other error.
func refusedOrFault(err error) (bool, error) {
	var refused *values.ParseError
	if errors.As(err, &refused) || errors.Is(err, apperrors.ErrPermissionDenied) || errors.Is(err, apperrors.ErrNotFound) {
		return false, nil
	}
	return false, err
}

// ensureDoneMessage is the shape Done takes on the disposition route: their
// message waiting for our answer, or our own message still waiting for theirs.
func (s *Store) ensureDoneMessage(ctx context.Context, tx pgx.Tx, id ids.ActivityID) error {
	kind, direction, err := activityShape(ctx, tx, id)
	if err != nil {
		return err
	}
	inbound := inboundMessageShape(kind, direction)
	if inbound == nil {
		return nil
	}
	if ownMessageShape(kind, direction) != nil {
		return inbound
	}
	return s.ensureOwnWaitingMessage(ctx, tx, id, readerOrNobody(ctx))
}

// ensureOwnWaitingMessage refuses a message we sent unless it is the one the
// contact page's waiting card rests on for this reader.
func (s *Store) ensureOwnWaitingMessage(ctx context.Context, tx pgx.Tx, id ids.ActivityID, reader ids.UUID) error {
	addresses, err := s.readerAddressList(ctx, tx, reader)
	if err != nil {
		return err
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	laterContent, err := auth.ActivityContentClause(ctx, "later", arg)
	if err != nil {
		return err
	}
	predicate := ownWaitingMessageSQL(fmt.Sprintf("$%d", arg(reader)), fmt.Sprintf("$%d", arg(addresses)), laterContent)
	var kind string
	var direction *string
	var waiting bool
	query := fmt.Sprintf(`SELECT a.kind, a.direction, %s FROM activity a
		WHERE a.id = $%d AND a.archived_at IS NULL`, predicate, arg(id))
	err = tx.QueryRow(ctx, query, args...).Scan(&kind, &direction, &waiting)
	if errors.Is(err, pgx.ErrNoRows) {
		return apperrors.ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("activities: reading whether the message still waits on its contact: %w", err)
	}
	if err := ownMessageShape(kind, direction); err != nil {
		return err
	}
	if !waiting {
		return &values.ParseError{
			Field: "id", Code: "not_your_waiting_message",
			Message: "Done on a message we sent takes your own newest message to a contact, before anyone wrote again",
		}
	}
	return nil
}

// ownWaitingMessageSQL is whether the reader sent message a, and for a contact
// it went to it is still the newest message either way. Those are the facts
// the waiting card shows. laterContent gates the newer messages, so only one
// the reader may read can end the wait, as on the page.
func ownWaitingMessageSQL(reader, addresses, laterContent string) string {
	return `(` + sentByReaderSQL(reader, addresses) + `
	   AND EXISTS (SELECT 1 FROM activity_link sent
	        WHERE sent.activity_id = a.id AND sent.contact_id IS NOT NULL
	          AND NOT EXISTS (SELECT 1 FROM activity_link heard
	                JOIN activity later ON later.id = heard.activity_id
	               WHERE heard.contact_id = sent.contact_id AND later.id <> a.id
	                 AND later.archived_at IS NULL AND later.kind IN ('` + kindEmail + `', '` + kindMessage + `')
	                 AND later.direction IN ('inbound', 'outbound')
	                 AND (later.occurred_at, later.id) > (a.occurred_at, a.id) -- occurred_at raw: which message is newer, an ordering of rows
	                 AND ` + laterContent + `)))`
}
