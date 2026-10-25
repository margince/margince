// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package company360

import (
	"reflect"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/langcopy/langcopytest"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

var englishSuggestions = suggestionCopyFor(textlang.English)

func TestSuggestionLanguagesCoverEveryPhrase(t *testing.T) {
	langcopytest.Census(t, suggestionWords)
}

func TestSuggestionLanguagePreservesEvidenceAndDismissalIdentity(t *testing.T) {
	company, deal := testCompanyID(t), ids.NewV7()
	in := suggestionInputs{
		timeline: true, loc: time.UTC, pipeline: true, hasNewest: true,
		newest: sentAgo(10, crmcontracts.ActivityDirectionOutbound), companyName: "Acme",
		contractEnded: true, contractEndedSaid: "Original contract words.", lifecycle: "customer",
		open: pipeline{OpenCount: 1, OpenDigest: "one", Open: []openDeal{{ID: deal, Name: "Expansion"}}, Stalled: []stalledDeal{{ID: deal, Name: "Expansion"}}},
	}
	original := candidateSuggestions(company, suggestNow, in)
	if len(original) != 4 {
		t.Fatalf("fixture produced %d suggestion kinds", len(original))
	}
	for _, lang := range textlang.Shipped {
		in.language = lang
		translated := candidateSuggestions(company, suggestNow, in)
		if len(translated) != len(original) {
			t.Fatal("language changed the advice")
		}
		for i, row := range translated {
			before := original[i]
			if row.Fingerprint != before.Fingerprint {
				t.Fatalf("%s changed dismissal identity", lang)
			}
			for j := range row.Evidence {
				if lang != textlang.English && row.Evidence[j].Origin != nil && before.Evidence[j].Origin != nil && *row.Evidence[j].Origin == *before.Evidence[j].Origin {
					t.Fatalf("%s left receipt origin in English", lang)
				}
				row.Evidence[j].Origin = before.Evidence[j].Origin
			}
			if !reflect.DeepEqual(row.Evidence, before.Evidence) {
				t.Fatalf("%s changed suggestion identity or evidence", lang)
			}
			if lang != textlang.English && (row.Reason == before.Reason || row.Title == nil || *row.Title == *before.Title) {
				t.Fatalf("%s left %s in English", lang, row.Kind)
			}
			if row.Action != nil && row.Action.Task != nil && *row.Title != row.Action.Task.Subject {
				t.Fatal("translated ask differs from the task it creates")
			}
		}
	}
}
