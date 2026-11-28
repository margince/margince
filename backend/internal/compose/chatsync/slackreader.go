// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package chatsync

import (
	"context"
	"time"

	"github.com/margince/margince/backend/internal/modules/chatlinks/messaging"
)

func refreshSlackToken(ctx context.Context, previous messaging.Credential, refresh func(context.Context, messaging.Credential) (messaging.Credential, error), now time.Time) (messaging.Credential, error) {
	if previous.ExpiresAt.IsZero() || previous.ExpiresAt.After(now.Add(2*time.Minute)) {
		return previous, nil
	}
	return refresh(ctx, previous)
}
