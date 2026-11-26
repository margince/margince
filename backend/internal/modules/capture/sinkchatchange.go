// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// The Sink's verb for a chat message that already landed and then changed at
// the provider: an edit, or a deletion. FileChatMessage files a message once;
// this acts on what the provider says afterwards, for every chat network.
//
// What the change MEANS is split as the mail verbs split it. activities owns
// the text and writes it; privacy destroys content; capture finds the message
// and answers whose it is. compose injects the two writers.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// ChatEditor writes the text a provider says a chat message now has when the
// edit is later than the stored text. found says a row holds the key; applied
// says the text was written; superseded is the raw row the edit replaced.
// activities owns the write; compose injects it. Nil is a Sink that acts on no
// edits.
type ChatEditor func(
	ctx context.Context, tx pgx.Tx, key connector.NaturalKey, connectorID string, change connector.ChatChange,
) (found, applied bool, superseded ids.UUID, err error)

// WithChatEditor returns a copy that applies a provider's edit of a chat message.
func (s *Sink) WithChatEditor(edit ChatEditor) *Sink {
	c := *s
	c.editChat = edit
	return &c
}

// ChatPurger destroys what a provider-side deletion of one chat message
// destroys. The rules are those of a mail deletion: a colleague's copy and the
// statutory floor keep the message. admit runs first in the transaction that
// selects what to destroy. compose injects it. Nil is a Sink that acts on no
// chat deletions.
type ChatPurger func(ctx context.Context, admit func(context.Context, pgx.Tx) error, seat ids.UUID, key connector.NaturalKey) error

// WithChatPurger returns a copy that acts on a provider's deletion of a chat message.
func (s *Sink) WithChatPurger(purge ChatPurger) *Sink {
	c := *s
	c.purgeChat = purge
	return &c
}

// ChangeChat acts on one provider report about a chat message, satisfying
// connector.ChatChanger. ChangeChatMessage puts the acting member and seat on
// the context, as filing does.
func (s *Sink) ChangeChat(ctx context.Context, key connector.NaturalKey, change connector.ChatChange) (connector.ChatChangeOutcome, error) {
	actor, ok := principal.Actor(ctx)
	if !ok || actor.Type != principal.PrincipalConnector {
		return "", errors.New("capture: a chat change requires a connector principal — the registry builds it, nothing else may")
	}
	if key.SourceSystem == "" || key.SourceID == "" {
		return "", errors.New("capture: a chat change needs a natural key")
	}
	var outcome connector.ChatChangeOutcome
	var err error
	switch change.Kind {
	case connector.ChatEdited:
		outcome, err = s.editChatMessage(ctx, actor, key, change)
	case connector.ChatDeleted:
		outcome, err = s.deleteChatMessage(ctx, actor, key)
	default:
		return "", fmt.Errorf("capture: a chat change of kind %q is not one the sink acts on", change.Kind)
	}
	// An edit that waits for its original keeps its row, which the retry reads.
	if err != nil || outcome == connector.ChatChangeNotFiled {
		return outcome, err
	}
	return outcome, s.dropUnlinkedRaw(ctx, change.StoredOriginal)
}

