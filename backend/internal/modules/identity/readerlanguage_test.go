// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
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
