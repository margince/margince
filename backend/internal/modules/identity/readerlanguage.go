// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"context"
	"errors"
	"log/slog"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// ReaderLanguageForRecord is the language for a sentence only the caller reads
// and nothing stores: their chosen display language, else the base language.
//
// Text stored once for the whole team keeps BaseLanguageForRecord. A system
// caller has no display language and gets the base language too.
//
// It never fails the caller. A failed read is logged and falls back to the
// base language, because a wording must not turn into a refusal.
func ReaderLanguageForRecord(ctx context.Context, tx pgx.Tx) textlang.Lang {
	human := actingHuman(ctx)
	if human.IsZero() {
		return BaseLanguageForRecord(ctx, tx)
	}
	var chosen *string
	err := tx.QueryRow(ctx, `SELECT locale FROM app_user WHERE id = $1`, human).Scan(&chosen)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		slog.WarnContext(ctx, "the reader's display language could not be read; the installation's base language is used instead",
			"reason", err)
	}
	if err == nil && chosen != nil && textlang.Known(*chosen) {
		return textlang.Lang(*chosen)
	}
	return BaseLanguageForRecord(ctx, tx)
}
