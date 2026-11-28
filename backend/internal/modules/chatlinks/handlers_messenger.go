// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package chatlinks

import (
	"errors"
	"net/http"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/chatlinks/messaging"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func (h Handlers) messengerReady(w http.ResponseWriter, r *http.Request) bool {
	if _, err := memberActor(r.Context()); err != nil {
		writeErr(w, r, err)
		return false
	}
	if h.store == nil {
		writeErr(w, r, messaging.ErrUnsupported)
		return false
	}
	return true
}

// GetMessengerIdentity reports personal connection state without returning its credential.
func (h Handlers) GetMessengerIdentity(w http.ResponseWriter, r *http.Request, provider crmcontracts.MessengerProvider) {
	if !h.messengerReady(w, r) {
		return
	}
	out := crmcontracts.MessengerIdentity{Provider: provider, State: "needs_consent"}
	grant, _, err := h.store.OpenMember(r.Context(), string(provider))
	switch {
	case errors.Is(err, messaging.ErrUnauthorized):
	case errors.Is(err, messaging.ErrUnsupported):
		out.State = "unavailable"
	case err != nil:
		writeErr(w, r, err)
		return
	default:
		sender := wireMember(messaging.Member{ID: grant.Member, Name: grant.Name, Active: true})
		out.State = crmcontracts.MessengerIdentityState(MemberConnected)
		out.GrantVersion = &grant.Version
		out.Workspace = &grant.Workspace
		out.Sender = &sender
	}
	httperr.WriteJSON(w, http.StatusOK, out)
}

