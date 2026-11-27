// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"fmt"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// SenderFor resolves the user's transmitting connection that mailbox names:
// the EmailSender seam, its unsealed credential, and the provider's scopes. A
// delivery staged before deliveries named a mailbox names the provider alone.
// It goes out from the default.
//
//nolint:ireturn // returns the optional connector.EmailSender seam by design (the same posture Registry.connector takes for connector.Connector)
func (r *Registry) SenderFor(ctx context.Context, userID ids.UserID, mailbox Mailbox) (connector.EmailSender, connector.Auth, []string, error) {
	shared, err := r.namesSharedMailbox(ctx, mailbox)
	if err != nil {
		return nil, nil, nil, err
	}
	if shared {
		return r.sharedSenderFor(ctx, userID, mailbox.ID)
	}
	// Two-value form: EmailSender is optional (connector.go), so a capture-only
	// connector is reported rather than silently treated as absent.
	var sender connector.EmailSender
	auth, granted, err := r.liveConnection(ctx, userID, mailbox, func(c connector.Connector) error {
		var sends bool
		if sender, sends = c.(connector.EmailSender); !sends {
			return fmt.Errorf("capture: connector %q: %w", mailbox.Provider, ErrConnectorCannotSend)
		}
		return nil
	})
	if err != nil {
		return nil, nil, nil, err
	}
	return sender, auth, granted, nil
}
