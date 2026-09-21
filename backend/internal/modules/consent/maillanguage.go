// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// Which language this installation's controller mail is written in.
//
// The setting lives in identity, and a module never imports a sibling — so the
// reader is injected and this file holds only the seam, exactly as
// installationcountry.go does for where the installation is established.

import (
	"context"

	"github.com/jackc/pgx/v5"
)

// MailLanguageReader answers the installation's base language, or empty when it
// is unstated.
//
// It takes the CALLER'S transaction rather than opening its own, for the reason
// InstallationCountryReader does: the wording it selects is staged on that
// transaction, so reading the language outside it would let the setting change
// between the read and the write and stage a message in a language the
// installation had already stopped speaking.
type MailLanguageReader interface {
	MailLanguageTx(ctx context.Context, tx pgx.Tx) (string, error)
}

// MailLanguageFunc adapts a plain function, so compose can inject identity's
// reader without either module knowing the other.
type MailLanguageFunc func(ctx context.Context, tx pgx.Tx) (string, error)

// MailLanguageTx satisfies MailLanguageReader.
func (f MailLanguageFunc) MailLanguageTx(ctx context.Context, tx pgx.Tx) (string, error) {
	return f(ctx, tx)
}

// WithMailLanguage injects the reader behind the controller mail's wording.
//
// It lands on the STORE because that is where the mail is staged: issueLink
// renders and queues on one transaction, and the handlers reach it through the
// store they already hold.
func (s *Store) WithMailLanguage(r MailLanguageReader) *Store {
	s.language = r
	return s
}

// WithMailLanguage is the handlers-side spelling, for the surface that mints a
// confirm link.
func (h Handlers) WithMailLanguage(r MailLanguageReader) Handlers {
	h.store = h.store.WithMailLanguage(r)
	return h
}
