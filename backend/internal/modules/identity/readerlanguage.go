// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"context"

	"github.com/jackc/pgx/v5"

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
	if actor, ok := principal.Actor(ctx); ok && textlang.Known(actor.Locale) {
		return textlang.Lang(actor.Locale)
	}
	return BaseLanguageForRecord(ctx, tx)
}
