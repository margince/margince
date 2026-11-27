// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package zalo

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// ReplyWindow is how long after a customer last wrote the Official Account may
// send them a customer-service message.
const ReplyWindow = 48 * time.Hour

// Sender is the connector.MessageSender of a connected Official Account. The
// auth names the OA's access token.
type Sender struct{ client *Client }

var _ connector.MessageSender = (*Sender)(nil)

// NewSender builds the sender over the OA Open API client.
func NewSender(client *Client) *Sender { return &Sender{client: client} }

// SendMessage transmits one customer-service text to the recipient's Zalo id.
func (s *Sender) SendMessage(ctx context.Context, auth connector.Auth, msg connector.ChannelMessage) (connector.SendReceipt, error) {
	if err := msg.Validate(); err != nil {
		return connector.SendReceipt{}, err
	}
	if len(msg.Files) > 0 {
		return connector.SendReceipt{}, connector.ErrFilesNotCarried
	}
	creds, ok := connector.ParseChatSendAuth(auth)
	if !ok {
		return connector.SendReceipt{}, fmt.Errorf("zalo: the credential names no access token: %w", connector.ErrAuthRejected)
	}
	id, err := s.client.SendText(ctx, creds.Token, msg.Recipient.ChannelUserID, msg.Body)
	if err != nil {
		return connector.SendReceipt{}, sendOutcome(err)
	}
	return connector.SendReceipt{ProviderMessageID: id}, nil
}

// sendOutcome maps Zalo's answers onto the shared send vocabulary. A call with
// no usable answer may have delivered the message, so its outcome is unknown.
func sendOutcome(err error) error {
	switch {
	case errors.Is(err, ErrRateLimited):
		return fmt.Errorf("%w: %w", &connector.RateLimitedError{RetryAfter: RetryAfter, Status: 429}, err)
	case errors.Is(err, ErrReauthRequired):
		return fmt.Errorf("%w: %w", connector.ErrAuthRejected, err)
	case errors.Is(err, ErrOutsideReplyWindow):
		return fmt.Errorf("%w: %w", connector.ErrOutsideReplyWindow, err)
	case errors.Is(err, ErrInvalidRecipient):
		return fmt.Errorf("%w: %w", connector.ErrRecipientUnreachable, err)
	case errors.Is(err, ErrUnreachable):
		return fmt.Errorf("%w: %w", connector.ErrSendOutcomeUnknown, err)
	}
	return err
}
