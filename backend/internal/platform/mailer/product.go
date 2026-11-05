// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package mailer

import (
	"context"

	"github.com/margince/margince/backend/internal/platform/emailview"
)

// SendProduct preserves the exact text and semantic blocks supplied by a product builder.
//
// A relay that knows the installation's letterhead draws the mail under it. A
// relay that only takes alternatives gets the plain "Margince" header, and a
// text-only relay gets the text.
func SendProduct(ctx context.Context, relay Mailer, to, subject string, message emailview.Message) error {
	if branded, ok := relay.(BrandedSender); ok {
		return branded.SendBranded(ctx, to, subject, message)
	}
	if alternative, ok := relay.(AlternativeSender); ok {
		return alternative.SendAlternative(ctx, to, subject, message.Text, emailview.Render(message, emailview.Letterhead{}), "")
	}
	return relay.Send(ctx, to, subject, message.Text)
}
