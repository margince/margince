// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package systemtext

import (
	"context"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

type localeKey struct{}

// WithLocale records a validated human reader’s presentation override.
func WithLocale(ctx context.Context, locale textlang.Lang) context.Context {
	return context.WithValue(ctx, localeKey{}, locale)
}

// Locale falls back from the reader override to the principal’s saved language.
func Locale(ctx context.Context) textlang.Lang {
	if locale, ok := ctx.Value(localeKey{}).(textlang.Lang); ok && textlang.Known(string(locale)) {
		return locale
	}
	if actor, ok := principal.Actor(ctx); ok && textlang.Known(actor.Locale) {
		return textlang.Lang(actor.Locale)
	}
	return textlang.English
}

// HasLocale distinguishes an explicit reader override from the language fallback.
func HasLocale(ctx context.Context) bool {
	locale, ok := ctx.Value(localeKey{}).(textlang.Lang)
	return ok && textlang.Known(string(locale))
}

type messageKey struct{}

// WithMessage carries internal provenance without admitting unbound generated prose.
func WithMessage(ctx context.Context, message *Message) context.Context {
	cloned := ForDestination(message)
	if cloned != nil {
		if cloned.Subject != nil && cloned.Subject.Origin == Generated && cloned.Subject.Digest == "" {
			cloned.Subject = nil
		}
		if cloned.Body != nil && cloned.Body.Origin == Generated && cloned.Body.Digest == "" {
			cloned.Body = nil
		}
	}
	return context.WithValue(ctx, messageKey{}, cloned)
}

// MessageIn reads provenance supplied by an internal producer, never a request body.
func MessageIn(ctx context.Context) *Message {
	message, ok := ctx.Value(messageKey{}).(*Message)
	if !ok {
		return nil
	}
	return message
}
