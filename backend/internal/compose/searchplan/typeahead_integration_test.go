// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package searchplan

import (
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/search"
)

// A typeahead search ranks messages only once the word being typed has four
// letters; every other type is found from the first letter. The messages carry
// the short words whole, because a message matches the fragment as a whole word.
func TestTypeaheadLeavesMessagesOutUntilTheLastWordHasFourLetters(t *testing.T) {
	e := integration.Setup(t)
	e.SeedCompany(t, "Abc Corp", nil)
	contact := e.SeedContact(t, "Kick Starter", nil)
	for _, subject := range []string{"Abc ki kic kick notes", "Über ube Protokoll"} {
		if _, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
			Kind: "note", Subject: new(subject), Source: "manual",
			Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: contact}},
		}); err != nil {
			t.Fatal(err)
		}
	}
	perThree := 3
	types := func(query string, typeahead bool, perType *int) []string {
		page, err := search.NewStore(e.DB()).Search(e.Admin(), search.Input{Query: query, Typeahead: typeahead, PerType: perType})
		if err != nil {
			t.Fatalf("search %q: %v", query, err)
		}
		var out []string
		for _, hit := range page.Hits {
			if !slices.Contains(out, hit.Type) {
				out = append(out, hit.Type)
			}
		}
		return out
	}

	cases := []struct {
		query       string
		typeahead   bool
		wantMail    bool
		wantNonMail string
		explanation string
	}{
		{"abc", true, false, "company", "a 3-letter company name is still found"},
		{"abc", false, true, "company", "without typeahead the results page ranks mail from the first letter"},
		{"ki", true, false, "contact", "a 2-letter fragment finds contacts only"},
		{"kic", true, false, "contact", "a 3-letter fragment finds contacts only"},
		{"kick", true, true, "contact", "a 4-letter fragment ranks mail"},
		{"abc kic", true, false, "", "a short last word after a finished one leaves mail out"},
		{"abc kick", true, true, "", "a long last word after a finished one ranks mail"},
		{"abc ", true, true, "", "a finished short word is no fragment"},
		{"ube", true, false, "", "3 letters with a diacritic count as 3, not by bytes"},
		{"uber", true, true, "", "4 letters with a diacritic rank mail"},
	}
	for _, c := range cases {
		for _, per := range []*int{nil, &perThree} {
			got := types(c.query, c.typeahead, per)
			if has := slices.Contains(got, "activity"); has != c.wantMail {
				t.Errorf("%s: query %q typeahead=%v grouped=%v returned %v, mail present = %v want %v",
					c.explanation, c.query, c.typeahead, per != nil, got, has, c.wantMail)
			}
			if c.wantNonMail != "" && !slices.Contains(got, c.wantNonMail) {
				t.Errorf("%s: query %q returned %v, want a %s hit", c.explanation, c.query, got, c.wantNonMail)
			}
		}
	}
}
