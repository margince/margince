// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package whatsapp

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// ReplyWindow is how long after a customer last wrote the business may send them
// a free-form message.
const ReplyWindow = 24 * time.Hour

// Sender is the connector.MessageSender of a connected WhatsApp Business number.
// The auth names the number's access token and its phone number id, so one
// Sender serves every connected number.
type Sender struct{ client *Client }

var _ connector.MessageSender = (*Sender)(nil)

// NewSender builds the sender over the Graph API client.
func NewSender(client *Client) *Sender { return &Sender{client: client} }

// SendMessage transmits one free-form text to the recipient's phone number. The
// receipt carries Meta's message id, which a later reply or echo names.
func (s *Sender) SendMessage(ctx context.Context, auth connector.Auth, msg connector.ChannelMessage) (connector.SendReceipt, error) {
	if err := msg.Validate(); err != nil {
		return connector.SendReceipt{}, err
	}
	if len(msg.Files) > 0 {
		return connector.SendReceipt{}, connector.ErrFilesNotCarried
	}
	creds, ok := connector.ParseChatSendAuth(auth)
	if !ok || creds.Account == "" {
		return connector.SendReceipt{}, fmt.Errorf("whatsapp: the credential names no access token or phone number id: %w", connector.ErrAuthRejected)
	}
	id, err := s.client.SendText(ctx, creds.Token, creds.Account, msg.Recipient.ChannelUserID, msg.Body)
	if err != nil {
		return connector.SendReceipt{}, sendOutcome(err)
	}
	return connector.SendReceipt{ProviderMessageID: id}, nil
}

// sendOutcome maps the Graph API's answers onto the shared send vocabulary. A
// transport fault or an unreadable answer may have delivered the message, and a
// repeat would ask twice, so it is an outcome nobody knows.
func sendOutcome(err error) error {
	switch {
	case errors.Is(err, connector.ErrRateLimited):
		return err
	case errors.Is(err, ErrAuth):
		return fmt.Errorf("%w: %w", connector.ErrAuthRejected, err)
	case errors.Is(err, ErrOutsideWindow):
		return fmt.Errorf("%w: %w", connector.ErrOutsideReplyWindow, err)
	case errors.Is(err, ErrRecipientInvalid):
		return fmt.Errorf("%w: %w", connector.ErrRecipientUnreachable, err)
	case errors.Is(err, ErrUnreachable):
		return fmt.Errorf("%w: %w", connector.ErrSendOutcomeUnknown, err)
	}
	return err
}
