// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// A client-rendered site sends a loader, not its words. Judging that "no
// readable text" puts a real company on file as an empty address, with no model
// call made and nothing later to reopen it — the same failure the meta-refresh
// follow exists to prevent, arriving by another route.
//
// The worker is the zero value: it carries no brain, so a verdict that needed a
// model call would panic rather than pass quietly.
func TestAJavaScriptShellIsNotSettledAsParked(t *testing.T) {
	shell := crawlPage{Text: "", ExternalScripts: 3}
	verdict, err := (&siteDeepReadWorker{}).classifySeed(context.Background(), shell)
	if err != nil {
		t.Fatalf("classifying: %v", err)
	}
	if verdict.Aborts() {
		t.Errorf("verdict %+v settles the domain — a shell this reader cannot render is an unread page, not an empty one", verdict)
	}
}

// The guard's other half. A page with nothing on it and nothing to build it
// with is genuinely empty, and that answer still costs no model call.
func TestAnEmptyPageWithNoScriptsIsStillParked(t *testing.T) {
	empty := crawlPage{Text: "", ExternalScripts: 0}
	verdict, err := (&siteDeepReadWorker{}).classifySeed(context.Background(), empty)
	if err != nil {
		t.Fatalf("classifying: %v", err)
	}
	if !verdict.Aborts() {
		t.Errorf("verdict %+v does not settle a genuinely empty landing page", verdict)
	}
}

// A shell that DID declare what it is about has prose to judge, so it goes to
// the classifier like any other page rather than being settled deterministically.
func TestAShellThatDeclaresItselfIsHandedToTheClassifier(t *testing.T) {
	declared := crawlPage{Text: "", ExternalScripts: 2, HeadText: []string{"We build robots."}}
	if !strings.Contains(declared.prose(), "We build robots.") {
		t.Fatalf("prose() = %q, want the head declaration", declared.prose())
	}
	if declared.isJSShell() != true {
		t.Fatal("a scripted, textless page is a shell whatever its head declared")
	}
}

// What the model is shown: the page's own claim about itself, ahead of its
// body text. On a shell it is the only prose the server ever sent.
func TestTheModelIsShownWhatTheHeadDeclared(t *testing.T) {
	page := crawlPage{
		Text:     "Body words.",
		HeadText: []string{"A description.", "A title."},
	}
	want := "A description.\nA title.\n\nBody words."
	if got := page.prose(); got != want {
		t.Fatalf("prose() = %q, want %q", got, want)
	}
	// Text itself is untouched: evidence snippets are matched against it.
	if page.Text != "Body words." {
		t.Fatalf("Text = %q — prose() must not rewrite it", page.Text)
	}
}

// A page that declared nothing reads exactly as it did before.
func TestAPageWithNoHeadDeclarationReadsAsItsTextAlone(t *testing.T) {
	page := crawlPage{Text: "Body words."}
	if got := page.prose(); got != "Body words." {
		t.Fatalf("prose() = %q, want the text unchanged", got)
	}
}

// The dossier says what kind of read this was, so a thin result is not
// mistaken for a thin company.
func TestTheDossierSaysWhenOnlyAShellCouldBeRead(t *testing.T) {
	shell := []crawlPage{{Kind: crmcontracts.SiteReadPageKindHome, Text: "", ExternalScripts: 4}}
	warnings := readWarnings("", nil, seedIsJSShell(shell))
	if len(warnings) != 1 || !strings.Contains(warnings[0], "browser") {
		t.Fatalf("warnings = %v, want one naming that the site builds its pages in the browser", warnings)
	}
	ordinary := []crawlPage{{Kind: crmcontracts.SiteReadPageKindHome, Text: strings.Repeat("word ", 40)}}
	if got := readWarnings("", nil, seedIsJSShell(ordinary)); len(got) != 0 {
		t.Fatalf("warnings = %v, want none for a site that served its words", got)
	}
}
