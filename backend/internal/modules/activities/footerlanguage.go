// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// What language the unsubscribe footer speaks, and whether the links in
// it can be opened at all.

import (
	"context"
	"fmt"

	"github.com/margince/margince/backend/internal/platform/netguard"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
	"github.com/margince/margince/backend/internal/shared/runtimeenv"
)

// BaseLanguageReader answers the installation's configured language.
// Injected, because it lives in identity and a module never imports a
// sibling.
type BaseLanguageReader interface {
	BaseLanguage(ctx context.Context) string
}

// BaseLanguageFunc adapts a plain function.
type BaseLanguageFunc func(ctx context.Context) string

// BaseLanguage satisfies BaseLanguageReader.
func (f BaseLanguageFunc) BaseLanguage(ctx context.Context) string { return f(ctx) }

// WithBaseLanguage wires the fallback behind the footer's language.
func (s *Store) WithBaseLanguage(r BaseLanguageReader) *Store {
	s.baseLanguage = r
	return s
}

// WithRuntimeEnvironment tells the send path which posture it runs under,
// which is what decides whether a loopback public origin is a working dev
// stack or a message nobody can act on.
func (s *Store) WithRuntimeEnvironment(env runtimeenv.Environment) *Store {
	s.env = env
	return s
}

// footerLanguage picks the language of the footer from the language of
// the message it sits under.
//
// The ladder is body, then subject, then the installation's own language,
// then English — the same shape the drafting floor already trusts.
// textlang.Detect is deliberately biased toward Unknown (it wants three
// stopword hits and a clear margin), so a two-line note falls through to
// the tiers below rather than being guessed at.
//
// Worth being plain about the limit: this picks the language of the
// MESSAGE, not the recipient's preference, which nothing in the product
// records. An English mail to a German reader gets an English footer,
// which at least matches the message around it — and the page the link
// opens carries a language switcher, which is the recovery path.
func (s *Store) footerLanguage(ctx context.Context, body, subject string) textlang.Lang {
	if lang := textlang.Detect(body); lang != textlang.Unknown {
		return lang
	}
	if lang := textlang.Detect(subject); lang != textlang.Unknown {
		return lang
	}
	if s.baseLanguage != nil {
		if base := s.baseLanguage.BaseLanguage(ctx); textlang.Known(base) {
			return textlang.Lang(base)
		}
	}
	return textlang.English
}

// PublicOriginUnusableError is the refusal when a tokenized send cannot
// build a link the recipient could open.
//
// A MessageFault rather than a field fault: nothing in the request says
// what the public origin is, so pointing at an argument would hand the
// caller a task they cannot perform. An operator has to fix the
// deployment.
type PublicOriginUnusableError struct {
	// Reason says what is wrong with the configured origin. It never
	// carries the origin itself, which may hold credentials.
	Reason string
}

func (e *PublicOriginUnusableError) Error() string {
	return "send: " + e.Reason
}

// MessageFault names the condition and who must act on it.
func (e *PublicOriginUnusableError) MessageFault() (string, string) {
	return "public_origin_unusable", "This message carries an unsubscribe link, and " + e.Reason +
		" — an administrator must set the installation's public address before it can be sent."
}

// publicOriginUsable refuses a tokenized send whose links would not work.
func (s *Store) publicOriginUsable() error {
	if s.publicBaseURL == "" {
		// Wording kept deliberately: this is the sentence the send path has
		// always failed with when no base is configured.
		return &PublicOriginUnusableError{Reason: "public base URL is not configured"}
	}
	if err := netguard.RequirePublicOrigin("the public base URL", s.publicBaseURL, s.env); err != nil {
		return &PublicOriginUnusableError{Reason: fmt.Sprintf("%v", err)}
	}
	return nil
}

// WithRuntimeEnvironment is the Handlers half of the same wiring: the HTTP
// transport and the tool surface must agree about the posture, or one of
// them sends links the other would refuse.
func (h Handlers) WithRuntimeEnvironment(env runtimeenv.Environment) Handlers {
	h.store = h.store.WithRuntimeEnvironment(env)
	return h
}

// WithBaseLanguage is the Handlers half of the footer's language fallback.
func (h Handlers) WithBaseLanguage(r BaseLanguageReader) Handlers {
	h.store = h.store.WithBaseLanguage(r)
	return h
}
