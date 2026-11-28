// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/compose/chatsync"
	"github.com/margince/margince/backend/internal/compose/messengeraccounts"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/chatlinks"
	"github.com/margince/margince/backend/internal/modules/chatlinks/dispact"
	"github.com/margince/margince/backend/internal/modules/chatlinks/messaging"
	"github.com/margince/margince/backend/internal/modules/chatlinks/slack"
	"github.com/margince/margince/backend/internal/modules/chatlinks/teams"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/recordpage"
	"github.com/margince/margince/backend/internal/shared/ports/authz"
)

type messengerOAuthHandlers struct {
	store          *chatlinks.Store
	cfg            GraphConfig
	http           *http.Client
	now            func() time.Time
	resolve        func(context.Context, string) (messengeraccounts.App, error)
	dispact        func(context.Context) (capture.LiveBot, error)
	installed      func(context.Context, string) (capture.LiveBot, error)
	historyProfile chatlinks.HistoryProfileReader
}

// WithMessengerOAuth composes personal consent independently of channel installation.
func WithMessengerOAuth(cfg GraphConfig, private bool) Option {
	return func(s *Server, _ *pgxpool.Pool) {
		h := &s.messengerOAuthHandlers
		h.cfg, h.http, h.now = cfg, chatsync.HTTPClient(private), time.Now
		h.resolve = s.messengerApp
		for provider, vendor := range map[string]capture.AppProvider{chatlinks.ProviderSlack: capture.AppProviderSlack, chatlinks.ProviderTeams: capture.AppProviderMicrosoft} {
			s.addRedirectURI(vendor, "messenger_connect", h.callbackURL(provider))
			if provider == chatlinks.ProviderSlack {
				s.addRedirectURI(vendor, "channel_connect", s.slackChannelCallbackURL())
			}
		}
	}
}

func (h messengerOAuthHandlers) callbackURL(provider string) string {
	base := h.cfg.APIBaseURL
	if base == "" {
		base = h.cfg.PublicBaseURL
	}
	return strings.TrimRight(base, "/") + "/v1/messenger/" + provider + "/callback"
}

func (s *Server) messengerApp(ctx context.Context, provider string) (messengeraccounts.App, error) {
	h := s.messengerOAuthHandlers
	if !h.cfg.canSignState() || h.store == nil {
		return messengeraccounts.App{}, messaging.ErrUnsupported
	}
	app, err := s.messagingAppCredentials(ctx, provider)
	if err != nil {
		return messengeraccounts.App{}, err
	}
	if provider == chatlinks.ProviderSlack {
		return messengeraccounts.SlackInstallationApp(ctx, h.http, app, h.now, h.installed)
	}
	return messengeraccounts.TeamsApp(h.http, app, h.now)
}

type messengerReturnRecord struct {
	Kind string
	ID   ids.UUID
}

func (h messengerOAuthHandlers) messengerLanding(st connectState, outcome, provider string) string {
	base := strings.TrimRight(h.cfg.PublicBaseURL, "/")
	landing := base + "/#/settings/integrations/telegram"
	if st.MessengerRecord != nil {
		if page := recordpage.URL(base, st.MessengerRecord.Kind, st.MessengerRecord.ID); page != nil {
			landing = *page
		}
	}
	return landing + "?" + url.Values{"messenger": {outcome}, landingProviderParam: {provider}}.Encode()
}

