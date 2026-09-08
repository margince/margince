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
	"github.com/margince/margince/backend/internal/shared/kernel/values"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// reservedDomains is test_mailbox's own quarantine (RFC 2606) — independent
// of offline_demo's dataset addresses, which are a different safety story.
// Named domains (example.com/.net/.org) admit their subdomains too — a
// realistic address like qa@mail.example.com belongs in the quarantine as
// much as buyer@example.com does. The four bare labels (test/example/
// invalid/localhost) are TLDs: any domain ending in one is reserved by
// definition, at any depth.
var reservedDomains = map[string]bool{
	"example.com": true, "example.net": true, "example.org": true,
	"test": true, "example": true, "invalid": true, "localhost": true,
}

// inQuarantine reports whether addr's domain is inside the reserved set —
// exactly, or as a subdomain of a reserved name or TLD. addr is parsed
// through values.ParseEmail rather than cut on "@" by hand: an address with
// more than one "@" (a quoted local part, RFC 5321) names its domain after
// the LAST one, and only a real parse gets that right — a raw first-"@" cut
// would misread the domain for such an address. A malformed addr — anything
// ParseEmail refuses — is outside the quarantine by construction: nothing
// here is a real destination for it to reach.
func inQuarantine(addr string) bool {
	email, err := values.ParseEmail(addr)
	if err != nil {
		return false
	}
	domain := email.Domain()
	for {
		if reservedDomains[domain] {
			return true
		}
		i := strings.IndexByte(domain, '.')
		if i < 0 {
			return false
		}
		domain = domain[i+1:]
	}
}

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
