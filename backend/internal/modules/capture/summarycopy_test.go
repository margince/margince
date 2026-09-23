// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// Every shipped language writes the dedupe summary in its own words, with both
// holes of the natural key the English sentence names.
func TestEveryShippedLanguageWritesItsOwnDuplicateLeadSummary(t *testing.T) {
	english, ok := summaryByLang[textlang.English]
	if !ok {
		t.Fatal("English has no summary set, and it is the fallback for everything else")
	}
	for _, lang := range textlang.Shipped {
		t.Run(string(lang), func(t *testing.T) {
			said, ok := summaryByLang[lang]
			if !ok {
				t.Fatalf("the product ships %s and this table has not learned it", lang)
			}
			got := said.duplicateLead
			if got == "" {
				t.Fatal("duplicateLead is empty, and a blank summary reaches the inbox as nothing at all")
			}
			if strings.Count(got, "%s/%s") != 1 || strings.Count(got, "%") != 2 {
				t.Errorf("duplicateLead must carry the natural key as one %%s/%%s and no other verb: %q", got)
			}
			if lang != textlang.English && got == english.duplicateLead {
				t.Error("duplicateLead is the English sentence verbatim; the entry exists and the reader still gets English")
			}
		})
	}
}

// An unshipped language answers English rather than an empty set, because the
// language comes off a settings row an admin can edit by hand.
func TestAnUnknownLanguageStagesTheMergeInEnglish(t *testing.T) {
	unshipped := func(context.Context) textlang.Lang { return textlang.Lang("kl") }
	if got := summaryIn(context.Background(), unshipped); got != summaryByLang[textlang.English] {
		t.Errorf("an unshipped language answered %+v, want the English set", got)
	}
}

var collidingCapture = connector.NaturalKey{SourceSystem: "gmail", SourceID: "msg-42"}

// The merge summary is written in the language the injected resolver answers.
func TestADuplicateLeadSummaryFollowsTheInjectedLanguage(t *testing.T) {
	german := func(context.Context) textlang.Lang { return textlang.German }
	sink := NewSink(nil).WithBaseLanguage(german)

	want := "Der erfasste Datensatz gmail/msg-42 ist ein Duplikat eines bestehenden Leads"
	if got := sink.duplicateLeadSummary(context.Background(), collidingCapture); got != want {
		t.Errorf("summary = %q, want %q", got, want)
	}
}

// A sink composed without a resolver writes the English sentence it always
// wrote.
func TestADuplicateLeadSummaryWithoutAResolverIsEnglish(t *testing.T) {
	want := "Captured gmail/msg-42 duplicates an existing lead"
	if got := NewSink(nil).duplicateLeadSummary(context.Background(), collidingCapture); got != want {
		t.Errorf("summary = %q, want %q", got, want)
	}
}
