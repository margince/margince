// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind falsification H2

package gates

// The word map reader reads each shape a frontend map is declared in, and
// fails one it cannot read. The gates that hold a frontend word map to the
// server vocabulary it words all read through it.

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// tsObjectEntries reads the object literal a declaration after marker in path
// is initialized with, as each top-level key and the text of its value. A
// spread is kept under "..." plus the spread name.
func tsObjectEntries(t *testing.T, path, marker string) map[string]string {
	t.Helper()
	source, err := gatekit.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	body, problem := tsDeclaredObject(string(source), marker)
	if problem != "" {
		t.Fatalf("%s: %s", path, problem)
	}
	entries := tsLiteralEntries(body)
	if len(entries) == 0 {
		t.Fatalf("%s: read no entries after %q, and a map read as empty agrees with everything", path, marker)
	}
	return entries
}

// tsDeclaredObject answers the body of the object literal that initializes the
// declaration after marker, or why it cannot. A type annotation, which may hold
// braces of its own, is skipped.
func tsDeclaredObject(source, marker string) (body, problem string) {
	text := gatekit.TSWithoutComments(source)
	at := indexAfter(text, marker)
	if at < 0 {
		return "", fmt.Sprintf("no longer declares %q, the map this gate reads", marker)
	}
	init := tsInitializer(text[at:])
	if init < 0 {
		return "", fmt.Sprintf("%q has no initializer this gate can find", marker)
	}
	rest := strings.TrimLeft(text[at+init:], " \t\r\n")
	if !strings.HasPrefix(rest, "{") {
		return "", fmt.Sprintf("%q is initialized with something other than an object literal, so its keys cannot be read", marker)
	}
	body, ok := tsBalanced(rest)
	if !ok {
		return "", fmt.Sprintf("the literal after %q is unterminated", marker)
	}
	return body, ""
}

// tsInitializer is the offset just past the `=` that opens a declaration's
// initializer, outside brackets, strings and arrows, or -1.
func tsInitializer(text string) int {
	depth := 0
	var quote byte
	for i := 0; i < len(text); i++ {
		c := text[i]
		switch {
		case quote != 0:
			switch c {
			case '\\':
				i++
			case quote:
				quote = 0
			}
		case c == '"' || c == '\'' || c == '`':
			quote = c
		case c == '=' && i+1 < len(text) && text[i+1] == '>':
			i++
		case c == '(' || c == '[' || c == '{' || c == '<':
			depth++
		case c == ')' || c == ']' || c == '}' || c == '>':
			depth--
		case c == '=' && depth == 0:
			return i + 1
		case c == ';' && depth == 0:
			return -1
		}
	}
	return -1
}

// tsBalanced answers the text between an opening brace and its match.
func tsBalanced(text string) (string, bool) {
	depth := 0
	var quote byte
	for i := 0; i < len(text); i++ {
		c := text[i]
		switch {
		case quote != 0:
			switch c {
			case '\\':
				i++
			case quote:
				quote = 0
			}
		case c == '"' || c == '\'' || c == '`':
			quote = c
		case c == '{' || c == '[' || c == '(':
			depth++
		case c == '}' || c == ']' || c == ')':
			depth--
			if depth == 0 {
				return text[1:i], true
			}
		}
	}
	return "", false
}

// tsLiteralEntries splits an object literal's body into its top-level entries.
func tsLiteralEntries(body string) map[string]string {
	entries := map[string]string{}
	for _, entry := range tsTopLevel(body) {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		if spread, ok := strings.CutPrefix(entry, "..."); ok {
			entries["..."+strings.TrimSpace(spread)] = ""
			continue
		}
		key, value, ok := tsCutKey(entry)
		if !ok {
			// A shorthand entry is a key whose value is the variable of that name.
			key, value = entry, entry
		}
		entries[key] = strings.TrimSpace(value)
	}
	return entries
}

// tsCutKey splits an entry at the colon after its key, so a quoted key may
// hold a colon of its own.
func tsCutKey(entry string) (key, value string, ok bool) {
	if quote := entry[0]; quote == '"' || quote == '\'' {
		end := strings.IndexByte(entry[1:], quote)
		if end < 0 {
			return "", "", false
		}
		rest := strings.TrimSpace(entry[end+2:])
		value, ok = strings.CutPrefix(rest, ":")
		return entry[1 : end+1], value, ok
	}
	key, value, ok = strings.Cut(entry, ":")
	return strings.TrimSpace(key), value, ok
}

// tsTopLevel splits text on the commas outside brackets and strings.
func tsTopLevel(text string) []string {
	return tsSplitTopLevel(text, ',')
}

