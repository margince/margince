// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package chatlinks

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/chatlinks/messaging"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// Handlers is the /channel-links transport. A role that composes no store
// leaves it zero, and every operation answers 503.
type Handlers struct {
	preview DeliveryPreviewer
	store   *Store
	filer   MessageFiler
	// manifestBase reads public_base_url when the manifest is asked for, so
	// the option that sets it may run after this one.
	manifestBase func() string
	// publisher reads the asking member's address, the manifest's publisher.
	publisher func(ctx context.Context) (string, error)
}

// DeliveryPreviewer shares the outbound preview engine with the HTTP surface.
type DeliveryPreviewer interface {
	PreviewDelivery(context.Context, string, ids.UUID) (crmcontracts.ChannelDeliveryPreview, error)
}

// WithDeliveryPreviewer shares delivery preparation with the HTTP preview surface.
func (h Handlers) WithDeliveryPreviewer(preview DeliveryPreviewer) Handlers {
	h.preview = preview
	return h
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
	out := wireState(state)
	if out.Link != nil && h.store.historyProfiles != nil {
		profile, err := h.store.historyProfiles(r.Context(), string(out.Link.Provider))
		if err != nil {
			writeErr(w, r, err)
			return
		}
		out.Link.HistoryProfile = WireHistoryProfile(profile)
	}
	httperr.WriteJSON(w, http.StatusOK, out)
}

// ListLinkableChannels lists the channels the record can be linked to.
func (h Handlers) ListLinkableChannels(w http.ResponseWriter, r *http.Request, kind crmcontracts.ChannelLinkRecordType, id crmcontracts.Id, params crmcontracts.ListLinkableChannelsParams) {
	if !h.composed(w, r) {
		return
	}
	provider := ProviderDispact
	if params.Provider != nil {
		provider = string(*params.Provider)
	}
	channels, err := h.store.LinkableProviderChannels(r.Context(), string(kind), ids.UUID(id), provider)
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
	if req.Provider != nil {
		link.Provider = string(*req.Provider)
	}
	if req.RootId != nil {
		link.RootID = *req.RootId
	}
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
	if err := httperr.RequireAnyField(r); err != nil {
		httperr.Write(w, r, err)
		return
	}
	state, err := h.store.SetSyncOptions(r.Context(), string(kind), ids.UUID(id), SyncOptions{ExpectedLink: (*ids.UUID)(req.ExpectedLinkId), ExpectedVersion: req.ExpectedVersion, Verbosity: (*string)(req.Verbosity), Read: req.SaveChannelUpdates, Post: req.SendCrmUpdates, MessageDetail: (*string)(req.MessageDetail)})
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
	if ended := s.LastSync; ended != nil {
		out.LastSync = &crmcontracts.ChannelEndedSync{Provider: crmcontracts.MessengerProvider(ended.Provider), EndedAt: ended.Ended, ReadThrough: ended.ReadThrough, LastPostedAt: ended.LastPosted, RetainedMessages: ended.RetainedMessages, ConfirmedRedactions: ended.ConfirmedRedactions, UnverifiedWithdrawals: ended.UnverifiedWithdrawals, UnconfirmedDeliveries: ended.UnconfirmedDeliveries}
	}
	if s.Health != nil {
		out.Sync = &crmcontracts.ChannelSyncHealth{NotificationsDelayed: &s.Health.NotificationsDelayed, CoverageAt: s.Health.Coverage, LastError: (*crmcontracts.ChannelSyncHealthLastError)(s.Health.Error), PendingConversations: s.Health.PendingRoots, FailedConversations: s.Health.FailedRoots, PendingCrmUpdates: s.Health.PendingUpdates, UnconfirmedDeliveries: s.Health.Unconfirmed, DeliveryError: (*crmcontracts.ChannelSyncHealthDeliveryError)(s.Health.DeliveryError)}
	}
	if s.Link != nil {
		link := wireLink(*s.Link, s.Address, s.Workspace)
		out.Link = &link
	}
	if s.Invites != nil {
		out.Invites = &crmcontracts.ChannelInvites{
			Invited: s.Invites.Invited, NotInChat: s.Invites.NotInChat, NotAdded: s.Invites.NotAdded, SkippedOverCap: s.Invites.SkippedOverCap,
		}
	}
	return out
}

