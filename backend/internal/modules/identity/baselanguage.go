// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/settings"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// BaseLanguage is the language AI writes in when what it writes is read by the
// whole team rather than by one contact.
//
// A model asked nothing about language answers in whatever language its input
// happened to be in, so a Vietnamese thread produced a Vietnamese claim on a
// record a German colleague then had to read. The installation names one
// language for that shared writing, the way it names one currency for money.
//
// It does NOT govern everything a model writes. Correspondence keeps the
// language of the correspondence — a German thread gets a German reply however
// this is set — and a brief cached for one reader keeps that reader's language.
// This is the language of the shared record.
//
// No freeze, unlike BaseCurrency. Changing it re-means nothing already stored:
// old artifacts stay in the language they were written in, and nothing converts
// against the answer the way money does.
var BaseLanguage = settings.Define[string](
	"installation.base_language",
	installationSettingsObject,
	"update",
	string(textlang.English),
	func(v string) error {
		if !textlang.Known(v) {
			return fmt.Errorf("a base language is one of en, de, vi")
		}
		return nil
	},
).AsInstallationIdentity().MachineryApplied()

// BaseLanguageOf resolves the language shared AI writing is written in, inside
// a transaction the caller already holds.
//
// GetTx rather than RequireTx, which is the opposite choice from the three
// above, and the reason is the upgrade rather than the value: every
// installation bootstrapped before this setting existed has no row for it. The
// three others are seeded together at bootstrap, so an absent row there means a
// broken installation and refusing is right. Here an absent row means an older
// one, and a brief that refuses to generate because nobody has named a language
// is worse than one that comes out in English — which is what those
// installations get today anyway.
func BaseLanguageOf(ctx context.Context, tx pgx.Tx) (string, error) {
	return settings.GetTx(ctx, tx, BaseLanguage)
}

// BaseLanguageForPrompt resolves the base language for a caller that holds a
// POOL rather than a transaction, opening the workspace transaction itself.
//
// It sits beside BaseLanguageOf rather than in either caller because both a
// compose engine and the deal-status service need exactly this, and the six
// lines are identical either way — two copies of one settings read is how one
// question comes to have two answers that drift.
//
// It never fails the caller. A prompt or a shared-record sentence is being
// built, and the language is the least important thing in it: refusing to
// extract a meeting's next steps because a settings read timed out trades a
// whole feature for a formatting preference. On any error the answer is
// English, which is what these prompts produced before the setting existed.
//
// The failure IS logged, and it has to be: this returns a string and nothing
// else, so a caller has no way to notice a degraded resolve and say so itself.
// A missing row does NOT reach that line — BaseLanguageOf answers the
// registered default for one — so a log here always means something actually
// went wrong.
func BaseLanguageForPrompt(ctx context.Context, pool *pgxpool.Pool) string {
	lang := string(textlang.English)
	err := database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error {
		resolved, err := BaseLanguageOf(ctx, tx)
		if err != nil {
			return err
		}
		lang = resolved
		return nil
	})
	if err != nil {
		slog.WarnContext(ctx, "the installation's base language could not be read; English is used instead",
			"reason", err)
		return string(textlang.English)
	}
	return lang
}

// BaseLanguageForRecord resolves the base language for a caller that holds a
// TRANSACTION rather than a pool — BaseLanguageForPrompt's sibling for that
// case, for the same never-fail reason: refusing to file a real observation
// about an account, or withhold a coverage finding, because a settings read
// failed trades a fact for a formatting preference. On any error the answer
// is English.
//
// The failure IS logged, and it has to be: this returns a language and
// nothing else, so a caller has no way to notice a degraded resolve and say
// so itself.
//
// BaseLanguageOf keeps its error return and its callers — a write inside the
// same transaction that must not proceed on a bad read still wants one. This
// is for the read whose only job is choosing a wording for shared-record
// text: a summary, a finding, a note, stored once and read by everyone who
// can see it.
func BaseLanguageForRecord(ctx context.Context, tx pgx.Tx) textlang.Lang {
	lang, err := BaseLanguageOf(ctx, tx)
	if err != nil {
		slog.WarnContext(ctx, "the installation's base language could not be read; English is used instead",
			"reason", err)
		return textlang.English
	}
	return textlang.Lang(lang)
}
