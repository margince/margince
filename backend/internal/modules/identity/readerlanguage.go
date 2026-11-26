// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
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
// principal from sign-in. An agent on a passport reads its granting human's
// choice. A system caller and a member who never chose have none.
func chosenLanguage(ctx context.Context) (textlang.Lang, bool) {
	if actor, ok := principal.Actor(ctx); ok && actor.HumanOrItsAgent() && (systemtext.HasLocale(ctx) || textlang.Known(actor.Locale)) {
		return systemtext.Locale(ctx), true
	}
	return textlang.Unknown, false
}

// readingLanguage is the language actor reads where no screen chose one: the
// saved choice of its human, else the installation's base language. That is
// the fallback ReaderLanguageForRecord gives a signed-in human. A saved choice
// binds every actor, a passportless agent included.
func readingLanguage(ctx context.Context, tx pgx.Tx, actor principal.Principal, saved *string) textlang.Lang {
	if saved != nil && textlang.Known(*saved) {
		return textlang.Lang(*saved)
	}
	return BaseLanguageForRecord(principal.WithActor(ctx, actor), tx)
}

// ReadingLanguage is ReadingLanguageTx in a transaction of its own.
func (s *Service) ReadingLanguage(ctx context.Context, actor principal.Principal) (textlang.Lang, error) {
	var lang textlang.Lang
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		var err error
		lang, err = ReadingLanguageTx(ctx, tx, actor)
		return err
	})
	return lang, err
}

// ReadingLanguageTx is readingLanguage for an actor rebuilt outside a request,
// read from the saved choice of actor.UserID.
func ReadingLanguageTx(ctx context.Context, tx pgx.Tx, actor principal.Principal) (textlang.Lang, error) {
	var saved *string
	if err := tx.QueryRow(ctx, `SELECT locale FROM app_user WHERE id = $1`, actor.UserID).Scan(&saved); err != nil {
		return textlang.Unknown, err
	}
	return readingLanguage(ctx, tx, actor, saved), nil
}
