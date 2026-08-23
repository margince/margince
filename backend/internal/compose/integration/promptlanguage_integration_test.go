// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The base language reaches a real prompt.
//
// This is the half the static gate in backend/promptlanguage_test.go cannot
// prove. That gate reads syntax: it sees that a language rule is ATTACHED to a
// request, never that the right language was resolved, and a prompt could
// satisfy it while passing a variable that is always English. What decides the
// question is a settings row, a workspace transaction and an accessor, and the
// only honest way to check those is to write the row and read the prompt back.
//
// It goes through compose.BaseLanguageForPrompt rather than a hand-copied
// six-line settings read, because a test that supplies its own version of the
// production path proves nothing about production.

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/gradionhq/margince/backend/internal/compose"
	"github.com/gradionhq/margince/backend/internal/compose/promptlang"
	"github.com/gradionhq/margince/backend/internal/modules/identity"
	"github.com/gradionhq/margince/backend/internal/platform/database"
	"github.com/gradionhq/margince/backend/internal/platform/settings"
)

// setBaseLanguage writes the setting through settings.SeedValue, which is the
// writer bootstrap itself uses. It seeds rather than updates, so the existing
// row is cleared first — SeedValue's whole contract is that it does not
// overwrite, and a helper that ignored that would silently leave the old
// language in place and make every assertion below pass for the wrong reason.
func setBaseLanguage(ctx context.Context, t *testing.T, e *Env, code string) {
	t.Helper()
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(context.Background(),
			`DELETE FROM setting WHERE key = $1`, identity.BaseLanguage.Key()); err != nil {
			return err
		}
		stored, err := settings.SeedValue(context.Background(), tx, identity.BaseLanguage, code)
		if err != nil {
			return err
		}
		if !stored {
			t.Fatalf("the base language %q was discarded rather than stored, so the test below would judge the previous value", code)
		}
		return nil
	}); err != nil {
		t.Fatalf("setting the base language to %q: %v", code, err)
	}
}

// The whole chain, in one assertion: a row says German, and the rendered rule
// says German. Every link is real — the settings write, the workspace
// transaction, identity.BaseLanguageOf, and promptlang.Rule.
func TestTheBaseLanguageSetOnTheInstallationReachesThePrompt(t *testing.T) {
	e := Setup(t)
	ctx := e.Admin()

	setBaseLanguage(ctx, t, e, "de")

	rule := promptlang.Rule(compose.BaseLanguageForPrompt(ctx, e.Pool))
	if !strings.Contains(rule, "German") {
		t.Fatalf("the installation is set to German and the prompt rule does not say so:\n%s", rule)
	}

	// The other direction, and it is the one that catches a resolver that
	// happens to return a constant. A test asserting only "German appears"
	// passes against code that always says German.
	setBaseLanguage(ctx, t, e, "vi")

	rule = promptlang.Rule(compose.BaseLanguageForPrompt(ctx, e.Pool))
	if !strings.Contains(rule, "Vietnamese") {
		t.Fatalf("the installation was changed to Vietnamese and the prompt rule did not follow:\n%s", rule)
	}
	if strings.Contains(rule, "German") {
		t.Errorf("the prompt rule still names German after the setting moved to Vietnamese:\n%s", rule)
	}
}

// An installation bootstrapped before this setting existed has no row for it,
// and must still write prompts rather than refusing them. This is the reason
// BaseLanguageOf uses GetTx rather than RequireTx, and the reason is worth a
// test: the three settings beside it are seeded together at bootstrap, so the
// opposite choice looks like the consistent one right up until an upgraded
// installation stops generating briefs.
func TestAnInstallationThatNeverNamedALanguageStillGetsAPrompt(t *testing.T) {
	e := Setup(t)
	ctx := e.Admin()

	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(),
			`DELETE FROM setting WHERE key = $1`, identity.BaseLanguage.Key())
		return err
	}); err != nil {
		t.Fatalf("removing the base-language row: %v", err)
	}

	rule := promptlang.Rule(compose.BaseLanguageForPrompt(ctx, e.Pool))
	if !strings.Contains(rule, "English") {
		t.Fatalf("an installation with no base-language row did not fall back to English:\n%s", rule)
	}
}
