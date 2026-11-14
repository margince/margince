// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package chatlinks

import (
	"context"
	"errors"
	"net/http"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Handlers is the /channel-links transport. A role that composes no store
// leaves it zero, and every operation answers 503.
type Handlers struct {
	store *Store
	filer MessageFiler
	// manifestBase reads public_base_url when the manifest is asked for, so
	// the option that sets it may run after this one.
	manifestBase func() string
	// publisher reads the asking member's address, the manifest's publisher.
	publisher func(ctx context.Context) (string, error)
}

// MessageFiler files a chosen channel message as a summary note. compose
// implements it, because the note crosses into activities and the AI.
type MessageFiler interface {
	FileMessage(ctx context.Context, kind string, id ids.UUID, ref string, thread bool) (crmcontracts.Activity, error)
}

// NewHandlers wires the transport over the store and the message filer.
func NewHandlers(store *Store, filer MessageFiler) Handlers {
	return Handlers{store: store, filer: filer, publisher: store.memberEmail}
}

// FileChannelMessage files a summary of one message, or its thread, as a note.
func (h Handlers) FileChannelMessage(w http.ResponseWriter, r *http.Request, kind crmcontracts.ChannelLinkRecordType, id crmcontracts.Id) {
	if !h.composed(w, r) {
		return
	}
	var req crmcontracts.FileChannelMessageRequest
	if !httperr.Decode(w, r, &req) {
		return
	}
	note, err := h.filer.FileMessage(r.Context(), string(kind), ids.UUID(id), req.Message, req.Thread != nil && *req.Thread)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusCreated, note)
}

// GetChannelLink reads a record's channel.
func (h Handlers) GetChannelLink(w http.ResponseWriter, r *http.Request, kind crmcontracts.ChannelLinkRecordType, id crmcontracts.Id) {
	if !h.composed(w, r) {
		return
	}
	state, err := h.store.GetState(r.Context(), string(kind), ids.UUID(id))
	if err != nil {
		writeErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, wireState(state))
}

// ListLinkableChannels lists the channels the record can be linked to.
func (h Handlers) ListLinkableChannels(w http.ResponseWriter, r *http.Request, kind crmcontracts.ChannelLinkRecordType, id crmcontracts.Id) {
	if !h.composed(w, r) {
		return
	}
	channels, err := h.store.LinkableChannels(r.Context(), string(kind), ids.UUID(id))
	if err != nil {
		writeErr(w, r, err)
		return
	}
	out := crmcontracts.LinkableChannels{Channels: make([]crmcontracts.LinkableChannel, 0, len(channels))}
	for _, ch := range channels {
		out.Channels = append(out.Channels, crmcontracts.LinkableChannel{Slug: ch.Slug, Name: ch.Name})
	}
	httperr.WriteJSON(w, http.StatusOK, out)
}

// LinkChannel links a record to a new or existing channel.
func (h Handlers) LinkChannel(w http.ResponseWriter, r *http.Request, kind crmcontracts.ChannelLinkRecordType, id crmcontracts.Id) {
	if !h.composed(w, r) {
		return
	}
	var req crmcontracts.LinkChannelRequest
	if !httperr.Decode(w, r, &req) {
		return
	}
	link := LinkRequest{Mode: string(req.Mode), SummarizeHistory: req.SummarizeHistory}
	if req.Channel != nil {
		link.Channel = *req.Channel
	}
	if req.Verbosity != nil {
		link.Verbosity = string(*req.Verbosity)
	}
	state, err := h.store.Link(r.Context(), string(kind), ids.UUID(id), link)
	if err != nil {
		writeErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusCreated, wireState(state))
}