// ConnectMessenger starts personal consent independently of shared channel installation.
func (s Server) ConnectMessenger(w http.ResponseWriter, r *http.Request, provider crmcontracts.MessengerProvider) {
	actor, ws, ok := signedInHuman(w, r)
	if !ok {
		return
	}
	if !auth.SeatAdmitsWrites(r.Context()) {
		httperr.Write(w, r, apperrors.ErrSeatTierInsufficient)
		return
	}
	h := s.messengerOAuthHandlers
	if h.resolve == nil {
		httperr.ServiceUnavailable(w, r, "Messaging consent is unavailable. Ask a workspace administrator to complete setup.")
		return
	}
	app, err := h.resolve(r.Context(), string(provider))
	if err != nil {
		httperr.ServiceUnavailable(w, r, "Set up this messaging provider's OAuth app in Settings before connecting your account.")
		return
	}
	st := connectState{Workspace: ws, User: actor.UserID, Provider: "messenger:" + string(provider), Version: stateVersionNamespacedCSRF}
	if r.ContentLength != 0 {
		var in crmcontracts.MessengerConnectRequest
		if !httperr.Decode(w, r, &in) {
			return
		}

		st.MessengerRecord, err = messengerReturnOf(in)
		if err != nil {
			httperr.Write(w, r, err)
			return
		}

	}
	st.MessagingWorkspace = app.Workspace
	st.Nonce = setCSRFNonce(w, "messenger_csrf_"+string(provider), "/v1/messenger")
	state := newStateSigner([]byte(h.cfg.StateKey)).sign(st, h.now().Add(connectStateTTL))
	target, err := url.Parse(app.Authorize(state, h.callbackURL(string(provider))))
	if err != nil {
		httperr.Write(w, r, messaging.ErrUnavailable)
		return
	}
	if provider == chatlinks.ProviderSlack && app.Workspace != "" {
		query := target.Query()
		query.Set("team", app.Workspace)
		target.RawQuery = query.Encode()
	}
	httperr.WriteJSON(w, http.StatusOK, struct {
		AuthorizeURL string `json:"authorize_url"`
	}{target.String()})
}

// MessengerOAuthCallback checks the signed consent binding before storing the personal grant.
func (s Server) MessengerOAuthCallback(w http.ResponseWriter, r *http.Request, provider crmcontracts.MessengerProvider, params crmcontracts.MessengerOAuthCallbackParams) {
	h := s.messengerOAuthHandlers
	if h.now == nil {
		httperr.Write(w, r, apperrors.ErrInvalidArgument)
		return
	}
	st, err := newStateSigner([]byte(h.cfg.StateKey)).verify(params.State, h.now())
	if err != nil || st.Provider != "messenger:"+string(provider) || !consumeNonceCookie(w, r, "messenger_csrf_"+string(provider), "/v1/messenger", st) {
		httperr.Write(w, r, apperrors.ErrInvalidArgument)
		return
	}
	land := func(outcome string) {
		http.Redirect(w, r, h.messengerLanding(st, outcome, string(provider)), http.StatusFound)
	}
	if params.Error != nil && *params.Error != "" {
		land("denied")
		return
	}
	if !oauthCodePresent(params.Code) {
		land("failed")
		return
	}
	ctx := grantorContext(r.Context(), st)
	rbac, seat, err := s.liveGrantorAuthority(ctx, st)
	if err != nil {
		land("failed")
		return
	}
	ctx = principal.WithActor(ctx, authz.HumanPrincipal(st.User, rbac, seat))
	app, err := h.resolve(ctx, string(provider))
	if err != nil {
		land("failed")
		return
	}
	grant, credential, err := app.Exchange(ctx, *params.Code, h.callbackURL(string(provider)))
	if err != nil {
		land("failed")
		return
	}
	if !messengeraccounts.WorkspaceMatches(st.MessagingWorkspace, app.Workspace, grant.Workspace) {
		land("failed")
		return
	}
	raw, err := json.Marshal(credential) // #nosec G117 -- Encoded only for encrypted credential-vault storage.
	if err != nil {
		land("failed")
		return
	}
	if _, err := h.store.ConnectMember(ctx, grant, raw); err != nil {
		land("failed")
		return
	}
	land("connected")
}

