// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// ReaderLanguageForRecord is the language for a sentence only the caller reads
// and nothing stores: their chosen display language, else the base language.
//
// The choice rides the principal from sign-in, so this costs no query when
// the caller chose one. Text stored once for the whole team keeps
// BaseLanguageForRecord, and so does a caller with no chosen language.
func ReaderLanguageForRecord(ctx context.Context, tx pgx.Tx) textlang.Lang {
	if chosen, ok := chosenLanguage(ctx); ok {
		return chosen
	}
	return BaseLanguageForRecord(ctx, tx)
}

// ReaderLanguageForPrompt is ReaderLanguageForRecord for a caller holding a
// pool. A model writes in it when its answer goes to the caller alone and
// nothing stores it. It never fails, for BaseLanguageForPrompt's reason.
func ReaderLanguageForPrompt(ctx context.Context, pool *pgxpool.Pool) string {
	if chosen, ok := chosenLanguage(ctx); ok {
		return string(chosen)
	}
	return BaseLanguageForPrompt(ctx, pool)
}

// chosenLanguage is the display language the caller chose, carried on their
// principal from sign-in. A system caller and a member who never chose have none.
func chosenLanguage(ctx context.Context) (textlang.Lang, bool) {
	if actor, ok := principal.Actor(ctx); ok && textlang.Known(actor.Locale) {
		return textlang.Lang(actor.Locale), true
	}
	return textlang.Unknown, false
}
