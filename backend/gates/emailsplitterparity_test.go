// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

// The server composes a row's preview and the browser folds the quoted tail in
// the drawer, from two copies of one vocabulary. If they drift, a row previews
// one thing and the message it opens says another — and it reads as a rendering
// bug, so it gets debugged on the side that is not wrong.
//
// The two tables were checked against each other by hand on twelve bodies. That
// proved they agreed at that moment and nothing about the next edit: a sign-off
// added to one side is silent on the other until somebody notices a preview
// ending mid-sentence.
//
// So this reads BOTH files and compares five vocabularies plus the four single
// patterns beside them, failing in either direction. It is shaped after
// frontendminorunits_test.go, including the lesson that cost that gate a
// rewrite: strip comments before parsing, and fail loudly when a side parses
// EMPTY rather than agreeing with a file it could not read.

import (
	"maps"
	"slices"
	"sort"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

const (
	goSplitter = "internal/modules/activities/emailtext.go"
	tsSplitter = "../frontend/src/format/emailtext.ts"
)

// splitterTables pairs the two sides' names for one vocabulary. The Go name is
// the declaration this gate reads out of the AST; the TypeScript name is the
// `const` it reads out of the source text.
//
// Adding a vocabulary to one side and not to this list is the one thing this
// gate cannot see, which is why the count below is asserted rather than left to
// the loop: a list that shrank would report PASS over a table nobody compares.
var splitterTables = []struct {
	goName, tsName string
	// kind says how the entries are compared. A word list is compared as text;
	// a pattern list is compared after the two dialects' case-insensitivity
	// spellings are reduced to the same pair.
	patterns bool
}{
	{goName: "signOffPrefixes", tsName: "SIGN_OFF_PREFIXES"},
	{goName: "signOffExact", tsName: "SIGN_OFF_EXACT"},
	{goName: "signOffLines", tsName: "SIGN_OFF_LINES", patterns: true},
	{goName: "attributionPatterns", tsName: "ATTRIBUTION", patterns: true},
	// The four single patterns. Each is one entry rather than a list, and they
	// are compared the same way — a preamble the server peels and the browser
	// does not is the same class of drift as a missing sign-off.
	{goName: "preamblePattern", tsName: "PREAMBLE", patterns: true},
	{goName: "signatureDelimiter", tsName: "SIGNATURE_DELIMITER", patterns: true},
	{goName: "replyHeaderFrom", tsName: "REPLY_HEADER_FROM", patterns: true},
	{goName: "replyHeaderSent", tsName: "REPLY_HEADER_SENT", patterns: true},
}

func TestTheTwoEmailSplittersReadTheSameVocabulary(t *testing.T) {
	t.Parallel()
	inGo := goSplitterTables(t)
	inTS := tsSplitterTables(t)

	assertEverySplitterDeclarationIsPaired(t, inGo, inTS)
	for _, table := range splitterTables {
		goSide, ok := inGo[table.goName]
		if !ok {
			t.Errorf("%s declares no %s — this gate is reading a shape that is gone, and a "+
				"vocabulary it cannot find is one it agrees with", goSplitter, table.goName)
			continue
		}
		tsSide, ok := inTS[table.tsName]
		if !ok {
			t.Errorf("%s declares no %s — this gate is reading a shape that is gone, and a "+
				"vocabulary it cannot find is one it agrees with", tsSplitter, table.tsName)
			continue
		}
		if len(goSide) == 0 || len(tsSide) == 0 {
			t.Errorf("%s/%s parsed %d and %d entries; a side that reads empty agrees with everything",
				table.goName, table.tsName, len(goSide), len(tsSide))
			continue
		}
		compareVocabularies(t, table.goName, table.tsName, goSide, tsSide)
	}

	// The window is a number rather than a list, so it is compared on its own.
	// A server scanning fifteen lines up against a browser scanning ten folds a
	// message on one surface and prints the sign-off on the other.
	goWindow := goSplitterConst(t, "trailingScanLines")
	tsWindow := tsSplitterConst(t, "TRAILING_SCAN_LINES")
	if goWindow != tsWindow {
		t.Errorf("the trailing scan window is %d lines in Go and %d in TypeScript — one side folds "+
			"a sign-off the other prints", goWindow, tsWindow)
	}
}

// compareVocabularies reports what each side holds and the other does not, in
// both directions, naming the entry rather than the count.
func compareVocabularies(t *testing.T, goName, tsName string, goSide, tsSide []string) {
	t.Helper()
	inTS := map[string]bool{}
	for _, entry := range tsSide {
		inTS[entry] = true
	}
	inGo := map[string]bool{}
	for _, entry := range goSide {
		inGo[entry] = true
	}
	for _, entry := range sortedEntries(goSide) {
		if !inTS[entry] {
			t.Errorf("%s holds %q and %s does not: the server folds it and the browser prints it",
				goName, entry, tsName)
		}
	}
	for _, entry := range sortedEntries(tsSide) {
		if !inGo[entry] {
			t.Errorf("%s holds %q and %s does not: the browser folds it and the server prints it",
				tsName, entry, goName)
		}
	}
}

// assertEverySplitterDeclarationIsPaired closes the direction the pair list
// cannot see on its own.
//
// Comparing splitterTables against a count catches an entry DELETED from it. It
// cannot catch one that was never added: a vocabulary declared on one side and
// paired with nothing is a table this gate simply does not read, and a gate that
// does not read a table agrees with it — which is the whole failure this file
// exists to prevent, one level up.
//
// So the population is the parsed declarations themselves, and every one of them
// is either paired here or ratified below with the reason it is not a
// vocabulary at all.
func assertEverySplitterDeclarationIsPaired(t *testing.T, inGo, inTS map[string][]string) {
	t.Helper()
	paired := map[string]bool{}
	for _, table := range splitterTables {
		paired[table.goName] = true
		paired[table.tsName] = true
	}
	for side, declarations := range map[string]map[string][]string{goSplitter: inGo, tsSplitter: inTS} {
		for _, name := range sortedEntries(slices.Collect(maps.Keys(declarations))) {
			if paired[name] || notASharedVocabulary.Waived(t, name) {
				continue
			}
			t.Errorf("%s declares %s and splitterTables pairs it with nothing, so this gate never reads it. "+
				"Pair it with the other side's name, or say in notASharedVocabulary why it is not a shared "+
				"vocabulary — an unpaired table is one the two splitters may already disagree about",
				side, name)
		}
	}
	notASharedVocabulary.AssertAllMatched(t)
}

// notASharedVocabulary names the declarations the readers can parse that are NOT
// half of a mirrored pair, each with the reason. Ratified rather than filtered
// out in the readers: a reader that skipped them would decide for itself what
// counts as a vocabulary, and this is the file where that judgement belongs.
var notASharedVocabulary = gatekit.Waive(map[string]string{
	"collapseWhitespace": "the one-line collapse for a row preview, and the browser spells it inline in " +
		"emailSummaryText's own replace rather than as a named const — there is no second declaration to " +
		"pair it with, and the behaviour it drives is held by the summary tests on both sides",
})

func sortedEntries(entries []string) []string {
	out := append([]string(nil), entries...)
	sort.Strings(out)
	return out
}
