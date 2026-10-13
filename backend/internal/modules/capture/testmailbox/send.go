// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package testmailbox

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// authPayload carries whose seat this credential names. There is no real
// credential to seal; Credential is the one place this shape is spelled, so
// the connect handler and every test that needs one call it rather than
// hand-marshalling the same fields a second time.
type authPayload struct {
	UserID string `json:"user_id"`
}

// Credential mints the opaque Auth bundle naming userID as the connection's
// owner — what the connect handler seals and what SendEmail/Sync read back.
func Credential(userID ids.UUID) (connector.Auth, error) {
	b, err := json.Marshal(authPayload{UserID: userID.String()})
	if err != nil {
		return nil, fmt.Errorf("test_mailbox: marshalling a credential for %s: %w", userID, err)
	}
	return connector.Auth(b), nil
}

func readUserID(auth connector.Auth) (ids.UUID, error) {
	var payload authPayload
	if err := json.Unmarshal(auth, &payload); err != nil {
		return ids.Nil, fmt.Errorf("test_mailbox: auth bundle carries no seat id: %w", err)
	}
	userID, err := ids.Parse(payload.UserID)
	if err != nil {
		return ids.Nil, fmt.Errorf("test_mailbox: auth bundle seat id %q is not a uuid: %w", payload.UserID, err)
	}
	return userID, nil
}

// SendEmail validates, refuses any recipient outside the RFC 2606 reserved
// names (values.IsReservedAddress), records the send for its own echo
// (sync.go), and returns a synthetic, idempotent receipt. It never reaches
// the network.
func (c *Connector) SendEmail(ctx context.Context, auth connector.Auth, msg connector.EmailMessage) (connector.SendReceipt, error) {
	if err := msg.Validate(); err != nil {
		return connector.SendReceipt{}, err
	}
	for _, addr := range append(append(append([]string{}, msg.To...), msg.Cc...), msg.Bcc...) {
		if !values.IsReservedAddress(addr) {
			return connector.SendReceipt{}, fmt.Errorf("test_mailbox: %q is outside the reserved-domain quarantine: %w", addr, connector.ErrRecipientUnreachable)
		}
	}
	userID, err := readUserID(auth)
	if err != nil {
		return connector.SendReceipt{}, err
	}
	if err := c.ledger.RecordSent(ctx, userID, msg.MessageID, msg.To, msg.Cc, msg.Subject); err != nil {
		return connector.SendReceipt{}, fmt.Errorf("test_mailbox: recording the send for its own echo: %w", err)
	}
	sum := sha256.Sum256([]byte(msg.MessageID))
	return connector.SendReceipt{
		ProviderMessageID: "test_mailbox:" + hex.EncodeToString(sum[:8]),
		RFC822MessageID:   "",
	}, nil
}

var _ connector.EmailSender = (*Connector)(nil)
