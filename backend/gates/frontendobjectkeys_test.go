// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gates

// Reading a TypeScript object literal's own keys, for the gates that hold a
// frontend word map to the server vocabulary it words.

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// tsObjectEntries reads the object literal opened after marker in path, as each
// top-level key and the text of its value. A spread is kept under "..." plus
// the spread name.
func tsObjectEntries(t *testing.T, path, marker string) map[string]string {
	t.Helper()
	source, err := gatekit.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	text := tsWithoutComments(string(source))
	at := indexAfter(text, marker)
	if at < 0 {
		t.Fatalf("%s no longer declares %q, the map this gate reads", path, marker)
	}
	open := strings.IndexByte(text[at:], '{')
	if open < 0 {
		t.Fatalf("%s: %q is not followed by an object literal", path, marker)
	}
	body, ok := tsBalanced(text[at+open:])
	if !ok {
		t.Fatalf("%s: the literal after %q is unterminated", path, marker)
	}
	entries := tsLiteralEntries(body)
	if len(entries) == 0 {
		t.Fatalf("%s: read no entries after %q, and a map read as empty agrees with everything", path, marker)
	}
	return entries
}

// tsWithoutComments blanks each comment outside a string, so a `//` inside a
// value stays part of the value.
func tsWithoutComments(text string) string {
	out := []byte(text)
	var quote byte
	for i := 0; i < len(out); i++ {
		c := out[i]
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
		case c == '/' && i+1 < len(out) && out[i+1] == '/':
			for ; i < len(out) && out[i] != '\n'; i++ {
				out[i] = ' '
			}
		case c == '/' && i+1 < len(out) && out[i+1] == '*':
			end := strings.Index(string(out[i+2:]), "*/")
			if end < 0 {
				end = len(out) - i - 4
			}
			for j := i; j < i+end+4; j++ {
				if out[j] != '\n' {
					out[j] = ' '
				}
			}
			i += end + 3
		}
	}
	return string(out)
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
		case c == ',' && depth == 0:
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