func wireLink(l Link, address, workspace string) crmcontracts.ChannelLink {
	detail := crmcontracts.ChannelMessageDetail(l.MessageDetail)
	sourceURL := ProviderChannelURL(l.Provider, workspace, address, l.ChannelSlug)
	if l.Provider == ProviderDispact && !l.NativeIdentity {
		sourceURL = ChannelURL(address, l.ChannelSlug)
	}
	return crmcontracts.ChannelLink{
		NativeIdentity:     new(l.NativeIdentity),
		OverviewCardPaused: &l.CardPaused,
		Id:                 openapi_types.UUID(l.ID),
		Provider:           crmcontracts.MessengerProvider(l.Provider),
		ChannelSlug:        l.ChannelSlug,
		ChannelName:        l.ChannelName,
		ChannelUrl:         sourceURL,
		RootId:             optionalRoot(l.RootID),
		CanSendCrmUpdates:  new(l.PostSupported),
		Verbosity:          crmcontracts.ChannelVerbosity(l.Verbosity),
		MessageDetail:      &detail,
		Status:             crmcontracts.ChannelLinkStatus(l.Status),
		CompanyId:          (*openapi_types.UUID)(l.CompanyID),
		SaveChannelUpdates: new(!l.ReadDisabled), SendCrmUpdates: new(!l.PostDisabled),
		DealId:    (*openapi_types.UUID)(l.DealID),
		ProjectId: (*openapi_types.UUID)(l.ProjectID),
		CreatedAt: l.CreatedAt,
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
	{ErrChannelIdentityNeedsReview, http.StatusConflict, "channel_identity_needs_review", "Choose the channel again to confirm its current identity before syncing."},
	{ErrMessengerDeliveryNeedsReview, http.StatusConflict, "messenger_delivery_needs_review", "An earlier delivery in this conversation is still unresolved. Refresh Messenger, check the conversation, and explicitly review the earlier attempt before sending again."},
	{ErrMembersConsentRequired, http.StatusConflict, "channel_members_consent_required", "Reconnect the shared Dispact app and approve channel:members:read before enabling automatic CRM posts. Incoming channel recaps can continue."},
	{ErrAudienceNeedsReview, http.StatusConflict, "channel_audience_needs_review", "Automatic sharing is paused because the complete channel audience could not be cleared for this record. Review the members and their record access, then review channel delivery again."},
	{ErrBotNotConnected, http.StatusConflict, "channel_bot_not_connected", "No bot connected. Connect one in Settings."},
	{ErrChannelNotFound, http.StatusNotFound, "channel_not_found", "Channel not found. Check the link."},
	{ErrChannelNameRefused, http.StatusUnprocessableEntity, "channel_name_refused", "The chat refused this channel name. Rename the record, or link an existing channel."},
	{ErrChannelNotPrivate, http.StatusUnprocessableEntity, "channel_not_private", "This channel is public. Pick a private channel."},
	{ErrBotNotMember, http.StatusUnprocessableEntity, "channel_bot_not_member", "The bot is not in this channel. Add it, then try again."},
	{ErrChannelAlreadyLinked, http.StatusConflict, "channel_already_linked", "This channel already belongs to another record."},
	{ErrRecordAlreadyLinked, http.StatusConflict, "record_already_linked", "This record already has a channel. Unlink it first."},
	{ErrLinkNotFound, http.StatusNotFound, "channel_link_not_found", "This record has no channel."},
	{ErrMessageNotFound, http.StatusNotFound, "channel_message_not_found", "That message is not in this record's channel. Check the link."},
	{ErrThreadTooLong, http.StatusUnprocessableEntity, "channel_thread_too_long", "This thread is too long to file as one note. File single messages instead."},
	{ErrNoSummaryModel, http.StatusConflict, httperr.CodeAINotConfigured, "Summarising chat talk needs an AI model. An administrator can bind one under Settings → AI."},
	{ErrSummaryFailed, http.StatusBadGateway, "channel_summary_failed", "The summary could not be written. Nothing was filed. Try again."},
	{ErrChatUnreachable, http.StatusBadGateway, "channel_provider_unreachable", "The chat could not be reached. Nothing was changed. Try again."},
	{messaging.ErrUnauthorized, http.StatusConflict, "messenger_consent_required", "Reconnect your messaging account to continue."},
	{messaging.ErrUnsupported, http.StatusServiceUnavailable, "messenger_unavailable", "This messaging operation is unavailable. Check the provider connection and supported capabilities."},
	{messaging.ErrUnavailable, http.StatusBadGateway, "messenger_conversation_unavailable", "This conversation is unavailable. Check access in your messaging app and refresh."},
	{messaging.ErrInvalid, http.StatusConflict, "messenger_review_required", "Review the destination, audience and message before sending."},
	{messaging.ErrDeliveryUnknown, http.StatusConflict, "messenger_delivery_unknown", "Check the conversation before sending again. Delivery could not be confirmed."},
}

func writeErr(w http.ResponseWriter, r *http.Request, err error) {
	if limited, ok := errors.AsType[*connector.RateLimitedError](err); ok {
		seconds := max(1, int(limited.RetryAfter.Seconds()))
		w.Header().Set("Retry-After", strconv.Itoa(seconds))
		httperr.Write(w, r, &httperr.DetailedError{Status: http.StatusTooManyRequests, Code: "messenger_rate_limited", Detail: "The provider asked us to wait. Your draft is preserved."})
		return
	}

	for _, refusal := range refusals {
		if errors.Is(err, refusal.err) {
			httperr.Write(w, r, &httperr.DetailedError{Status: refusal.status, Code: refusal.code, Detail: refusal.detail})
			return
		}
	}
	httperr.Write(w, r, err)
}

func optionalRoot(root string) *string {
	if root == "" {
		return nil
	}
	return &root
}
