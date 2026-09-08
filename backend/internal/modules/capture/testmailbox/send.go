// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package testmailbox

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// reservedDomains is test_mailbox's own quarantine (RFC 2606) — independent
// of offline_demo's dataset addresses, which are a different safety story.
var reservedDomains = map[string]bool{
	"example.com": true, "example.net": true, "example.org": true,
	"test": true, "example": true, "invalid": true, "localhost": true,
}

// inQuarantine reports whether addr's domain (or, for the four bare TLD
// entries above, its final label) is inside the reserved set.
func inQuarantine(addr string) bool {
	_, domain, ok := strings.Cut(addr, "@")
	if !ok {
		return false
	}
	domain = strings.ToLower(domain)
	if reservedDomains[domain] {
		return true
	}
	if i := strings.LastIndexByte(domain, '.'); i >= 0 {
		return reservedDomains[domain[i+1:]]
	}
	return false
}

// authPayload carries whose seat this credential names — the connect
// handler (compose/connectors_testmailbox.go) writes this from the
// authenticated actor, exactly like offlinedemo.authPayload; there is no
// real credential to seal.
type authPayload struct {
	UserID string `json:"user_id"`
}

func readUserID(auth connector.Auth) (ids.UUID, error) {
	var payload authPayload
	if err := json.Unmarshal(auth, &payload); err != nil {
		return ids.Nil, fmt.Errorf("test_mailbox: auth bundle carries no seat id: %w", err)
	}
	return ids.Parse(payload.UserID)
}

// SendEmail validates, quarantines, records the send for its own echo
// (sync.go), and returns a synthetic, idempotent receipt. It never reaches
// the network.
func (c *Connector) SendEmail(ctx context.Context, auth connector.Auth, msg connector.EmailMessage) (connector.SendReceipt, error) {
	if err := msg.Validate(); err != nil {
		return connector.SendReceipt{}, err
	}
	for _, addr := range append(append(append([]string{}, msg.To...), msg.Cc...), msg.Bcc...) {
		if !inQuarantine(addr) {
			return connector.SendReceipt{}, fmt.Errorf("test_mailbox: %q is outside the reserved-domain quarantine: %w", addr, connector.ErrRecipientUnreachable)
		}
	}
	userID, err := readUserID(auth)
	if err != nil {
		return connector.SendReceipt{}, err
	}
	if err := c.ledger.RecordSent(ctx, userID, msg.MessageID, msg.To, msg.Subject); err != nil {
		return connector.SendReceipt{}, fmt.Errorf("test_mailbox: recording the send for its own echo: %w", err)
	}
	sum := sha256.Sum256([]byte(msg.MessageID))
	return connector.SendReceipt{
		ProviderMessageID: "test_mailbox:" + hex.EncodeToString(sum[:8]),
		RFC822MessageID:   "",
	}, nil
}

var _ connector.EmailSender = (*Connector)(nil)
