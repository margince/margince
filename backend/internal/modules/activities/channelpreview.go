// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// DecodeSendMessageRequest keeps preview consent claims subject to the send path's parser.
func DecodeSendMessageRequest(req crmcontracts.SendMessageRequest) (SendMessageInput, error) {
	claimed, err := sendContextFrom((*string)(req.CommunicationContext), req.MarketingPurpose, req.OperatorReason, req.Evidence)
	if err != nil {
		return SendMessageInput{}, err
	}
	return SendMessageInput{
		Body: req.Body, AttachmentIDs: attachmentIDsFrom(req.AttachmentIds), ConsentPurpose: legacyPurposeOf(req.ConsentPurpose),
		Context: claimed.category, MarketingPurpose: claimed.marketing, OperatorReason: claimed.reason, Evidence: claimed.evidence,
	}, nil
}

// ChannelReplyPreview binds the reply destination to the readable anchor, not caller-supplied routing.
type ChannelReplyPreview struct {
	Provider  string
	Recipient connector.ChannelIdentity
	Links     []ActivityLinkInput
}

// PreviewChannelReply checks the destination and authority without sending a message.
func (s *Store) PreviewChannelReply(ctx context.Context, anchorID ids.ActivityID) (ChannelReplyPreview, error) {
	anchor, err := s.GetActivity(ctx, anchorID, storekit.LiveOnly)
	if err != nil {
		return ChannelReplyPreview{}, err
	}
	if err := auth.Require(ctx, "activity", principal.ActionCreate); err != nil {
		return ChannelReplyPreview{}, err
	}
	if s.reachability == nil {
		return ChannelReplyPreview{}, errNoChannelReachability
	}
	provider, err := s.channelProviderOf(ctx, anchorID)
	if err != nil {
		return ChannelReplyPreview{}, err
	}
	if provider == "" {
		return ChannelReplyPreview{}, &NotAChannelConversationError{Kind: string(anchor.Kind)}
	}
	conversation, err := s.resolveConversation(ctx, anchorID, provider)
	if err != nil {
		return ChannelReplyPreview{}, err
	}
	return ChannelReplyPreview{Provider: provider, Recipient: conversation.recipient, Links: inheritedLinks(anchor)}, nil
}
