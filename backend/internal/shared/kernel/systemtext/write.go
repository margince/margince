// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package systemtext

import (
	"context"
	"log/slog"
)

// ForWrite drops invalid display metadata so valid domain work can still commit.
func ForWrite(ctx context.Context, message *Message, subject, body string) *Message {
	bound, err := Bind(message, subject, body)
	if err != nil {
		slog.ErrorContext(ctx, "system text display metadata rejected", "err", err)
		return nil
	}
	return bound
}

// FitForWrite preserves canonical domain text when optional display fitting fails.
func FitForWrite(ctx context.Context, message *Message, subject, body string, subjectLimit, bodyLimit int) (*Message, string, string) {
	fitted, title, text, err := Fit(message, subject, body, subjectLimit, bodyLimit)
	if err != nil {
		slog.ErrorContext(ctx, "system text display could not fit", "err", err)
		return nil, subject, body
	}
	return fitted, title, text
}

// BoundEffect admits only provenance already bound by the staging owner.
func BoundEffect(message *Message) *Message {
	if message == nil || message.Version != 1 {
		return nil
	}
	out := *message
	if out.Subject != nil && out.Subject.Digest == "" {
		out.Subject = nil
	}
	if out.Body != nil && out.Body.Digest == "" {
		out.Body = nil
	}
	if out.Subject == nil && out.Body == nil {
		return nil
	}
	return &out
}
