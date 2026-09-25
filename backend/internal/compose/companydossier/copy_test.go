// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package companydossier

// Every language the product ships labels every dossier field it can state.
//
// A label missing in one language is not a wrong sentence, it is an absent one:
// fieldSentence SKIPS a field it has no label for, so the dossier would quietly
// carry fewer statements in that language than in English.

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

func TestEveryShippedLanguageLabelsTheDossier(t *testing.T) {
	english, ok := dossierLabels[textlang.English]
	if !ok {
		t.Fatal("English is absent from the dossier's label table")
	}
	if len(english) == 0 {
		t.Fatal("the English label table is empty; this census would certify nothing")
	}
	for _, lang := range textlang.Shipped {
		labels, ok := dossierLabels[lang]
		if !ok {
			t.Errorf("%s ships in this product but labels no dossier field, so its floor states "+
				"nothing where English states fifteen fields", lang)
			continue
		}
		for field := range english {
			label, ok := labels[field]
			if !ok {
				t.Errorf("%s has no label for %s, and fieldSentence SKIPS a field it cannot "+
					"label — the dossier silently carries one statement fewer in that language",
					lang, field)
				continue
			}
			if strings.TrimSpace(label) == "" {
				t.Errorf("%s labels %s with blank text", lang, field)
			}
		}
		for field := range labels {
			if _, ok := english[field]; !ok {
				t.Errorf("%s labels %s, which English does not — a field labelled in one language "+
					"only is a statement that appears and disappears with the base language",
					lang, field)
			}
		}
	}
}

func TestAnUnshippedLanguageFallsBackToTheEnglishDossierLabels(t *testing.T) {
	if got := labelsFor("kl"); len(got) != len(dossierLabels[textlang.English]) {
		t.Fatalf("an unshipped language answered %d labels, want the English %d",
			len(got), len(dossierLabels[textlang.English]))
	}
}
