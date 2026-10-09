// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package collections

// A user deciding a tag suggestion. Accept applies the tag as them. Dismiss
// records for the whole workspace that the evidence does not earn the tag.
// Both lock the suggestion and re-read it under the caller's visibility first.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// TagSuggestionDecidedError is a decision on a suggestion somebody already
// decided. It answers 409.
type TagSuggestionDecidedError struct{ State string }

func (e *TagSuggestionDecidedError) Error() string {
	return "collections: the tag suggestion is already " + e.State
}

// MessageFault names the state the suggestion is in, so the caller can reload.
func (e *TagSuggestionDecidedError) MessageFault() (code, message string) {
	return "suggestion_decided", "This suggestion was already " + e.State + ". Reload to see where it stands."
}

func (e *TagSuggestionDecidedError) Unwrap() error { return apperrors.ErrConflict }

// AcceptTagSuggestion applies the suggested tag as the caller and records the
// decision, in one transaction. The tag's audit row names the suggestion.
func (s *Store) AcceptTagSuggestion(ctx context.Context, id ids.UUID) (TagSuggestion, error) {
	var out TagSuggestion
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		current, err := openTagSuggestionForDecision(ctx, tx, id)
		if err != nil {
			return err
		}
		if _, err := applyTagTx(ctx, tx, current.TagID, current.EntityType, current.EntityID,
			map[string]any{"tag_suggestion_id": id}); err != nil {
			return err
		}
		out = current
		out.State = TagSuggestionAccepted
		return recordTagSuggestionDecision(ctx, tx, id, TagSuggestionAccepted)
	})
	return out, err
}

// DismissTagSuggestion records that the evidence does not earn the tag. The
// tag is proposed on this record again only on evidence newer than now.
func (s *Store) DismissTagSuggestion(ctx context.Context, id ids.UUID) (TagSuggestion, error) {
	var out TagSuggestion
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		current, err := openTagSuggestionForDecision(ctx, tx, id)
		if err != nil {
			return err
		}
		out = current
		out.State = TagSuggestionDismissed
		return recordTagSuggestionDecision(ctx, tx, id, TagSuggestionDismissed)
	})
	return out, err
}

// openTagSuggestionForDecision locks the suggestion under the caller's
// visibility and holds the caller to what applying the tag would ask: write
// authority over the live record. Deciding either way settles the tag on the
// record for everybody, so a reader who could not tag it may not decide. Only
// then does a decided suggestion answer 409, so the conflict confirms nothing
// to a caller the checks above refuse.
func openTagSuggestionForDecision(ctx context.Context, tx pgx.Tx, id ids.UUID) (TagSuggestion, error) {
	current, err := readVisibleTagSuggestionTx(ctx, tx, id, tagSuggestionRead{evidence: true}, true)
	if err != nil {
		return TagSuggestion{}, err
	}
	if err := auth.Require(ctx, current.EntityType, principal.ActionUpdate); err != nil {
		return TagSuggestion{}, err
	}
	if err := auth.EnsureWritableLive(ctx, tx, current.EntityType, current.EntityID); err != nil {
		return TagSuggestion{}, err
	}
	if current.State != TagSuggestionOpen {
		return TagSuggestion{}, &TagSuggestionDecidedError{State: current.State}
	}
	return current, nil
}

// recordTagSuggestionDecision moves an open suggestion to its decided state,
// with its audit row in the same transaction.
func recordTagSuggestionDecision(ctx context.Context, tx pgx.Tx, id ids.UUID, state string) error {
	actor, err := storekit.Actor(ctx)
	if err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `
		UPDATE tag_suggestion SET state = $2, decided_by = $3, decided_at = now()
		 WHERE id = $1 AND state = 'open'`,
		id, state, storekit.UUIDOrNil(actor.UserID))
	if err != nil {
		return fmt.Errorf("collections: recording the tag suggestion decision: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return &TagSuggestionDecidedError{State: "decided"}
	}
	return recordTagSuggestionState(ctx, tx, id, state)
}
