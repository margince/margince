// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

func TestAReaderWhoChoseALanguageGetsItWithoutAQuery(t *testing.T) {
	ctx := principal.WithActor(context.Background(), principal.Principal{Type: principal.PrincipalHuman, Locale: "de"})
	if got := ReaderLanguageForPrompt(ctx, nil); got != "de" {
		t.Fatalf("a reader who chose de gets %q", got)
	}
}

func TestAReaderLanguageThatCannotBeReadFallsBackToEnglish(t *testing.T) {
	ctx := principal.WithActor(context.Background(), principal.Principal{Type: principal.PrincipalHuman, Locale: "fr"})
	if got := ReaderLanguageForPrompt(ctx, nil); got != "en" {
		t.Fatalf("a reader with no shipped language and no readable base gets %q, want en", got)
	}
}

func TestAnAgentOnAPassportReadsInItsGrantingHumansChosenLanguage(t *testing.T) {
	agent := AgentIdentity{PassportID: ids.PassportID{UUID: ids.NewV7()}, OnBehalfOf: ids.UserID{UUID: ids.NewV7()}, Locale: "vi"}
	ctx := principal.WithActor(context.Background(), agent.Principal())
	if got := systemtext.Locale(ctx); got != textlang.Vietnamese {
		t.Fatalf("an agent whose granting human chose vi reads system text in %q", got)
	}
	if got := ReaderLanguageForPrompt(ctx, nil); got != "vi" {
		t.Fatalf("an agent whose granting human chose vi is answered in %q", got)
	}
}
