// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package systemtext

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

const quotedSubject = `Re: „Angebot“ "Q4"`

func TestAQuotedArgumentShowsTheLanguagesMarksAndNoBackslash(t *testing.T) {
	component := Sentence("approval.heldNotSent", Text(quotedSubject), MessageArgument("approval.heldSenderInactive"))
	for lang, open := range map[textlang.Lang]string{textlang.English: "“", textlang.German: "„", textlang.Vietnamese: "“"} {
		rendered, err := component.Render(lang)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(rendered, open+quotedSubject) || strings.Contains(rendered, `\`) {
			t.Errorf("%s rendered %s", lang, rendered)
		}
	}
}

func TestTextStoredWithGoQuotingStillBindsAndIsShownInTheReadersLanguage(t *testing.T) {
	stored := fmt.Sprintf("%q was not sent: %s", quotedSubject, Copy("approval.heldSenderInactive", textlang.English))
	message := ForWrite(context.Background(), &Message{Subject: Sentence("approval.heldNotSent",
		Text(quotedSubject), MessageArgument("approval.heldSenderInactive"))}, stored, "")
	if message == nil || message.Subject == nil {
		t.Fatal("text stored with Go quoting no longer binds to its template")
	}
	shown := message.SubjectIn(stored, textlang.German)
	if !strings.HasPrefix(shown, "„"+quotedSubject+"“") {
		t.Fatalf("a German reader got %s", shown)
	}
}

// Each code the old spelling is kept for still quotes a value, so the list
// cannot keep a code that no longer needs it.
func TestEveryGoQuotedCodeStillQuotesAValue(t *testing.T) {
	for code := range goQuotedCodes {
		definition, ok := definitionOf(code)
		if !ok {
			t.Errorf("%s is kept for Go quoting but is not in the catalog", code)
			continue
		}
		for _, lang := range textlang.Shipped {
			if goQuoting.Replace(definition.Words.In(lang)) == definition.Words.In(lang) {
				t.Errorf("%s in %s quotes no value, so it has no old spelling to keep", code, lang)
			}
		}
	}
}
