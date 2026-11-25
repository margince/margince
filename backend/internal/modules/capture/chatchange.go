// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// The one entry every chat network changes a delivered message through, beside
// FileChatMessage, which files it. A network states what the provider reported
// (connector.ChatChange) and what its connection is (connector.ChatBinding).
// The key, the acting member and the seat are derived as for filing. A change
// therefore reaches the record the original reached.

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// ChatSink is the Sink as a chat network uses it: it files messages and acts
// on what the provider later says about them.
type ChatSink interface {
	connector.Sink
	connector.ChatChanger
}

// ChangeChatMessage acts on one provider report about a delivered chat message.
//
// An edit whose original has not landed files the message from the edit's own
// restatement. The late original then finds the row and changes nothing. A
// provider that does not restate gets ChatChangeNotFiled and retries later.
//
// A delivery status never creates a conversation entry. No delivery state is
// kept for a captured message, so a status is noted and written nowhere.
func ChangeChatMessage(
	ctx context.Context, sink ChatSink, binding connector.ChatBinding, change connector.ChatChange,
) (connector.ChatChangeOutcome, error) {
	key, err := chatChangeKey(binding, change)
	if err != nil {
		return "", err
	}
	if err := validChatChange(change); err != nil {
		return "", err
	}
	if change.Kind == connector.ChatDeliveryReported {
		slog.DebugContext(ctx, "capture: a chat delivery status writes nothing to the conversation",
			"provider", binding.Provider, "status", string(change.Status))
		return connector.ChatChangeNoted, nil
	}
	ctx, err = chatActingContext(ctx, binding)
	if err != nil {
		return "", err
	}
	outcome, err := sink.ChangeChat(ctx, key, change)
	if err != nil || outcome != connector.ChatChangeNotFiled || change.Restated == nil {
		return outcome, err
	}
	return fileRestatedEdit(ctx, sink, binding, key, change)
}

// fileRestatedEdit files the message an edit restated, then runs the edit again.
// An original that lands in between wins the row. The second pass then applies
// the edit to it by revision.
func fileRestatedEdit(
	ctx context.Context, sink ChatSink, binding connector.ChatBinding, key connector.NaturalKey, change connector.ChatChange,
) (connector.ChatChangeOutcome, error) {
	restated := *change.Restated
	restated.Revision = change.Revision
	if _, err := FileChatMessage(ctx, sink, binding, restated); err != nil {
		if errors.Is(err, connector.ErrSkip) {
			return connector.ChatChangeStale, nil
		}
		return "", err
	}
	outcome, err := sink.ChangeChat(ctx, key, change)
	if outcome == connector.ChatChangeStale {
		// The restated filing already carries the edit's text and revision.
		return connector.ChatChangeApplied, err
	}
	return outcome, err
}

// chatChangeKey derives the natural key of the message a change is about, from
// the same binding and ids filing uses.
func chatChangeKey(binding connector.ChatBinding, change connector.ChatChange) (connector.NaturalKey, error) {
	if err := validChatBinding(binding); err != nil {
		return connector.NaturalKey{}, err
	}
	msg := connector.ChatMessage{
		ConversationID: change.ConversationID, IdempotencyKey: change.IdempotencyKey, Keys: change.Keys,
	}
	if change.Keys != nil {
		if change.Keys.SourceID == "" {
			return connector.NaturalKey{}, fmt.Errorf("%w: a chat change's own keys name no source id", apperrors.ErrInvalidArgument)
		}
	} else if err := validScopedIDs(binding, msg.ConversationID, msg.IdempotencyKey); err != nil {
		return connector.NaturalKey{}, err
	}
	keys := chatKeys(binding, msg)
	return connector.NaturalKey{
		SourceSystem:          chatSourceSystem(binding),
		SourceID:              keys.SourceID,
		SourceIDNamesAContact: keys.SourceIDNamesAContact,
	}, nil
}

// validChatChange refuses a change that states less than its kind needs.
func validChatChange(change connector.ChatChange) error {
	switch change.Kind {
	case connector.ChatEdited:
		if change.Revision.IsZero() {
			return fmt.Errorf("%w: a chat edit names no edit time, so it could not be ordered against another", apperrors.ErrInvalidArgument)
		}
	case connector.ChatDeleted:
	case connector.ChatDeliveryReported:
		if change.Status == "" {
			return fmt.Errorf("%w: a chat delivery report names no status", apperrors.ErrInvalidArgument)
		}
	default:
		return fmt.Errorf("%w: a chat change is an edit, a deletion or a delivery report", apperrors.ErrInvalidArgument)
	}
	return nil
}
