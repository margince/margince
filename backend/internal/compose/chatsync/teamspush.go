// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package chatsync

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/margince/margince/backend/internal/compose/oidctoken"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/chatlinks"
	"github.com/margince/margince/backend/internal/modules/chatlinks/chatwire"
	"github.com/margince/margince/backend/internal/modules/chatlinks/messaging"
	"github.com/margince/margince/backend/internal/modules/chatlinks/teams"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// TeamsVerifier checks the application and service identity of incoming activities.
type TeamsVerifier interface {
	VerifyAs(context.Context, string, func(oidctoken.Claims) error) (oidctoken.Claims, error)
}

// TeamsReceiver accepts verified Bot Framework activities as source-change cues.
type TeamsReceiver struct {
	*Receiver
	Store    *chatlinks.Store
	Binding  func(context.Context) (capture.LiveBot, error)
	App      BotApp
	HTTP     messaging.Doer
	Verifier TeamsVerifier
}

//nolint:tagliatelle // Microsoft's provider protocol fixes these JSON field names.
type teamsActivity struct {
	ID         string    `json:"id"`
	Type       string    `json:"type"`
	Action     string    `json:"action"`
	At         time.Time `json:"timestamp"`
	ServiceURL string    `json:"serviceUrl"`
	Channel    string    `json:"channelId"`
	ReplyTo    string    `json:"replyToId"`
	Recipient  struct {
		ID string `json:"id"`
	} `json:"recipient"`
	Conversation struct {
		ID     string `json:"id"`
		Tenant string `json:"tenantId"`
	} `json:"conversation"`
	Added []struct {
		ID string `json:"id"`
	} `json:"membersAdded"`
	Removed []struct {
		ID string `json:"id"`
	} `json:"membersRemoved"`
	Data struct {
		Tenant struct {
			ID string `json:"id"`
		} `json:"tenant"`
		Team struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"team"`
		Channel struct {
			ID string `json:"id"`
		} `json:"channel"`
		ReplyChain struct {
			ID string `json:"id"`
		} `json:"replyChain"`
	} `json:"channelData"`
}

// Receive validates provider notifications before queueing source synchronization.
func (r *TeamsReceiver) Receive(ctx context.Context, d chatlinks.Delivery) (crmcontracts.DispactChannelEventReceipt, error) {
	ctx = principal.WithActor(ctx, chatLinksPrincipal)
	var activity teamsActivity
	if json.Unmarshal(d.Body, &activity) != nil || activity.ID == "" || activity.At.IsZero() || activity.At.After(r.Now().Add(noticeSkew)) {
		return crmcontracts.DispactChannelEventReceipt{}, chatlinks.ErrEventMalformed
	}
	app, err := r.App(ctx, chatlinks.ProviderTeams)
	if err != nil {
		return crmcontracts.DispactChannelEventReceipt{}, chatlinks.ErrEventsUnavailable
	}
	if err := r.authorize(ctx, app, activity, d.Authorization); err != nil {
		return crmcontracts.DispactChannelEventReceipt{}, err
	}
	if !handledTeamsActivity(activity.Type) {
		return receipt(crmcontracts.DispactChannelEventReceiptStatusIgnored), nil
	}
	if activity.Data.Team.ID == "" {
		return receipt(crmcontracts.DispactChannelEventReceiptStatusIgnored), nil
	}
	digest := sha256.Sum256(d.Body)
	revision := hex.EncodeToString(digest[:])
	key := "teams-activity|" + app.ClientID + "|" + revision
	if err := r.spend(ctx, key); errors.Is(err, chatlinks.ErrEventReplayed) {
		return receipt(crmcontracts.DispactChannelEventReceiptStatusAlreadyTaken), nil
	} else if err != nil {
		return crmcontracts.DispactChannelEventReceipt{}, err
	}
	result, err := r.actTeams(ctx, app, activity, revision)
	if err != nil {
		r.release(ctx, key)
	}
	return result, err
}

func (r *TeamsReceiver) authorize(ctx context.Context, app capture.ConnectorApp, activity teamsActivity, authorization string) error {
	if r.Verifier == nil || activity.Channel != "msteams" || activity.Data.Tenant.ID != app.Tenant || activity.Conversation.Tenant != "" && activity.Conversation.Tenant != app.Tenant || activity.Recipient.ID != "28:"+app.ClientID {
		return chatlinks.ErrEventRejected
	}
	if err := teams.CheckServiceURL(activity.ServiceURL); err != nil {
		return chatlinks.ErrEventRejected
	}
	bearer, ok := strings.CutPrefix(authorization, "Bearer ")
	if !ok || bearer == "" {
		return chatlinks.ErrEventRejected
	}
	if _, err := r.Verifier.VerifyAs(ctx, bearer, oidctoken.BotFrameworkIdentity(app.ClientID, activity.ServiceURL)); err != nil {
		return chatlinks.ErrEventRejected
	}
	return nil
}

func (r *TeamsReceiver) actTeams(ctx context.Context, app capture.ConnectorApp, activity teamsActivity, revision string) (crmcontracts.DispactChannelEventReceipt, error) {
	installed, err := r.installation(ctx, app, activity)
	if err != nil {
		return crmcontracts.DispactChannelEventReceipt{}, err
	}
	if !installed.Installed {
		return r.pauseRemovedTeam(ctx, installed)
	}
	root, eligible := teamsMessageTarget(activity)
	removing := teamsRemoval(activity) && activity.Data.Channel.ID != ""
	if !eligible && !removing {
		return receipt(crmcontracts.DispactChannelEventReceiptStatusIgnored), nil
	}
	bot, err := r.Binding(ctx)
	if errors.Is(err, capture.ErrNoConnection) {
		return receipt(crmcontracts.DispactChannelEventReceiptStatusIgnored), nil
	}
	if err != nil {
		return crmcontracts.DispactChannelEventReceipt{}, err
	}
	channel := installed.Team + "/" + activity.Data.Channel.ID
	links, err := r.Store.ConnectionLinks(ctx, bot.ConnectionID)
	if err != nil {
		return crmcontracts.DispactChannelEventReceipt{}, err
	}
	matched := []chatlinks.Link{}
	for _, link := range links {
		if link.ChannelSlug == channel {
			matched = append(matched, link)
		}
	}
	if len(matched) == 0 {
		return receipt(crmcontracts.DispactChannelEventReceiptStatusIgnored), nil
	}
	if removing {
		return r.removedTeamsChannel(ctx, app, installed, channel, matched)
	}
	if err := r.Store.RecordTeamThread(ctx, installed, channel, root, activity.Conversation.ID, activity.At); err != nil {
		return crmcontracts.DispactChannelEventReceipt{}, err
	}
	if err := r.Store.InvalidateNativeRoot(ctx, bot.ConnectionID, channel, chatwire.RootRef{ID: root, Revision: revision, Deleted: activity.Type == "messageDelete" && root == activity.ID}); err != nil {
		return crmcontracts.DispactChannelEventReceipt{}, err
	}
	if err := r.Pull(ctx, bot.ConnectionID); err != nil {
		return crmcontracts.DispactChannelEventReceipt{}, err
	}
	return receipt(crmcontracts.DispactChannelEventReceiptStatusPullQueued), nil
}

func teamsRemoval(activity teamsActivity) bool {
	remove := activity.Type == "installationUpdate" && activity.Action == "remove"
	for _, member := range activity.Removed {
		remove = remove || member.ID == activity.Recipient.ID
	}
	return remove
}

func (r *TeamsReceiver) removedTeamsChannel(ctx context.Context, app capture.ConnectorApp, installed chatlinks.TeamInstallation, channel string, links []chatlinks.Link) (crmcontracts.DispactChannelEventReceipt, error) {
	credential := chatwire.BotCredential{Workspace: installed.Tenant, Member: installed.Team, NativeTeam: installed.NativeTeam, BotMember: installed.BotMember, ServiceURL: installed.ServiceURL, Credential: messaging.Credential{AppID: installed.AppID}}
	bot, err := TeamsBot(ctx, r.HTTP, app, credential)
	if err != nil {
		return crmcontracts.DispactChannelEventReceipt{}, err
	}
	if _, err = bot.Channel(ctx, channel); err == nil {
		return receipt(crmcontracts.DispactChannelEventReceiptStatusIgnored), nil
	}
	if !errors.Is(err, chatwire.ErrNotMember) && !errors.Is(err, messaging.ErrNotFound) {
		return crmcontracts.DispactChannelEventReceipt{}, err
	}
	for _, link := range links {
		if err := r.Store.MarkChannelGone(ctx, link.ID); err != nil {
			return crmcontracts.DispactChannelEventReceipt{}, err
		}
	}
	return receipt(crmcontracts.DispactChannelEventReceiptStatusLinkPaused), nil
}

func (r *TeamsReceiver) installation(ctx context.Context, app capture.ConnectorApp, activity teamsActivity) (chatlinks.TeamInstallation, error) {
	prior, err := r.Store.TeamInstallationByNative(ctx, app.ClientID, app.Tenant, activity.Data.Team.ID)
	if err != nil && !errors.Is(err, apperrors.ErrNotFound) {
		return prior, err
	}
	remove := teamsRemoval(activity)
	if remove {
		if errors.Is(err, apperrors.ErrNotFound) {
			return chatlinks.TeamInstallation{}, nil
		}
		credential := chatwire.BotCredential{Workspace: prior.Tenant, Member: prior.Team, NativeTeam: prior.NativeTeam, BotMember: prior.BotMember, ServiceURL: prior.ServiceURL, Credential: messaging.Credential{AppID: prior.AppID}}
		bot, checkErr := TeamsBot(ctx, r.HTTP, app, credential)
		if checkErr != nil {
			return prior, checkErr
		}
		if _, checkErr = bot.Me(ctx); checkErr == nil {
			return prior, nil
		}
		if !errors.Is(checkErr, chatwire.ErrNotMember) && !errors.Is(checkErr, messaging.ErrNotFound) {
			return prior, checkErr
		}
		prior.Installed, prior.SignedAt = false, activity.At
		if err := r.Store.RecordTeamInstallation(ctx, prior); err != nil {
			return prior, err
		}
		return r.Store.TeamInstallationByNative(ctx, app.ClientID, app.Tenant, activity.Data.Team.ID)
	}
	if err == nil && prior.Installed {
		return prior, nil
	}
	credential := chatwire.BotCredential{Workspace: app.Tenant, NativeTeam: activity.Data.Team.ID, BotMember: activity.Recipient.ID, ServiceURL: activity.ServiceURL, Credential: messaging.Credential{AppID: app.ClientID}}
	bot, err := TeamsBot(ctx, r.HTTP, app, credential)
	if err != nil {
		return chatlinks.TeamInstallation{}, err
	}
	details, err := bot.TeamDetails(ctx)
	if err != nil {
		return chatlinks.TeamInstallation{}, err
	}
	installation := chatlinks.TeamInstallation{AppID: app.ClientID, Tenant: app.Tenant, Team: details.Group, NativeTeam: details.ID, BotMember: activity.Recipient.ID, ServiceURL: activity.ServiceURL, Name: details.Name, Installed: true, SignedAt: activity.At}
	if err := r.Store.RecordTeamInstallation(ctx, installation); err != nil {
		return installation, err
	}
	return r.Store.TeamInstallationByNative(ctx, app.ClientID, app.Tenant, activity.Data.Team.ID)
}

func (r *TeamsReceiver) pauseRemovedTeam(ctx context.Context, installation chatlinks.TeamInstallation) (crmcontracts.DispactChannelEventReceipt, error) {
	bot, err := r.Binding(ctx)
	if errors.Is(err, capture.ErrNoConnection) {
		return receipt(crmcontracts.DispactChannelEventReceiptStatusIgnored), nil
	}
	if err != nil {
		return crmcontracts.DispactChannelEventReceipt{}, err
	}

	links, err := r.Store.ConnectionLinks(ctx, bot.ConnectionID)
	if err != nil {
		return crmcontracts.DispactChannelEventReceipt{}, err
	}
	for _, link := range links {
		if !strings.HasPrefix(link.ChannelSlug, installation.Team+"/") {
			continue
		}
		if err := r.Links.MarkChannelGone(ctx, link.ID); err != nil {
			return crmcontracts.DispactChannelEventReceipt{}, err
		}
	}
	return receipt(crmcontracts.DispactChannelEventReceiptStatusLinkPaused), nil
}

func teamsMessageTarget(activity teamsActivity) (string, bool) {
	if activity.Type != "message" && activity.Type != "messageUpdate" && activity.Type != "messageDelete" {
		return "", false
	}
	root := activity.Data.ReplyChain.ID
	if root == "" {
		root = activity.ReplyTo
	}
	if root == "" {
		root = activity.ID
	}
	if activity.Data.Channel.ID == "" || activity.Conversation.ID == "" {
		return "", false
	}
	return root, true
}

func handledTeamsActivity(kind string) bool {
	switch kind {
	case "installationUpdate", "conversationUpdate", messageKind, "messageUpdate", messageDeleteType:
		return true
	default:
		return false
	}
}
