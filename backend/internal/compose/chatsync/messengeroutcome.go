// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package chatsync

import (
	"context"
	"slices"
	"strings"
	"unicode/utf8"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/chatlinks"
	"github.com/margince/margince/backend/internal/modules/chatlinks/chatwire"
	"github.com/margince/margince/backend/internal/modules/chatlinks/messaging"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// DraftMessengerOutcome reuses the restriction-aware summary engine without filing private messages.
func (f *Filer) DraftMessengerOutcome(ctx context.Context, kind string, id ids.UUID, provider, conversation, root string, selected []string) (crmcontracts.MessengerOutcomeDraft, error) {
	if len(selected) < 1 || len(selected) > 100 {
		return crmcontracts.MessengerOutcomeDraft{}, apperrors.ErrInvalidArgument
	}
	initial, err := f.Store.MessengerConversation(ctx, kind, id, provider, conversation)
	if err != nil {
		return crmcontracts.MessengerOutcomeDraft{}, err
	}
	if initial.Conversation.SupportsOutcomeDraft != nil && !*initial.Conversation.SupportsOutcomeDraft {
		return crmcontracts.MessengerOutcomeDraft{}, messaging.ErrUnsupported
	}
	messages, err := f.messengerSelection(ctx, kind, id, provider, conversation, root, selected, initial.Conversation)
	if err != nil {
		return crmcontracts.MessengerOutcomeDraft{}, err
	}
	link, err := outcomeRecordLink(kind, id, provider, conversation, initial.Conversation.Name)
	if err != nil {
		return crmcontracts.MessengerOutcomeDraft{}, err
	}
	reply, err := f.summariseChosen(ctx, link, messages)
	if err != nil {
		return crmcontracts.MessengerOutcomeDraft{}, err
	}
	current, err := f.Store.MessengerConversation(ctx, kind, id, provider, conversation)
	if err != nil {
		return crmcontracts.MessengerOutcomeDraft{}, err
	}
	if current.Grant.Version != initial.Grant.Version || current.AudienceKey != initial.AudienceKey {
		return crmcontracts.MessengerOutcomeDraft{}, apperrors.ErrVersionSkew
	}
	confirmed, err := f.messengerSelection(ctx, kind, id, provider, conversation, root, selected, current.Conversation)
	if err != nil {
		return crmcontracts.MessengerOutcomeDraft{}, err
	}
	if err := confirmMessengerContent(messages, confirmed); err != nil {
		return crmcontracts.MessengerOutcomeDraft{}, err
	}
	return crmcontracts.MessengerOutcomeDraft{Subject: reply.Title, Body: noteBody(reply)}, nil
}

func (f *Filer) messengerSelection(ctx context.Context, kind string, id ids.UUID, provider, conversation, root string, selected []string, room messaging.Conversation) ([]chatwire.Message, error) {
	wanted := map[string]bool{}
	for _, messageID := range selected {
		if messageID == "" || wanted[messageID] {
			return nil, apperrors.ErrInvalidArgument
		}
		wanted[messageID] = true
	}
	authors := map[string]messaging.Member{}
	for _, member := range room.Members {
		authors[member.ID] = member
	}
	messages := []chatwire.Message{}
	cursor := ""
	seen := map[string]bool{}
	runes := 0
	for range 20 {
		page, err := f.Store.MessengerMessages(ctx, kind, id, provider, conversation, root, cursor)
		if err != nil {
			return nil, err
		}
		for _, message := range page.Messages {
			if !wanted[message.ID] {
				continue
			}
			if message.Deleted {
				return nil, messaging.ErrNotFound
			}
			delete(wanted, message.ID)
			author := authors[message.Author]
			text := message.Text
			if message.ContentIncomplete {
				text += "\n[Some source content was not read]"
			}
			runes += utf8.RuneCountInString(text)
			if runes > 120000 {
				return nil, chatwire.ErrThreadTooLong
			}
			messages = append(messages, chatwire.Message{TS: message.ID, NativeID: message.ID, ThreadTS: message.Root, Text: text, PostedAt: message.At, UserName: author.Name, UserEmail: author.Email})
		}
		if len(wanted) == 0 {
			slices.SortFunc(messages, compareMessengerMessages)
			return messages, nil
		}
		if page.Next == "" || seen[page.Next] {
			return nil, messaging.ErrNotFound
		}
		seen[page.Next], cursor = true, page.Next
	}
	return nil, chatwire.ErrThreadTooLong
}

func outcomeRecordLink(kind string, id ids.UUID, provider, conversation, name string) (chatlinks.Link, error) {
	link := chatlinks.Link{Provider: provider, ChannelSlug: conversation, ChannelName: name, MessageDetail: chatlinks.MessageDetailFull}
	switch kind {
	case chatlinks.RecordCompany:
		link.CompanyID = &id
	case chatlinks.RecordDeal:
		link.DealID = &id
	case chatlinks.RecordProject:
		link.ProjectID = &id
	default:
		return chatlinks.Link{}, apperrors.ErrInvalidArgument
	}
	return link, nil
}

func confirmMessengerContent(messages, confirmed []chatwire.Message) error {
	before, err := nativeContentDigest(messages)
	if err != nil {
		return err
	}
	after, err := nativeContentDigest(confirmed)
	if err != nil {
		return err
	}
	if before != after {
		return apperrors.ErrConflict
	}
	return nil
}

func compareMessengerMessages(a, b chatwire.Message) int {
	if at := a.PostedAt.Compare(b.PostedAt); at != 0 {
		return at
	}
	return strings.Compare(a.TS, b.TS)
}
