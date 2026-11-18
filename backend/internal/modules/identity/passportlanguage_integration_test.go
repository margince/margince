// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package identity

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

func TestAnAgentReadsSystemTextInItsGrantingHumansLanguage(t *testing.T) {
	for _, chosen := range []textlang.Lang{textlang.German, textlang.Vietnamese} {
		t.Run(string(chosen), func(t *testing.T) {
			e := setupRevocationEnv(t, "passport-language-"+string(chosen))
			human := e.wsCtx(e.member)
			issued, err := e.svc.IssuePassport(human, e.member, IssuePassportInput{Scopes: []string{"read"}})
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
