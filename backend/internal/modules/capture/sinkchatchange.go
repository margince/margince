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
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// ChatEditor writes the text a provider says a chat message now has when the
// edit is newer than the stored text. found says a row holds the key; applied
// says the text was written. activities owns the write; compose injects it. Nil
// is a Sink that acts on no edits.
type ChatEditor func(
	ctx context.Context, tx pgx.Tx, key connector.NaturalKey, connectorID, body string, revision time.Time,
) (found, applied bool, err error)

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
	switch change.Kind {
	case connector.ChatEdited:
		return s.editChatMessage(ctx, actor, key, change)
	case connector.ChatDeleted:
		return s.deleteChatMessage(ctx, actor, key)
	}
	return "", fmt.Errorf("capture: a chat change of kind %q is not one the sink acts on", change.Kind)
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
		found, applied, err := s.editChat(ctx, tx, key, actor.ID, change.Body, change.Revision)
		switch {
		case err != nil:
			return err
		case applied:
			outcome = connector.ChatChangeApplied
		case found:
		default:
			// Nothing holds the key: either the original has not landed yet, or
			// the provider deleted the message before it did.
			removed, err := chatRemovedTx(ctx, tx, key)
			if err != nil {
				return err
			}
			if !removed {
				outcome = connector.ChatChangeNotFiled
			}
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
