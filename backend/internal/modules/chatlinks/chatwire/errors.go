// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package chatwire

import "errors"

// Provider failures preserve consent, membership, source and delivery distinctions for repair.
var (
	ErrHistoryProfileNeedsReview = errors.New("chat: review the Slack installation history profile before canonical sync")
	ErrAudienceNeedsReview       = errors.New("chat: review the channel audience before automatic sharing")
	ErrMembersConsentRequired    = errors.New("chat: approve channel member access before automatic sharing")
	ErrThreadTooLong             = errors.New("chat: the thread is too long to read completely")
	ErrTokenRejected             = errors.New("chat: the bot token was rejected")
	ErrNotMember                 = errors.New("chat: the bot is not a member of the channel")
	ErrChannelGone               = errors.New("chat: the channel is gone")
	ErrUnreachable               = errors.New("chat: the chat could not be reached")
	ErrRequestRejected           = errors.New("chat: the chat refused the request")
	ErrNameRefused               = errors.New("chat: the chat refused the channel name")
)