// tsSplitTopLevel splits text on each sep outside brackets and strings.
func tsSplitTopLevel(text string, sep byte) []string {
	var parts []string
	depth, start := 0, 0
	var quote byte
	for i := 0; i < len(text); i++ {
		c := text[i]
		switch {
		case quote != 0:
			switch c {
			case '\\':
				i++
			case quote:
				quote = 0
			}
		case c == '"' || c == '\'' || c == '`':
			quote = c
		case c == '{' || c == '[' || c == '(':
			depth++
		case c == '}' || c == ']' || c == ')':
			depth--
		case c == sep && depth == 0:
			parts = append(parts, text[start:i])
			start = i + 1
		}
	}
	return append(parts, text[start:])
}

// tsKeySet is the keys of a word map. A spread hides keys this cannot read, so
// it fails.
func tsKeySet(t *testing.T, entries map[string]string) map[string]bool {
	t.Helper()
	keys, spreads := tsKeys(entries)
	for _, spread := range spreads {
		t.Errorf("a compared word map spreads %s, whose keys this gate does not read: write the entries out", spread)
	}
	return keys
}

// tsKeys splits a word map's entries into its keys and the names it spreads.
func tsKeys(entries map[string]string) (keys map[string]bool, spreads []string) {
	keys = map[string]bool{}
	for key := range entries {
		if spread, isSpread := strings.CutPrefix(key, "..."); isSpread {
			spreads = append(spreads, spread)
			continue
		}
		keys[key] = true
	}
	return keys, spreads
}

// wordsMatchVocabulary fails each Go value with no frontend key, and each
// frontend key no Go value has.
func wordsMatchVocabulary(t *testing.T, what, mapName string, inGo, inTS map[string]bool) {
	t.Helper()
	if len(inGo) == 0 {
		t.Fatalf("read no %s from the server, and a census that reads nothing agrees with everything", what)
	}
	for _, value := range sortedKeys(inGo) {
		if !inTS[value] {
			t.Errorf("the server has %s %q and %s has no words for it", what, value, mapName)
		}
	}
	for _, key := range sortedKeys(inTS) {
		if !inGo[key] {
			t.Errorf("%s words %s %q, which the server never has", mapName, what, key)
		}
	}
}

func TestTheWordMapReaderReadsEachKeyShape(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "words.ts")
	source := "export const WORDS: Readonly<\n  Record<string, (key: string) => string>\n> = {\n" +
		"  contact,\n" +
		"  \"a:b\": \"colon\",\n" +
		"  'quoted': 'single',\n" +
		"  link: \"https://example.test\", // a comment, not a key\n" +
		"  plain: 'x',\n" +
		"};\n"
	if err := os.WriteFile(path, []byte(source), 0o600); err != nil {
		t.Fatal(err)
	}
	entries := tsObjectEntries(t, path, "export const WORDS")
	keys, spreads := tsKeys(entries)
	if want := []string{"a:b", "contact", "link", "plain", "quoted"}; !slices.Equal(sortedKeys(keys), want) || len(spreads) != 0 {
		t.Errorf("the reader read keys %v and spreads %v, want %v and none", sortedKeys(keys), spreads, want)
	}
	if entries["link"] != `"https://example.test"` {
		t.Errorf("the reader read link as %q, so a // inside a string was taken for a comment", entries["link"])
	}
	_, spreads = tsKeys(tsLiteralEntries("...SHARED, own: 'x'"))
	if !slices.Equal(spreads, []string{"SHARED"}) {
		t.Errorf("the reader read spreads %v, want SHARED, which a compared map must fail on", spreads)
	}
}

func TestTheWordMapReaderRefusesAnInitializerThatIsNoLiteral(t *testing.T) {
	t.Parallel()
	body, problem := tsDeclaredObject("export const TYPED: { contact: MessageKey } = labels;\n", "export const TYPED")
	if problem == "" {
		t.Errorf("the reader read %q out of a type annotation, want it to refuse the initializer", body)
	}
}

func TestTheWordMapReaderFollowsAnImportByItsExportedName(t *testing.T) {
	t.Parallel()
	source := "import type { MessageKey } from \"../i18n/en\";\n" +
		"import { SOURCE_LABELS as SHARED_LABELS, other } from \"../lib/sources\";\n"
	cases := []struct{ local, exported, from string }{
		{"SHARED_LABELS", "SOURCE_LABELS", "../lib/sources"},
		{"other", "other", "../lib/sources"},
		{"MessageKey", "MessageKey", "../i18n/en"},
	}
	for _, tc := range cases {
		exported, from, ok := tsImportOf(source, tc.local)
		if !ok || exported != tc.exported || from != tc.from {
			t.Errorf("%s: read %q from %q (found %v), want %q from %q", tc.local, exported, from, ok, tc.exported, tc.from)
		}
	}
	if _, _, ok := tsImportOf(source, "SOURCE_LABELS"); ok {
		t.Error("SOURCE_LABELS is only the exported name, and this file binds no such local")
	}
}
