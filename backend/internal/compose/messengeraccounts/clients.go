// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package messengeraccounts

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/margince/margince/backend/internal/compose/chatsync"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/chatlinks"
	"github.com/margince/margince/backend/internal/modules/chatlinks/dispact"
	"github.com/margince/margince/backend/internal/modules/chatlinks/messaging"
	"github.com/margince/margince/backend/internal/modules/chatlinks/slack"
	"github.com/margince/margince/backend/internal/modules/chatlinks/teams"
)

// Clients binds personal clients to the installation that issued their grants.
func Clients(private bool, installation func(context.Context, string) (capture.LiveBot, error), profiles ...chatlinks.HistoryProfileReader) chatlinks.MemberClient {
	httpClient := chatsync.HTTPClient(private)
	return func(ctx context.Context, grant chatlinks.MemberGrant, raw []byte) (messaging.Client, error) {
		var credential messaging.Credential
		if json.Unmarshal(raw, &credential) != nil || credential.AccessToken == "" {
			return nil, messaging.ErrUnauthorized
		}
		switch grant.Provider {
		case "dispact":
			client, err := dispactClient(ctx, httpClient, installation, grant, credential)
			if err != nil {
				return nil, err
			}
			return client, nil
		case "slack":
			client, err := slackClient(ctx, httpClient, installation, grant, credential)
			if err != nil {
				return nil, err
			}
			if len(profiles) > 0 && profiles[0] != nil {
				profile, err := profiles[0](ctx, grant.Provider)
				if err != nil {
					return nil, err
				}
				client.WithParticipantHistory(profile, credential.AppID)
			} else {
				client.WithParticipantHistory(nil, credential.AppID)
			}
			return client, nil
		case chatlinks.ProviderTeams:
			return teams.New(httpClient, credential.AccessToken, grant.Workspace, grant.Member), nil
		default:
			return nil, messaging.ErrUnsupported
		}
	}
}

func dispactClient(ctx context.Context, httpClient messaging.Doer, installation func(context.Context, string) (capture.LiveBot, error), grant chatlinks.MemberGrant, credential messaging.Credential) (*dispact.Participant, error) {
	if installation == nil {
		return nil, messaging.ErrUnsupported
	}
	live, err := installation(ctx, grant.Provider)
	if err != nil {
		return nil, err
	}
	if live.ConnectionID.String() != credential.InstallationID || live.TeamID != grant.Workspace {
		return nil, messaging.ErrUnauthorized
	}
	return dispact.NewParticipant(httpClient, live.Address, credential.AccessToken, grant.Workspace, grant.Member), nil
}

func slackClient(ctx context.Context, httpClient messaging.Doer, installation func(context.Context, string) (capture.LiveBot, error), grant chatlinks.MemberGrant, credential messaging.Credential) (*slack.Client, error) {
	var live capture.LiveBot
	err := capture.ErrNoConnection
	if installation != nil {
		live, err = installation(ctx, grant.Provider)
	}
	if err != nil && !errors.Is(err, capture.ErrNoConnection) {
		return nil, err
	}
	if err == nil && live.TeamID != grant.Workspace {
		return nil, messaging.ErrUnauthorized
	}
	return slack.New(httpClient, credential.AccessToken, grant.Workspace, grant.Member), nil
}