// SetChannelVerbosity changes how much is posted into a record's channel.
func (h Handlers) SetChannelVerbosity(w http.ResponseWriter, r *http.Request, kind crmcontracts.ChannelLinkRecordType, id crmcontracts.Id) {
	if !h.composed(w, r) {
		return
	}
	var req crmcontracts.SetChannelVerbosityRequest
	if !httperr.Decode(w, r, &req) {
		return
	}
	state, err := h.store.SetVerbosity(r.Context(), string(kind), ids.UUID(id), string(req.Verbosity))
	if err != nil {
		writeErr(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, wireState(state))
}

// UnlinkChannel stops syncing a record with its channel.
func (h Handlers) UnlinkChannel(w http.ResponseWriter, r *http.Request, kind crmcontracts.ChannelLinkRecordType, id crmcontracts.Id) {
	if !h.composed(w, r) {
		return
	}
	if err := h.store.Unlink(r.Context(), string(kind), ids.UUID(id)); err != nil {
		writeErr(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h Handlers) composed(w http.ResponseWriter, r *http.Request) bool {
	if h.store != nil && h.filer != nil {
		return true
	}
	httperr.Write(w, r, &httperr.DetailedError{
		Status: http.StatusServiceUnavailable,
		Code:   "channel_links_not_configured",
		Detail: "This deployment serves no chat channel links.",
	})
	return false
}

func wireState(s State) crmcontracts.ChannelLinkState {
	out := crmcontracts.ChannelLinkState{State: crmcontracts.ChannelLinkStateState(s.State)}
	if s.Link != nil {
		link := wireLink(*s.Link, s.Address)
		out.Link = &link
	}
	if s.Invites != nil {
		out.Invites = &crmcontracts.ChannelInvites{
			Invited: s.Invites.Invited, NotInChat: s.Invites.NotInChat, NotAdded: s.Invites.NotAdded, SkippedOverCap: s.Invites.SkippedOverCap,
		}
	}
	return out
}

func wireLink(l Link, address string) crmcontracts.ChannelLink {
	return crmcontracts.ChannelLink{
		Id:          openapi_types.UUID(l.ID),
		Provider:    crmcontracts.ChannelLinkProvider(l.Provider),
		ChannelSlug: l.ChannelSlug,
		ChannelName: l.ChannelName,
		ChannelUrl:  ChannelURL(address, l.ChannelSlug),
		Verbosity:   crmcontracts.ChannelVerbosity(l.Verbosity),
		Status:      crmcontracts.ChannelLinkStatus(l.Status),
		DealId:      (*openapi_types.UUID)(l.DealID),
		ProjectId:   (*openapi_types.UUID)(l.ProjectID),
		CreatedAt:   l.CreatedAt,
	}
}

// ChannelURL is the web address of a channel in the chat.
func ChannelURL(address, slug string) string {
	return address + "/channels/" + slug
}

// refusals maps each named refusal to its wire answer.
var refusals = []struct {
	err    error
	status int
	code   string
	detail string
}{
	{ErrBotNotConnected, http.StatusConflict, "channel_bot_not_connected", "No bot connected. Connect one in Settings."},
	{ErrChannelNotFound, http.StatusNotFound, "channel_not_found", "Channel not found. Check the link."},
	{ErrChannelNotPrivate, http.StatusUnprocessableEntity, "channel_not_private", "This channel is public. Pick a private channel."},
	{ErrBotNotMember, http.StatusUnprocessableEntity, "channel_bot_not_member", "The bot is not in this channel. Add it, then try again."},
	{ErrChannelAlreadyLinked, http.StatusConflict, "channel_already_linked", "This channel already belongs to another record."},
	{ErrRecordAlreadyLinked, http.StatusConflict, "record_already_linked", "This record already has a channel. Unlink it first."},
	{ErrLinkNotFound, http.StatusNotFound, "channel_link_not_found", "This record has no channel."},
	{ErrMessageNotFound, http.StatusNotFound, "channel_message_not_found", "That message is not in this record's channel. Check the link."},
	{ErrThreadTooLong, http.StatusUnprocessableEntity, "channel_thread_too_long", "This thread is too long to file as one note. File single messages instead."},
	{ErrNoSummaryModel, http.StatusConflict, "ai_not_configured", "Summarising chat talk needs an AI model. An administrator can bind one under Settings → AI."},
	{ErrSummaryFailed, http.StatusBadGateway, "channel_summary_failed", "The summary could not be written. Nothing was filed. Try again."},
	{ErrChatUnreachable, http.StatusBadGateway, "channel_provider_unreachable", "The chat could not be reached. Nothing was changed. Try again."},
}

func writeErr(w http.ResponseWriter, r *http.Request, err error) {
	for _, refusal := range refusals {
		if errors.Is(err, refusal.err) {
			httperr.Write(w, r, &httperr.DetailedError{Status: refusal.status, Code: refusal.code, Detail: refusal.detail})
			return
		}
	}
	httperr.Write(w, r, err)
}
