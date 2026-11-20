// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package search

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// writtenBody is a body Margince wrote, bound to the exact stored bytes.
func writtenBody(t *testing.T, stored string) storekit.SystemWords {
	t.Helper()
	body := systemtext.Sentence("automation.why.new_lead", systemtext.Text(strings.Repeat("Rena Quill ", 17)), systemtext.Date("2026-11-16"))
	body.Digest = systemtext.Fingerprint(stored)
	return storekit.SystemWords{Body: stored, Message: &systemtext.Message{Version: 1, Body: body}}
}

func TestASnippetMarginceWroteIsCutAndTrimmedInTheReadersLanguage(t *testing.T) {
	english, err := systemtext.Sentence("automation.why.new_lead",
		systemtext.Text(strings.Repeat("Rena Quill ", 17)), systemtext.Date("2026-11-16")).Render(textlang.English)
	if err != nil {
		t.Fatal(err)
	}
	stored := "\n  " + english
	activity := ids.NewV7()
	// The branch SQL cuts the body and the scan trims what it cut.
	hits := []Hit{{Type: hitTypeActivity, ID: activity, Snippet: strings.TrimSpace(string([]rune(stored)[:activitySnippetRunes]))}}
	nameHits(hits, map[ids.UUID]storekit.SystemWords{activity: writtenBody(t, stored)}, textlang.German)

	if !strings.HasPrefix(hits[0].Snippet, "Rena Quill") || strings.Contains(hits[0].Snippet, "came in") ||
		hits[0].Snippet != strings.TrimSpace(hits[0].Snippet) || len([]rune(hits[0].Snippet)) > activitySnippetRunes {
		t.Errorf("the German snippet is %q", hits[0].Snippet)
	}

	edited := []Hit{{Type: hitTypeActivity, ID: activity, Snippet: "Somebody rewrote this"}}
	nameHits(edited, map[ids.UUID]storekit.SystemWords{activity: writtenBody(t, stored)}, textlang.German)
	if edited[0].Snippet != "Somebody rewrote this" {
		t.Errorf("a snippet that is not the stored body was replaced with %q", edited[0].Snippet)
	}
}
