// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package comms

import (
	"context"
	"log/slog"
)

// SentEffect is a domain change that waits on a message being handed to the
// mail provider. It is told the message id this system minted for the delivery.
// It runs only after the receipt has committed. So it must be derivable from
// the sent receipt alone, and a periodic pass must be able to repeat it.
type SentEffect func(ctx context.Context, messageID string) error

// WithSentEffect adds an effect that runs once a delivery's receipt lands.
func WithSentEffect(effect SentEffect) StoreOption {
	return func(s *Store) { s.sentEffect = effect }
}

// applySentEffect runs the effect after the receipt committed, so a failing
// effect can never undo a receipt and mail the recipient a second time. A
// failure is logged; the owner's periodic pass applies the effect again.
func (s *Store) applySentEffect(ctx context.Context, messageID string) {
	if s.sentEffect == nil || messageID == "" {
		return
	}
	ctx, cancel := detachedWrite(ctx)
	defer cancel()
	if err := s.sentEffect(ctx, messageID); err != nil {
		slog.ErrorContext(ctx, "a change waiting on a sent message did not apply; its periodic pass retries it",
			"message_id", messageID, "cause", err)
	}
}