// GetMessengerSetup reports the consent prerequisites for a personal connection.
func (s Server) GetMessengerSetup(w http.ResponseWriter, r *http.Request, provider crmcontracts.MessengerProvider) {
	if _, _, ok := signedInHuman(w, r); !ok {
		return
	}
	h := s.messengerOAuthHandlers
	out := crmcontracts.MessengerSetup{Provider: provider, ParticipantScopes: []string{}}
	switch provider {
	case chatlinks.ProviderDispact:
		out.ParticipantScopes = []string{string(scopeRead), string(scopeWrite)}
		out.MaxGroupRecipients = dispact.MaxGroupRecipients
		if h.dispact != nil {
			installation, err := h.dispact(r.Context())
			if err != nil && !errors.Is(err, capture.ErrNoConnection) {
				writeConnectorAppFailure(w, r, err)
				return
			}
			out.Configured = err == nil
			if err == nil {
				account := strings.TrimRight(installation.Address, "/")
				out.AccountUrl = &account
			}
		}
		httperr.WriteJSON(w, http.StatusOK, out)
		return
	case chatlinks.ProviderSlack:
		out.ParticipantScopes = append(out.ParticipantScopes, slack.UserScopes...)
		out.MaxGroupRecipients = slack.MaxGroupRecipients
	case chatlinks.ProviderTeams:
		out.ParticipantScopes = append(out.ParticipantScopes, messengeraccounts.TeamsScopes...)
		out.MaxGroupRecipients = teams.MaxGroupRecipients
	}
	if h.cfg.canSignState() {
		out.CallbackUrl = h.callbackURL(string(provider))
	}
	if h.resolve != nil {
		_, err := h.resolve(r.Context(), string(provider))
		if err != nil && !errors.Is(err, messaging.ErrUnsupported) {
			writeConnectorAppFailure(w, r, err)
			return
		}
		out.Configured = err == nil
	}
	httperr.WriteJSON(w, http.StatusOK, out)
}

func (s *Server) messagingAppCredentials(ctx context.Context, provider string) (capture.ConnectorApp, error) {
	return resolveMessagingApp(ctx, provider, s.cfg, s.connectorAppHandlers.store)
}

func resolveMessagingApp(ctx context.Context, provider string, cfg GraphConfig, store *capture.ConnectorAppStore) (capture.ConnectorApp, error) {
	vendor := capture.AppProviderSlack
	if provider == chatlinks.ProviderTeams {
		vendor = capture.AppProviderMicrosoft
	} else if provider != chatlinks.ProviderSlack {
		return capture.ConnectorApp{}, messaging.ErrUnsupported
	}
	var app capture.ConnectorApp
	if provider == chatlinks.ProviderTeams && cfg.canSync() {
		app = capture.ConnectorApp{ClientID: cfg.ClientID, ClientSecretRef: cfg.ClientSecret, Tenant: cfg.Tenant}
	} else {
		if store == nil {
			return capture.ConnectorApp{}, messaging.ErrUnsupported
		}
		var ok bool
		var err error
		app, ok, err = appCredentialsFrom(store, vendor)(ctx)
		if err != nil {
			return capture.ConnectorApp{}, err
		}
		if !ok {
			return capture.ConnectorApp{}, messaging.ErrUnsupported
		}
	}
	return app, nil
}

func messengerReturnOf(in crmcontracts.MessengerConnectRequest) (*messengerReturnRecord, error) {
	id := ids.UUID(in.RecordId)
	if err := httperr.RequireBodyID("record_id", id); err != nil {
		return nil, err
	}
	switch in.RecordType {
	case crmcontracts.ChannelLinkRecordType(recordTypeCompany), crmcontracts.ChannelLinkRecordType(recordTypeDeal), crmcontracts.ChannelLinkRecordType(recordTypeProject):
	default:
		return nil, apperrors.ErrInvalidArgument
	}
	return &messengerReturnRecord{Kind: string(in.RecordType), ID: id}, nil
}

func oauthCodePresent(code *string) bool { return code != nil && *code != "" }
