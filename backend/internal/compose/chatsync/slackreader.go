// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package chatsync

import (
	"context"
	"errors"
	"time"

	"github.com/margince/margince/backend/internal/modules/chatlinks/chatwire"
	"github.com/margince/margince/backend/internal/modules/chatlinks/messaging"
	"github.com/margince/margince/backend/internal/modules/chatlinks/slack"
)

func refreshSlackToken(ctx context.Context, previous messaging.Credential, refresh func(context.Context, messaging.Credential) (messaging.Credential, error), now time.Time) (messaging.Credential, error) {
	if previous.ExpiresAt.IsZero() || previous.ExpiresAt.After(now.Add(2*time.Minute)) {
		return previous, nil
	}
	return refresh(ctx, previous)
}

func slackReaderFailure(err error) error {
	if errors.Is(err, messaging.ErrUnauthorized) || errors.Is(err, messaging.ErrNotFound) {
		return errors.Join(chatwire.ErrHistoryProfileNeedsReview, err)
	}
	return errors.Join(chatwire.ErrUnreachable, err)
}

func (c LiveChats) slackThreadReader(ctx context.Context, credential chatwire.BotCredential, failure error) (*slack.Client, error) {
	if failure != nil {
		return nil, failure
	}
	if credential.SlackReader == nil || credential.SlackReader.AccessToken == "" || credential.SlackReaderMember == "" || credential.SlackReader.AppID != credential.Credential.AppID {
		return nil, chatwire.ErrHistoryProfileNeedsReview
	}
	reader := slack.New(c.HTTP, credential.SlackReader.AccessToken, credential.Workspace, credential.SlackReaderMember)
	identity, err := reader.Identity(ctx)
	if err != nil {
		return nil, slackReaderFailure(err)
	}
	if !identity.Member.Active || identity.Member.Bot || identity.Member.External {
		return nil, chatwire.ErrHistoryProfileNeedsReview
	}
	return reader, nil
}
