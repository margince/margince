// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"net/http"
	"net/url"
	"strings"

	"github.com/margince/margince/backend/internal/compose/chatsync"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/chatlinks"
	"github.com/margince/margince/backend/internal/modules/chatlinks/chatwire"
	"github.com/margince/margince/backend/internal/modules/chatlinks/messaging"
	"github.com/margince/margince/backend/internal/modules/chatlinks/slack"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/authz"
)

func (s Server) slackChannelCallbackURL() string {
	return strings.TrimSuffix(s.messengerOAuthHandlers.callbackURL("slack"), "/messenger/slack/callback") + "/channel-providers/slack/callback"
}

func (s Server) slackChannelApp(ctx context.Context) (slack.OAuth, string, error) {
	h := s.messengerOAuthHandlers
	if !h.cfg.canSignState() || h.now == nil || s.connectorAppHandlers.store == nil {
		return slack.OAuth{}, "", messaging.ErrUnsupported
	}
	app, ok, err := appCredentialsFrom(s.connectorAppHandlers.store, capture.AppProviderSlack)(ctx)
	if err != nil {
		return slack.OAuth{}, "", err
	}
	if !ok || !app.SigningSecretSet || app.SigningSecret == "" {
		return slack.OAuth{}, "", messaging.ErrUnsupported
	}
	return slack.OAuth{HTTP: h.http, ClientID: app.ClientID, ClientSecret: app.ClientSecretRef, Now: h.now}, app.SigningSecret, nil
}

// GetSlackChannelSetup reports shared installation and history-profile prerequisites.
func (s Server) GetSlackChannelSetup(w http.ResponseWriter, r *http.Request) {
	if _, ok := signedInHuman(w, r); !ok {
		return
	}
	if err := auth.Require(r.Context(), "channel_connection", principal.ActionRead); err != nil {
		httperr.Write(w, r, err)
		return
	}
	_, _, err := s.slackChannelApp(r.Context())
	out := crmcontracts.ChannelProviderSetup{
		Configured: err == nil, CallbackUrl: s.slackChannelCallbackURL(),
		EventsUrl: strings.TrimSuffix(s.slackChannelCallbackURL(), "/channel-providers/slack/callback") + "/channel-events/slack", Scopes: slack.BotScopes,
	}
	manifest, manifestErr := slack.AppManifest(s.messengerOAuthHandlers.callbackURL("slack"), out.CallbackUrl, out.EventsUrl)
	if manifestErr != nil {
		httperr.Write(w, r, manifestErr)
		return
	}
	out.AppManifest = &manifest
	if s.historyProfile != nil {
		profile, err := s.historyProfile(r.Context(), "slack")
		if err != nil {
			chatlinks.WriteMessengerFailure(w, r, err)
			return
		}
		out.HistoryProfile = chatlinks.WireHistoryProfile(profile)
	}
	httperr.WriteJSON(w, http.StatusOK, out)
}

// ConnectSlackChannels requests the shared bot installation permissions.
func (s Server) ConnectSlackChannels(w http.ResponseWriter, r *http.Request) {
	actor, ok := signedInHuman(w, r)
	if !ok {
		return
	}
	if !auth.SeatAdmitsWrites(r.Context()) {
		httperr.Write(w, r, apperrors.ErrSeatTierInsufficient)
		return
	}
	if err := auth.Require(r.Context(), "channel_connection", principal.ActionCreate); err != nil {
		httperr.Write(w, r, err)
		return
	}
	app, _, err := s.slackChannelApp(r.Context())
	if err != nil {
		httperr.ServiceUnavailable(w, r, "Set up the Slack app and its signing secret in Messaging providers before installing the workspace bot.")
		return
	}
	var in crmcontracts.ConnectSlackChannelsJSONRequestBody
	if !httperr.Decode(w, r, &in) {
		return
	}
	profile, err := chatwire.SlackHistoryProfile(string(in.Distribution), s.messengerOAuthHandlers.now())
	if err != nil {
		httperr.Write(w, r, apperrors.ErrInvalidArgument)
		return
	}
	selection, err := s.links.MessengerSelection(r.Context())
	if err != nil {
		chatlinks.WriteMessengerFailure(w, r, err)
		return
	}
	if selection.Version > 0 {
		if err := s.links.SetWorkspaceHistoryProfile(r.Context(), selection, string(in.Distribution), s.messengerOAuthHandlers.now()); err != nil {
			chatlinks.WriteMessengerFailure(w, r, err)
			return
		}
	}
	h := s.messengerOAuthHandlers
	state := connectState{User: actor.UserID, Provider: "channel:slack", Version: stateVersionNamespacedCSRF}
	state.SlackHistory = profile
	state.Nonce = setCSRFNonce(w, "channel_slack_csrf", "/v1/channel-providers/slack")
	signed := h.signer().signFor(r.Context(), state, h.now().Add(connectStateTTL))
	httperr.WriteJSON(w, http.StatusOK, struct {
		AuthorizeURL string `json:"authorize_url"`
	}{app.AuthorizeBotURL(signed, s.slackChannelCallbackURL())})
}

// SlackChannelsOAuthCallback stores the bot and channel-reader grants under the signed installation binding.
func (s Server) SlackChannelsOAuthCallback(w http.ResponseWriter, r *http.Request, params crmcontracts.SlackChannelsOAuthCallbackParams) {
	h := s.messengerOAuthHandlers
	if h.now == nil {
		httperr.Write(w, r, apperrors.ErrInvalidArgument)
		return
	}
	state, err := h.signer().verify(params.State, h.now())
	if err != nil || state.Provider != "channel:slack" || !consumeNonceCookie(w, r, "channel_slack_csrf", "/v1/channel-providers/slack", state) {
		httperr.Write(w, r, apperrors.ErrInvalidArgument)
		return
	}
	land := func(result string) {
		http.Redirect(w, r, strings.TrimRight(h.cfg.PublicBaseURL, "/")+"/#/settings/integrations/telegram?"+url.Values{"channel_install": {result}, landingProviderParam: {chatlinks.ProviderSlack}}.Encode(), http.StatusFound)
	}
	if params.Error != nil && *params.Error != "" {
		land("denied")
		return
	}
	if !oauthCodePresent(params.Code) {
		land("failed")
		return
	}
	ctx := grantorContext(r.Context(), state)
	rbac, seat, err := s.liveGrantorAuthority(ctx, state)
	if err != nil {
		land("failed")
		return
	}
	ctx = principal.WithActor(ctx, authz.HumanPrincipal(state.User, rbac, seat))
	if !auth.SeatAdmitsWrites(ctx) || auth.Require(ctx, "channel_connection", principal.ActionCreate) != nil {
		land("failed")
		return
	}
	app, signing, err := s.slackChannelApp(ctx)
	if err != nil {
		land("failed")
		return
	}
	granted, err := app.ExchangeBot(ctx, *params.Code, s.slackChannelCallbackURL())
	if err != nil {
		land("failed")
		return
	}
	if state.SlackHistory == nil {
		land("profile_required")
		return
	}
	if err := chatsync.ConnectSlack(ctx, state.SlackHistory, granted, signing, s.ConnectBot); err != nil {
		land("failed")
		return
	}
	land("connected")
}
