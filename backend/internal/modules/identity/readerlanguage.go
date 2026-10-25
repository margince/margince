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

// ReaderLanguageForRecord is the language a sentence is written in when the
// caller alone reads it and nothing stores it: the caller's chosen display
// language, else the installation's base language.
//
// Text that is stored once for the whole team keeps BaseLanguageForRecord. A
// system caller has no display language and gets the base language too.
//
// Like BaseLanguageForRecord it never fails the caller: a wording must not turn
// into a refusal, so a failed read is logged and falls back to the base language.
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
