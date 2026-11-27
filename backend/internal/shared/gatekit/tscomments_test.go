// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gatekit_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// Each row is a line a census reads. kept must survive the strip, and gone
// must not: a comment that survives is read as code.
func TestTheTSStripperBlanksCommentsAndNothingElse(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, source string
		kept, gone   []string
	}{
		{
			name:   "a slash pair inside a string is the string",
			source: "const home = \"https://x.test\"; // page_cap\n",
			kept:   []string{`"https://x.test"`}, gone: []string{"page_cap"},
		},
		{
			// The case a quote-only lexer reads wrong: the quote in the regex
			// opened a string, and the comment after it survived.
			name:   "a quote inside a regex literal opens no string",
			source: "const quote = /\"/; // case 'comment_only':\n",
			kept:   []string{`/"/`}, gone: []string{"comment_only"},
		},
		{
			name:   "a slash in a character class does not close the regex",
			source: "const path = /[/\"]+/g; // gone_after_class\n",
			kept:   []string{`/[/"]+/g`}, gone: []string{"gone_after_class"},
		},
		{
			name:   "a slash after an operand divides",
			source: "const half = total / 2 / count; // gone_after_division\n",
			kept:   []string{"total / 2 / count"}, gone: []string{"gone_after_division"},
		},
		{
			name:   "a slash after a keyword starts a regex",
			source: "function f() { return /'/.test(s); } // gone_after_return\n",
			kept:   []string{"/'/.test(s)"}, gone: []string{"gone_after_return"},
		},
		{
			// An apostrophe in JSX text never closes, and TypeScript ends a
			// quote at its line's end, so the next line is code again.
			name:   "an unclosed quote ends at its line's end",
			source: "<p>Don't</p>\n// gone_next_line\nconst x = 1;\n",
			kept:   []string{"const x = 1;"}, gone: []string{"gone_next_line"},
		},
		{
			name:   "a template literal spans lines",
			source: "const t = `a\n// kept_in_template\n`; // gone_after_template\n",
			kept:   []string{"// kept_in_template"}, gone: []string{"gone_after_template"},
		},
		{
			name:   "a block comment keeps its newlines",
			source: "a /* gone_block\nstill_gone */ b\n",
			kept:   []string{"a ", "\n", " b"}, gone: []string{"gone_block", "still_gone"},
		},
		{
			name:   "a block comment that never closes stays visible",
			source: "a: 1,\n/* cut here\n",
			kept:   []string{"/* cut here"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := gatekit.TSWithoutComments(tc.source)
			if len(got) != len(tc.source) || strings.Count(got, "\n") != strings.Count(tc.source, "\n") {
				t.Errorf("the strip moved text: %q from %q", got, tc.source)
			}
			for _, want := range tc.kept {
				if !strings.Contains(got, want) {
					t.Errorf("%q lost %q: %q", tc.source, want, got)
				}
			}
			for _, unwanted := range tc.gone {
				if strings.Contains(got, unwanted) {
					t.Errorf("%q kept the comment %q: %q", tc.source, unwanted, got)
				}
			}
		})
	}
}

func TestTheTSStripperListsTheStringsItPassed(t *testing.T) {
	t.Parallel()
	_, literals := gatekit.TSCode("const a = \"one\"; // 'not_a_literal'\nconst b = 'two' + `three`; const r = /'x'/;\n")
	if want := []string{"one", "two", "three"}; !slices.Equal(literals, want) {
		t.Errorf("listed %q, want %q", literals, want)
	}
}
