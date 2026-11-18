// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package mailprobe answers a pending mail approval whether its sender has a
// mailbox that can transmit, with the check the send itself runs.
package mailprobe

import (
	"context"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// For asks the check a send runs. An installation composed without an authority
// runs no pre-flight, so every mailbox counts as able to send.
func For(authority activities.SendAuthority) approvals.SendProbe {
	return func(ctx context.Context) (bool, error) {
		if authority == nil {
			return true, nil
		}
		// No chosen mailbox and no anchor: the default mailbox the send falls back to.
		provider, _, err := authority.SendableMailbox(ctx, ids.UUID{}, ids.ActivityID{})
		return provider != "", err
	}
}
