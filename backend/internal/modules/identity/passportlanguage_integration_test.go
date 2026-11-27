// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package identity

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

func TestAnAgentReadsSystemTextInItsGrantingHumansLanguage(t *testing.T) {
	t.Parallel()
	for _, chosen := range []textlang.Lang{textlang.German, textlang.Vietnamese} {
		t.Run(string(chosen), func(t *testing.T) {
			e := setupRevocationEnv(t, "passport-language-"+string(chosen))
			human := e.wsCtx(e.member)
			issued, err := e.svc.IssuePassport(human, e.member, IssuePassportInput{Label: new(testPassportLabel), Scopes: []string{"read"}})
			if err != nil {
				t.Fatalf("minting a passport: %v", err)
			}
			if _, err := e.svc.SaveMyLocale(human, string(chosen)); err != nil {
				t.Fatalf("saving the granting human's language: %v", err)
			}

			byToken, err := e.svc.AuthenticateAgent(human, issued.Token)
			if err != nil {
				t.Fatalf("authenticating by token: %v", err)
			}
			byID, err := e.svc.AuthenticateAgentByID(human, issued.ID)
			if err != nil {
				t.Fatalf("resolving by passport id: %v", err)
			}
			for door, agent := range map[string]AgentIdentity{"token": byToken, "passport id": byID} {
				ctx := principal.WithActor(human, agent.Principal())
				if got := systemtext.Locale(ctx); got != chosen {
					t.Errorf("an agent authenticated by %s reads system text in %q, want %q", door, got, chosen)
				}
				if got := ReaderLanguageForPrompt(ctx, e.pool); got != string(chosen) {
					t.Errorf("an agent authenticated by %s is answered in %q, want %q", door, got, chosen)
				}
			}
		})
	}
}

func TestAnAgentWhoseHumanNeverChoseReadsTheInstallationsBaseLanguage(t *testing.T) {
	t.Parallel()
	e := setupRevocationEnv(t, "passport-language-base")
	setBaseLanguage(t, e, textlang.German)
	t.Cleanup(func() { setBaseLanguage(t, e, textlang.English) })
	// A role lets the settings gate admit the agent's read of the base language.
	if err := e.svc.ChangeUserRole(e.wsCtx(e.admin), e.admin, e.member.UserID, "rep"); err != nil {
		t.Fatalf("giving the member a role: %v", err)
	}
	human := e.wsCtx(e.member)
	issued, err := e.svc.IssuePassport(human, e.member, IssuePassportInput{Label: new(testPassportLabel), Scopes: []string{"read"}})
	if err != nil {
		t.Fatalf("minting a passport: %v", err)
	}
	agent, err := e.svc.AuthenticateAgent(human, issued.Token)
	if err != nil {
		t.Fatalf("authenticating: %v", err)
	}
	if got := systemtext.Locale(principal.WithActor(human, agent.Principal())); got != textlang.German {
		t.Errorf("an agent whose human never chose reads system text in %q, want the base language de", got)
	}
}

func setBaseLanguage(t *testing.T, e *revocationEnv, lang textlang.Lang) {
	t.Helper()
	if err := e.svc.db.Tx(e.wsCtx(e.admin), func(tx pgx.Tx) error {
		if _, err := tx.Exec(context.Background(), `DELETE FROM setting WHERE key = $1`, BaseLanguage.Key()); err != nil {
			return err
		}
		_, err := config.SeedTx(context.Background(), tx, nil, BaseLanguage, string(lang))
		return err
	}); err != nil {
		t.Fatalf("setting the base language to %s: %v", lang, err)
	}
}

func TestAFailedBaseLanguageReadLeavesTheCallersTransactionUsable(t *testing.T) {
	t.Parallel()
	e := setupRevocationEnv(t, "passport-language-savepoint")
	ctx := e.wsCtx(e.admin)
	if err := e.svc.db.Tx(ctx, func(tx pgx.Tx) error {
		// With only pg_catalog on the path, the settings table cannot be found.
		if _, err := tx.Exec(ctx, `SET LOCAL search_path = pg_catalog`); err != nil {
			return err
		}
		if got := BaseLanguageForRecord(ctx, tx); got != textlang.English {
			t.Errorf("a failed read answers %q, want English", got)
		}
		var one int
		return tx.QueryRow(ctx, `SELECT 1`).Scan(&one)
	}); err != nil {
		t.Fatalf("the caller's transaction broke after a failed base-language read: %v", err)
	}
}
