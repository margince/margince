// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The one way a compose engine holding a pool — rather than a transaction —
// resolves the installation's base language for a prompt.
//
// identity.BaseLanguageOf is the accessor and stays the only reader of the
// setting; this opens the workspace transaction it needs. It exists because
// three background engines (signal extraction, transcript proposals, site
// triage) build their prompts from a pool and would otherwise each grow their
// own copy of the same six lines, which is how one question ends up with three
// answers that drift.

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/gradionhq/margince/backend/internal/modules/identity"
	"github.com/gradionhq/margince/backend/internal/platform/database"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/textlang"
)

// BaseLanguageForPrompt resolves the installation's base language for a prompt.
//
// It never fails the caller. A prompt is being built, and the language is the
// least important thing in it: refusing to extract a meeting's next steps
// because a settings read timed out trades a whole feature for a formatting
// preference. On any error the answer is English, which is what these prompts
// produced before the setting existed — the degradation is visible in the
// output rather than silent, since a German installation suddenly reading
// English answers is exactly the complaint that gets reported.
//
// Exported so the integration lane can prove the whole chain — settings row,
// workspace transaction, accessor — against a real database, through the same
// function production calls. A test that re-implemented these six lines would
// prove its own copy works.
//
// The error is deliberately not logged here. Every caller has its own logger
// and its own idea of what a degraded pass is worth saying; a helper that logs
// on their behalf writes a line none of them chose.
func BaseLanguageForPrompt(ctx context.Context, pool *pgxpool.Pool) string {
	lang := string(textlang.English)
	err := database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error {
		resolved, err := identity.BaseLanguageOf(ctx, tx)
		if err != nil {
			return err
		}
		lang = resolved
		return nil
	})
	if err != nil {
		return string(textlang.English)
	}
	return lang
}
