// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package comms

import (
	"context"
	"log/slog"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// WithPartyAdmission derives recipients after the successful receipt commits.
func WithPartyAdmission(admit func(context.Context, ids.ActivityID) error) StoreOption {
	return func(s *Store) { s.partyAdmission = admit }
}

// Derivation follows the durable receipt, so its faults cannot request another transmission.
func (s *Store) admitSentParties(caller context.Context, activity ids.ActivityID) {
	if s.partyAdmission == nil {
		return
	}
	ctx, cancel := detachedWrite(caller)
	defer cancel()
	defer func() {
		if problem := recover(); problem != nil {
			slog.ErrorContext(ctx, "post-send participant admission panicked", "activity_id", activity, "panic", problem)
		}
	}()
	if err := s.partyAdmission(ctx, activity); err != nil {
		slog.ErrorContext(ctx, "post-send participant admission failed", "activity_id", activity, "err", err)
	}
}
