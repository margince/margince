// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package chatlinks

import (
	"net/http"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/chatlinks/messaging"
)

func wireMember(member messaging.Member) crmcontracts.MessengerMember {
	out := crmcontracts.MessengerMember{Id: member.ID, Name: member.Name, Active: member.Active, External: member.External}
	if member.Bot {
		out.Bot = &member.Bot
	}
	if member.Email != "" {
		out.Email = &member.Email
	}
	return out
}

func wireMembers(members []messaging.Member) []crmcontracts.MessengerMember {
	out := make([]crmcontracts.MessengerMember, 0, len(members))
	for _, member := range members {
		out = append(out, wireMember(member))
	}
	return out
}

func wireConversation(room messaging.Conversation) crmcontracts.MessengerConversation {
	out := crmcontracts.MessengerConversation{SupportsOutcomeDraft: room.SupportsOutcomeDraft, SourceUrl: &room.SourceURL, Id: room.ID, Name: room.Name, Kind: crmcontracts.MessengerConversationKind(room.Kind), AudienceKnown: room.AudienceKnown, External: room.External, Members: wireMembers(room.Members), SupportsThreads: &room.SupportsThreads}
	if room.Access != "" {
		access := crmcontracts.MessengerConversationAccess(room.Access)
		out.Access = &access
	}
	return out
}

func wireMessenger(ctx MessengerContext) crmcontracts.MessengerContext {
	out := crmcontracts.MessengerContext{Provider: crmcontracts.MessengerProvider(ctx.Grant.Provider), Workspace: ctx.Grant.Workspace, GrantVersion: ctx.Grant.Version, Sender: wireMember(ctx.Sender), AudienceKey: ctx.AudienceKey, Conversation: wireConversation(ctx.Conversation)}
	if ctx.Unresolved != nil {
		attempt := wireAttempt(*ctx.Unresolved)
		out.UnresolvedAttempt = &attempt
	}
	return out
}

func wireAttempt(attempt MessengerAttempt) crmcontracts.MessengerAttempt {
	return crmcontracts.MessengerAttempt{Id: openapi_types.UUID(attempt.ID), RequestId: openapi_types.UUID(attempt.RequestID), ConversationId: attempt.Conversation, ProviderMessageId: attempt.Message, State: crmcontracts.MessengerAttemptState(attempt.State), UpdatedAt: attempt.UpdatedAt, FailureCode: &attempt.FailureCode, RetryAt: attempt.RetryAt}
}

func wireMessengerPage(page messaging.Page) crmcontracts.MessengerPage {
	out := crmcontracts.MessengerPage{NextCursor: page.Next, Messages: make([]crmcontracts.MessengerMessage, 0, len(page.Messages))}
	if page.Next != "" {
		direction := crmcontracts.MessengerPageCursorDirectionOlder
		if page.NextDirection == "newer" {
			direction = crmcontracts.MessengerPageCursorDirectionNewer
		}
		out.CursorDirection = &direction
	}
	for _, m := range page.Messages {
		out.Messages = append(out.Messages, crmcontracts.MessengerMessage{ReplyCount: &m.ReplyCount, ContentIncomplete: &m.ContentIncomplete, SourceUrl: &m.SourceURL, Id: m.ID, RootId: m.Root, Author: m.Author, Text: m.Text, At: m.At, Edited: m.Edited, Deleted: m.Deleted})
	}
	return out
}

func optionalString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

// WriteMessengerFailure keeps composed consent routes on the participant API's error contract.
func WriteMessengerFailure(w http.ResponseWriter, r *http.Request, err error) { writeErr(w, r, err) }