// DisconnectMessengerIdentity withdraws the caller's personal provider grant.
func (h Handlers) DisconnectMessengerIdentity(w http.ResponseWriter, r *http.Request, provider crmcontracts.MessengerProvider) {
	if !h.messengerReady(w, r) {
		return
	}
	if err := h.store.DisconnectMember(r.Context(), string(provider)); err != nil {
		writeErr(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ListMessengerChannels discovers rooms through the caller's personal grant.
func (h Handlers) ListMessengerChannels(w http.ResponseWriter, r *http.Request, provider crmcontracts.MessengerProvider, kind crmcontracts.ChannelLinkRecordType, id crmcontracts.Id, params crmcontracts.ListMessengerChannelsParams) {
	if !h.messengerReady(w, r) {
		return
	}
	rooms, next, err := h.store.MessengerChannels(r.Context(), string(kind), ids.UUID(id), string(provider), optionalString(params.Cursor))
	if err != nil {
		writeErr(w, r, err)
		return
	}
	out := crmcontracts.MessengerChannels{Conversations: make([]crmcontracts.MessengerConversation, 0, len(rooms)), NextCursor: next}
	for _, room := range rooms {
		out.Conversations = append(out.Conversations, wireConversation(room))
	}
	httperr.WriteJSON(w, http.StatusOK, out)
}

// ListMessengerMembers discovers recipients without borrowing the shared bot's authority.
func (h Handlers) ListMessengerMembers(w http.ResponseWriter, r *http.Request, provider crmcontracts.MessengerProvider, kind crmcontracts.ChannelLinkRecordType, id crmcontracts.Id, params crmcontracts.ListMessengerMembersParams) {
	if !h.messengerReady(w, r) {
		return
	}
	members, next, err := h.store.MessengerMembers(r.Context(), string(kind), ids.UUID(id), string(provider), optionalString(params.Q), optionalString(params.Cursor))
	if err != nil {
		writeErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, crmcontracts.MessengerMembers{Members: wireMembers(members), NextCursor: next})
}

// OpenMessengerConversation opens a reviewed private destination in the record context.
func (h Handlers) OpenMessengerConversation(w http.ResponseWriter, r *http.Request, provider crmcontracts.MessengerProvider, kind crmcontracts.ChannelLinkRecordType, id crmcontracts.Id) {
	if !h.messengerReady(w, r) {
		return
	}
	var in crmcontracts.MessengerOpenRequest
	if !httperr.Decode(w, r, &in) {
		return
	}
	if (in.ConversationId == nil) == (in.Members == nil) {
		writeErr(w, r, apperrors.ErrInvalidArgument)
		return
	}
	var out MessengerContext
	var err error
	if in.Members != nil {
		out, err = h.store.MessengerOpenMembers(r.Context(), string(kind), ids.UUID(id), string(provider), *in.Members)
	} else {
		out, err = h.store.MessengerConversation(r.Context(), string(kind), ids.UUID(id), string(provider), *in.ConversationId)
	}
	if err != nil {
		writeErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, wireMessenger(out))
}

// GetMessengerMessages reads through personal consent rather than the shared bot.
func (h Handlers) GetMessengerMessages(w http.ResponseWriter, r *http.Request, provider crmcontracts.MessengerProvider, kind crmcontracts.ChannelLinkRecordType, id crmcontracts.Id, params crmcontracts.GetMessengerMessagesParams) {
	if !h.messengerReady(w, r) {
		return
	}
	page, err := h.store.MessengerMessages(r.Context(), string(kind), ids.UUID(id), string(provider), params.ConversationId, optionalString(params.RootId), optionalString(params.Cursor))
	if err != nil {
		writeErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, wireMessengerPage(page))
}

// SendMessengerMessage sends through the caller's consent and reviewed native audience.
func (h Handlers) SendMessengerMessage(w http.ResponseWriter, r *http.Request, provider crmcontracts.MessengerProvider, kind crmcontracts.ChannelLinkRecordType, id crmcontracts.Id) {
	if !h.messengerReady(w, r) {
		return
	}
	var in crmcontracts.MessengerSendRequest
	if !httperr.Decode(w, r, &in) {
		return
	}
	input, err := messengerSendInput(in)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	attempt, err := h.store.SendMessenger(r.Context(), string(kind), ids.UUID(id), string(provider), input)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, wireAttempt(attempt))
}

// GetMessengerAttempt reads the caller's durable receipt for the record context.
func (h Handlers) GetMessengerAttempt(w http.ResponseWriter, r *http.Request, provider crmcontracts.MessengerProvider, kind crmcontracts.ChannelLinkRecordType, id crmcontracts.Id, requestID openapi_types.UUID) {
	if !h.messengerReady(w, r) {
		return
	}
	attempt, err := h.store.MessengerAttempt(r.Context(), string(kind), ids.UUID(id), string(provider), ids.UUID(requestID))
	if err != nil {
		writeErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, wireAttempt(attempt))
}

// GetMessengerTeam compares CRM assignment with native identity verification.
func (h Handlers) GetMessengerTeam(w http.ResponseWriter, r *http.Request, provider crmcontracts.MessengerProvider, kind crmcontracts.ChannelLinkRecordType, id crmcontracts.Id) {
	if !h.messengerReady(w, r) {
		return
	}
	people, err := h.store.MessengerTeam(r.Context(), string(kind), ids.UUID(id), string(provider))
	if err != nil {
		writeErr(w, r, err)
		return
	}
	out := make([]crmcontracts.MessengerTeamMember, 0, len(people))
	for _, person := range people {
		out = append(out, crmcontracts.MessengerTeamMember{CrmId: openapi_types.UUID(person.Person.ID), Kind: crmcontracts.MessengerTeamMemberKind(person.Person.Kind), Name: person.Person.Name, MemberId: person.Member, State: crmcontracts.MessengerTeamMemberState(person.State)})
	}
	httperr.WriteJSON(w, http.StatusOK, struct {
		Members []crmcontracts.MessengerTeamMember `json:"members"`
	}{out})
}

// GetMessengerConversation refreshes native audience state before a conversation is used.
func (h Handlers) GetMessengerConversation(w http.ResponseWriter, r *http.Request, provider crmcontracts.MessengerProvider, kind crmcontracts.ChannelLinkRecordType, id crmcontracts.Id, params crmcontracts.GetMessengerConversationParams) {
	if !h.messengerReady(w, r) {
		return
	}
	context, err := h.store.MessengerConversation(r.Context(), string(kind), ids.UUID(id), string(provider), params.ConversationId)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, wireMessenger(context))
}
