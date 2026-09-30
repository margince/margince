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

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

func TestEveryShippedLanguageLabelsTheDossier(t *testing.T) {
	if len(dossierLabels) == 0 {
		t.Fatal("the label table is empty; this census would certify nothing")
	}
	for field, p := range dossierLabels {
		for _, lang := range textlang.Shipped {
			if strings.TrimSpace(p.in(lang)) == "" {
				t.Errorf("%s has no label for %s, and fieldSentence SKIPS a field it cannot "+
					"label — the dossier silently carries one statement fewer in that language",
					lang, field)
			}
		}
	}
}

func TestAnUnshippedLanguageFallsBackToTheEnglishDossierLabels(t *testing.T) {
	field := crmcontracts.CompanyProfileFieldFieldIcp
	got, ok := labelFor(field, "kl")
	if !ok {
		t.Fatalf("%s is not labelled at all", field)
	}
	if want := dossierLabels[field].in(textlang.English); got != want {
		t.Fatalf("an unshipped language answered %q, want the English %q", got, want)
	}
}
