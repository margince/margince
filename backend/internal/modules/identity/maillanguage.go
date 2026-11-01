// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"context"
	"errors"
	"log/slog"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/mailcopy"
	"github.com/margince/margince/backend/internal/platform/mailidentity"
	"github.com/margince/margince/backend/internal/platform/owndomain"
)

// mailCopy uses a known recipient's saved locale, then the installation language.
// Public reset requests resolve this only after minting a valid account token.
func (h Handlers) mailCopy(ctx context.Context, recipient ...string) mailcopy.Copy {
	language := string(mailcopy.Fallback)
	err := h.svc.db.Tx(ctx, func(tx pgx.Tx) error {
		stored, err := LanguageOf(ctx, tx)
		if err != nil {
			return err
		}
		if len(recipient) > 0 && recipient[0] != "" {
			var chosen *string
			args := []any{mailidentity.FoldAddress(recipient[0]), owndomain.OfAddress(recipient[0])}
			err := tx.QueryRow(ctx, "SELECT locale FROM app_user u WHERE "+
				storekit.SeatLoginIsSQL("u", "$1", "$2")+" AND "+ActivatableMemberSQL("u"), args...).Scan(&chosen)
			if err != nil && !errors.Is(err, pgx.ErrNoRows) {
				return err
			}
			if chosen != nil && mailcopy.Known(*chosen) {
				stored = *chosen
			}
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
