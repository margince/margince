// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package chatsync

import (
	"context"
	"slices"
	"strings"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/chatlinks"
	"github.com/margince/margince/backend/internal/modules/chatlinks/chatwire"
	"github.com/margince/margince/backend/internal/modules/chatlinks/messaging"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// PreviewDelivery shows eligible content and audience before recurring posting is enabled.
func (f *Filer) PreviewDelivery(ctx context.Context, kind string, recordID ids.UUID) (crmcontracts.ChannelDeliveryPreview, error) {
	link, err := f.Store.ManagedLink(ctx, kind, recordID)
	if err != nil {
		return crmcontracts.ChannelDeliveryPreview{}, err
	}
	grant, _, err := f.Store.OpenMember(ctx, link.Provider)
	if err != nil {
		return crmcontracts.ChannelDeliveryPreview{}, err
	}
	system := principal.WithActor(ctx, chatLinksPrincipal)
	session, err := chatlinks.OpenProvider(system, f.Chats, link.Provider)
	if err != nil {
		return crmcontracts.ChannelDeliveryPreview{}, err
	}
	if session.ConnectionID != link.ConnectionID || grant.Workspace != session.TeamID {
		return crmcontracts.ChannelDeliveryPreview{}, messaging.ErrUnauthorized
	}
	if _, native := session.Chat.(chatwire.RootReader); !native {
		return crmcontracts.ChannelDeliveryPreview{}, messaging.ErrUnsupported
	}
	origin := ""
	if f.PreviewOrigin != nil {
		origin = f.PreviewOrigin()
	}
	engine := Flusher{Pool: f.Pool, Store: f.Store, Chats: f.Chats, Lane: f.Lane, Origin: origin, previewOnly: true}
	record, err := readRecord(system, f.Pool, link)
	if err != nil {
		return crmcontracts.ChannelDeliveryPreview{}, err
	}
	pending, err := f.Store.PendingFor(system, link.ID)
	if err != nil {
		return crmcontracts.ChannelDeliveryPreview{}, err
	}
	// Resolve participant access before any source text reaches the model.
	audience, err := engine.outboundAudience(system, session.Chat, link)
	if err != nil {
		return crmcontracts.ChannelDeliveryPreview{}, err
	}
	if err := f.confirmDeliveryPreview(ctx, kind, recordID, link, grant, audience); err != nil {
		return crmcontracts.ChannelDeliveryPreview{}, err
	}
	prepared, err := engine.nativeLines(system, session.Chat, link, record, pending.Items, 25000)
	if err != nil {
		return crmcontracts.ChannelDeliveryPreview{}, err
	}
	if err := f.confirmDeliveryPreview(ctx, kind, recordID, link, grant, prepared.audience); err != nil {
		return crmcontracts.ChannelDeliveryPreview{}, err
	}
	out := crmcontracts.ChannelDeliveryPreview{LinkId: crmcontracts.Id(link.ID), Version: link.Version, Provider: crmcontracts.MessengerProvider(link.Provider), Workspace: link.WorkspaceName, ChannelName: link.ChannelName, RootId: optionalPreviewRoot(link.RootID), OverviewText: chatCardText(record, origin), UpdateText: strings.Join(prepared.text, "\n\n"), PendingUpdates: len(prepared.text), ExcludedUpdates: len(pending.Items) - len(prepared.text), Verbosity: crmcontracts.ChannelVerbosity(link.Verbosity), MessageDetail: crmcontracts.ChannelMessageDetail(link.MessageDetail), Audience: []crmcontracts.ChannelPreviewMember{}}
	out.Audience = previewAudienceMembers(prepared.audience)
	return out, nil
}

func (f *Filer) confirmDeliveryPreview(ctx context.Context, kind string, record ids.UUID, link chatlinks.Link, grant chatlinks.MemberGrant, audience outboundAudience) error {
	current, err := f.Store.ManagedLink(ctx, kind, record)
	if err != nil {
		return err
	}
	if current.ID != link.ID || current.Version != link.Version {
		return apperrors.ErrConflict
	}
	live, personal, err := f.Store.OpenMember(ctx, link.Provider)
	if err != nil || live.ID != grant.ID || live.Version != grant.Version {
		return messaging.ErrUnauthorized
	}
	conversation, err := personal.Conversation(ctx, link.ChannelSlug)
	if err != nil {
		return err
	}
	personalKey, err := previewAudienceKey(conversation, audience.bot)
	if err != nil {
		return err
	}
	botKey, err := previewAudienceKey(audience.conversation, audience.bot)
	if err != nil || personalKey != botKey {
		return apperrors.ErrConflict
	}
	return nil
}

func previewAudienceKey(conversation messaging.Conversation, bot string) (string, error) {
	conversation.Members = slices.DeleteFunc(slices.Clone(conversation.Members), func(member messaging.Member) bool { return member.ID == bot })
	return messaging.AudienceKey(conversation)
}

func optionalPreviewRoot(root string) *string {
	if root == "" {
		return nil
	}
	return &root
}

func previewMemberName(member messaging.Member) string {
	if member.Name != "" {
		return member.Name
	}
	return member.Email
}

func previewAudienceMembers(audience outboundAudience) []crmcontracts.ChannelPreviewMember {
	members := []crmcontracts.ChannelPreviewMember{}
	for _, member := range audience.conversation.Members {
		if member.Active && member.ID != audience.bot {
			members = append(members, crmcontracts.ChannelPreviewMember{Id: member.ID, Name: previewMemberName(member)})
		}
	}
	return members
}
