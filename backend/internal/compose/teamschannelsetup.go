// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"slices"
	"strings"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/margince/margince/backend/internal/compose/teamsapp"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/chatlinks"
	"github.com/margince/margince/backend/internal/modules/chatlinks/chatwire"
	"github.com/margince/margince/backend/internal/modules/chatlinks/messaging"
	"github.com/margince/margince/backend/internal/modules/chatlinks/teams"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func (s Server) channelAdmin(ctx context.Context, action principal.Action) (context.Context, error) {
	if s.chatSourceHandlers.pool == nil || s.links == nil {
		return nil, messaging.ErrUnsupported
	}
	ctx, err := s.liveReader(ctx)
	if err != nil {
		return nil, err
	}
	if err = auth.RequireHuman(ctx); err != nil {
		return nil, err
	}
	if action != principal.ActionRead && !auth.SeatAdmitsWrites(ctx) {
		return nil, apperrors.ErrSeatTierInsufficient
	}
	switch action {
	case principal.ActionRead:
		err = auth.Require(ctx, "channel_connection", principal.ActionRead)
	case principal.ActionCreate:
		err = auth.Require(ctx, "channel_connection", principal.ActionCreate)
	default:
		return nil, apperrors.ErrInvalidArgument
	}
	if err != nil {
		return nil, err
	}
	return ctx, nil
}

func (s Server) teamsChannelApp(ctx context.Context) (capture.ConnectorApp, error) {
	app, err := s.messagingAppCredentials(ctx, "teams")
	if err != nil {
		return app, err
	}
	for _, id := range []string{app.ClientID, app.Tenant} {
		if _, err := ids.Parse(id); err != nil {
			return capture.ConnectorApp{}, messaging.ErrUnsupported
		}
	}
	return app, nil
}

// GetTeamsChannelSetup lists verified installations eligible for channel association.
func (s Server) GetTeamsChannelSetup(w http.ResponseWriter, r *http.Request) {
	ctx, err := s.channelAdmin(r.Context(), principal.ActionRead)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	out := crmcontracts.TeamsChannelSetup{EventsUrl: strings.TrimSuffix(s.messengerOAuthHandlers.callbackURL("teams"), "/messenger/teams/callback") + "/channel-events/teams/bot", Scopes: teams.BotScopes}
	out.Installations = make([]crmcontracts.TeamsChannelInstallation, 0)
	app, err := s.teamsChannelApp(ctx)
	if errors.Is(err, messaging.ErrUnsupported) {
		httperr.WriteJSON(w, http.StatusOK, out)
		return
	}
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	out.Configured, out.AppId, out.TenantId = true, app.ClientID, app.Tenant
	if !auth.SeatAdmitsWrites(ctx) || auth.Require(ctx, "channel_connection", principal.ActionCreate) != nil {
		httperr.WriteJSON(w, http.StatusOK, out)
		return
	}
	installations, err := s.links.TeamInstallations(ctx, app.ClientID, app.Tenant)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	for _, item := range installations {
		out.Installations = append(out.Installations, crmcontracts.TeamsChannelInstallation{Id: openapi_types.UUID(item.ID), Name: item.Name, TeamId: item.Team})
	}
	httperr.WriteJSON(w, http.StatusOK, out)
}

// ConnectTeamsChannels binds a verified Teams installation to channel synchronization.
func (s Server) ConnectTeamsChannels(w http.ResponseWriter, r *http.Request) {
	ctx, err := s.channelAdmin(r.Context(), principal.ActionCreate)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	var in crmcontracts.ConnectTeamsChannelsJSONRequestBody
	if !httperr.Decode(w, r, &in) {
		return
	}
	installationID, err := chatlinks.TeamInstallationID(in)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	app, err := s.teamsChannelApp(ctx)
	if err != nil {
		chatlinks.WriteMessengerFailure(w, r, err)
		return
	}
	installations, err := s.links.TeamInstallations(ctx, app.ClientID, app.Tenant)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	for _, installation := range installations {
		if installation.ID != installationID {
			continue
		}
		credential := chatwire.BotCredential{Workspace: installation.Tenant, Member: installation.Team, NativeTeam: installation.NativeTeam, BotMember: installation.BotMember, ServiceURL: installation.ServiceURL, Credential: messaging.Credential{AppID: app.ClientID}, AllowedTeams: []string{installation.Team}}
		registry := capture.NewRegistry(InstallationDB(s.chatSourceHandlers.pool), nil, nil, s.vault)
		current, err := registry.LiveBot(ctx, chatlinks.ProviderTeams)
		if err == nil {
			var previous chatwire.BotCredential
			if json.Unmarshal([]byte(current.Token), &previous) != nil || previous.Workspace != app.Tenant || previous.Credential.AppID != app.ClientID {
				httperr.Write(w, r, apperrors.ErrConflict)
				return
			}
			credential.AllowedTeams = slices.Clone(previous.AllowedTeams)
			for _, team := range []string{previous.Member, installation.Team} {
				if !slices.Contains(credential.AllowedTeams, team) {
					credential.AllowedTeams = append(credential.AllowedTeams, team)
				}
			}
		} else if !errors.Is(err, capture.ErrNoConnection) {
			httperr.Write(w, r, err)
			return
		}
		if len(credential.AllowedTeams) > 100 {
			httperr.Write(w, r, apperrors.ErrInvalidArgument)
			return
		}
		raw, err := json.Marshal(credential)
		if err != nil {
			httperr.Write(w, r, err)
			return
		}
		if _, err = s.ConnectBot(ctx, capture.ConnectRequest{Provider: chatlinks.ProviderTeams, BotToken: string(raw)}); err != nil {
			httperr.Write(w, r, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}
	httperr.Write(w, r, apperrors.ErrConflict)
}

// DownloadTeamsChannelPackage packages the verified application's installation metadata.
func (s Server) DownloadTeamsChannelPackage(w http.ResponseWriter, r *http.Request) {
	ctx, err := s.channelAdmin(r.Context(), principal.ActionCreate)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	var in crmcontracts.DownloadTeamsChannelPackageJSONRequestBody
	if !httperr.Decode(w, r, &in) {
		return
	}
	app, err := s.teamsChannelApp(ctx)
	if err != nil {
		chatlinks.WriteMessengerFailure(w, r, err)
		return
	}
	archive, err := teamsapp.Archive(teamsapp.Config{AppID: app.ClientID, Origin: strings.TrimRight(s.cfg.PublicBaseURL, "/"), PrivacyURL: in.PrivacyUrl, TermsURL: in.TermsUrl})
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	(httperr.Download{ContentType: "application/zip", Filename: "margince-teams.zip", Size: int64(len(archive))}).WriteHeaders(w)
	if _, err = w.Write(archive); err != nil {
		s.log.ErrorContext(ctx, "write Teams app package", "error", err)
	}
}
