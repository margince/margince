// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

// The language this installation's transactional mail is written in.
//
// A password link and an invitation are read once, in a moment where the reader
// already knows what they asked for — so the stakes are lower than the weekly
// retrospective's. What they have in common is that an installation whose
// screens are German should not change language the moment the product leaves
// the browser.

import (
	"context"
	"log/slog"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/mailcopy"
)

// mailCopy resolves what this installation's mail says.
//
// It answers the FALLBACK language on any failure, and logs it, for the reason
// BaseLanguageForPrompt gives: refusing to send a password link because a
// settings read failed trades the whole message for a formatting preference,
// and a link that does not arrive is worse for its reader than an English one.
//
// A missing row does not reach that line — BaseLanguageOf answers the
// registered default for one — so a log here always means something went wrong.
func (h Handlers) mailCopy(ctx context.Context) mailcopy.Copy {
	language := string(mailcopy.Fallback)
	err := h.svc.db.Tx(ctx, func(tx pgx.Tx) error {
		resolved, err := BaseLanguageOf(ctx, tx)
		if err != nil {
			return err
		}
		language = resolved
		return nil
	})
	if err != nil {
		slog.WarnContext(ctx, "the installation's mail language could not be read; sending in the fallback",
			"fallback", mailcopy.Fallback, "cause", err)
	}
	return mailcopy.For(language)
}
