// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

// The language this installation's transactional mail is written in.
//
// A password link and an invitation are read once, in a moment where the reader
// already knows what they asked for. What they have in common with the weekly
// retrospective is that an installation whose screens are German should not
// change language the moment the product leaves the browser.

import (
	"context"
	"log/slog"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/mailcopy"
)

// mailCopy resolves what this installation's mail says.
//
// It reads through LanguageOf, which is the one reader of this setting: the
// query and the "which principal may ask" decision are spelled once, and a
// second copy here would be a second place to get either wrong.
//
// Held by: TestTheInstallationLanguageHasOneReader
// (backend/gates/maillanguagereader_test.go)
//
// LanguageOf goes through settings.ApplyTx, which skips the entry's read gate.
// That is what this path needs: those readers take the installation_settings
// object gate, and this runs where there is no principal for the gate to judge.
// `POST /auth/forgot-password` is public — it answers 202 before it knows
// whether the address maps to an account, and everything after that runs off
// the request path with nothing bound. Through the gated reader every reset
// mail would be refused the setting and fall back to English, on exactly the
// installations this catalog exists for.
//
// Nothing is widened by it. The value is the installation's own label, chosen
// by an administrator and shown on a settings screen; it is not tenant data,
// and the language a message is written in is not a fact about the contact
// receiving it.
//
// It answers the FALLBACK language on any failure, and logs it, for the reason
// BaseLanguageForPrompt gives: refusing to send a password link because a
// settings read failed trades the whole message for a formatting preference,
// and a link that does not arrive is worse for its reader than an English one.
// An installation with no row stored is not a failure — it has never chosen,
// which is what the fallback is for — so a log here always means something
// actually went wrong.
func (h Handlers) mailCopy(ctx context.Context) mailcopy.Copy {
	language := string(mailcopy.Fallback)
	err := h.svc.db.Tx(ctx, func(tx pgx.Tx) error {
		stored, err := LanguageOf(ctx, tx)
		if err != nil {
			return err
		}
		if stored != "" {
			language = stored
		}
		return nil
	})
	if err != nil {
		slog.WarnContext(ctx, "the installation's mail language could not be read; sending in the fallback",
			"fallback", mailcopy.Fallback, "cause", err)
	}
	return mailcopy.For(language)
}