// dropUnlinkedRaw deletes the raw row a report arrived in unless a message
// holds it as its stored original. A row that changed nothing keeps no text,
// and the privacy purge and erasure reach only the rows an activity links.
func (s *Sink) dropUnlinkedRaw(ctx context.Context, raw ids.UUID) error {
	if raw.IsZero() {
		return nil
	}
	return s.db.Tx(ctx, func(tx pgx.Tx) error {
		var linked bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM activity WHERE raw_capture_id = $1)`, raw).Scan(&linked); err != nil {
			return fmt.Errorf("capture: checking whether a message holds a raw row: %w", err)
		}
		if linked {
			return nil
		}
		return DeleteRawCaptureTx(ctx, tx, raw)
	})
}

// editChatMessage writes an edit under the lock filing the message takes. An
// edit and its original are then applied in a fixed order, whichever arrives
// first.
func (s *Sink) editChatMessage(
	ctx context.Context, actor principal.Principal, key connector.NaturalKey, change connector.ChatChange,
) (connector.ChatChangeOutcome, error) {
	if s.editChat == nil {
		return connector.ChatChangeStale, nil
	}
	outcome := connector.ChatChangeStale
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		if err := memberGuardTx(ctx, tx, actor.UserID); err != nil {
			return err
		}
		if err := lockChatKeyTx(ctx, tx, key); err != nil {
			return err
		}
		// A deletion that was admitted leaves its record first, so an edit that
		// arrives while the content is being destroyed finds the message removed.
		removed, err := chatRemovedTx(ctx, tx, key)
		if err != nil || removed {
			return err
		}
		found, applied, superseded, err := s.editChat(ctx, tx, key, actor.ID, change)
		switch {
		case err != nil:
			return err
		case applied:
			outcome = connector.ChatChangeApplied
			if !superseded.IsZero() {
				return DeleteRawCaptureTx(ctx, tx, superseded)
			}
		case !found:
			outcome = connector.ChatChangeNotFiled
		}
		return nil
	})
	return outcome, err
}

// deleteChatMessage mirrors what RemoveMessage does for mail. A shared account
// removes nothing, because anyone may delete there. A member's own account
// destroys the content unless a colleague holds a copy or the law requires
// keeping it. The row stays erased, so a late original is refused. A message
// not filed yet is remembered as removed, with no content.
func (s *Sink) deleteChatMessage(ctx context.Context, actor principal.Principal, key connector.NaturalKey) (connector.ChatChangeOutcome, error) {
	if _, shared := sharedMailboxOf(ctx); shared || actor.UserID.IsZero() {
		return connector.ChatChangeKept, nil
	}
	var settled connector.ChatChangeOutcome
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		if err := memberGuardTx(ctx, tx, actor.UserID); err != nil {
			return err
		}
		if err := lockChatKeyTx(ctx, tx, key); err != nil {
			return err
		}
		var archived, restricted bool
		err := tx.QueryRow(ctx, `
			SELECT archived_at IS NOT NULL, restricted_at IS NOT NULL FROM activity
			 WHERE source_system = $1 AND source_id = $2
			   AND (captured_by = $3 OR starts_with(captured_by, $3 || ':'))`,
			key.SourceSystem, key.SourceID, actor.ID).Scan(&archived, &restricted)
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			settled = connector.ChatChangeTombstoned
			return writeChatTombstoneTx(ctx, tx, key)
		case err != nil:
			return fmt.Errorf("capture: finding the chat message a provider deleted: %w", err)
		case restricted:
			settled = connector.ChatChangeKept
		case archived:
			settled = connector.ChatChangeStale
		default:
			// Recorded under the lock before the purge runs in transactions of
			// its own, so an edit in between sees the message as removed.
			return writeChatTombstoneTx(ctx, tx, key)
		}
		return nil
	})
	if err != nil || settled != "" {
		return settled, err
	}
	return s.purgeDeletedChat(ctx, actor.UserID, key)
}

// purgeDeletedChat destroys the message the provider deleted, and answers
// whether it did. The purge decides what survives: a colleague's copy and the
// statutory floor keep the message standing.
func (s *Sink) purgeDeletedChat(ctx context.Context, seat ids.UUID, key connector.NaturalKey) (connector.ChatChangeOutcome, error) {
	if s.purgeChat == nil {
		return connector.ChatChangeKept, nil
	}
	admit := func(ctx context.Context, tx pgx.Tx) error { return memberGuardTx(ctx, tx, seat) }
	if err := s.purgeChat(ctx, admit, seat, key); err != nil {
		return "", err
	}
	var archived bool
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT archived_at IS NOT NULL FROM activity WHERE source_system = $1 AND source_id = $2`,
			key.SourceSystem, key.SourceID).Scan(&archived)
	})
	if err != nil {
		return "", fmt.Errorf("capture: reading back a chat message after its deletion: %w", err)
	}
	if archived {
		return connector.ChatChangeRemoved, nil
	}
	return connector.ChatChangeKept, nil
}
