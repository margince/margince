// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package mailer

import (
	"context"

	"github.com/margince/margince/backend/internal/platform/emailview"
)

// SendProduct preserves the exact text and semantic blocks supplied by a product builder.
func SendProduct(ctx context.Context, relay Mailer, to, subject string, message emailview.Message) error {
	if alternative, ok := relay.(AlternativeSender); ok {
		return alternative.SendAlternative(ctx, to, subject, message.Text, emailview.Render(message), "")
	}
	return relay.Send(ctx, to, subject, message.Text)
}
