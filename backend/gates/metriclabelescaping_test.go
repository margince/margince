// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind prohibition H2

package gates

// A label VALUE in the hand-rolled exposition is escaped by httpserver.Label,
// never by %q.
//
// The two agree on ordinary input, which is why %q survived here for as long as
// it did. They part on everything else: strconv.Quote is Go's escaping and also
// emits \t, \r, \xNN and \uNNNN, none of which the Prometheus text format
// defines. Its parser rejects an invalid escape by discarding the WHOLE SCRAPE
// — not the line, not the family — so one stray byte in one label takes every
// series this process publishes off the dashboard at once, and the target then
// looks to Prometheus exactly like a target that is down.
//
// It stopped being hypothetical when the AI families gained a `model` label.
// PUT /ai/routing accepts any string for a tier binding's model and
// ValidateTierBinding constrains provider, profile and base_url but never
// Model — an ollama or vLLM identity is genuinely arbitrary text an operator
// types. Every other label in the tree is a closed vocabulary or a
// compile-time template, which is precisely why nobody had to think about this
// before and why the next author will not either.
//
// THE CENSUS IS OVER WHAT THE TREE EMITS, not over a list of files: it finds
// every line that writes a Prometheus label pair, wherever it is written, so a
// renderer added tomorrow is judged the day it is written and one that is
// deleted takes its row with it. A skip-list would be the second place this
// can fail short, so there is none — a writer that must use %q says so in
// source with a craft:ignore-style reason this gate reads.
//
// WHAT IT DOES NOT SEE, stated because a prohibition that overclaims is worse
// than one that is narrow: it reads the FORMAT VERB beside a label name, so a
// value pre-formatted into a variable with %q on an earlier line, or
// concatenated in from a helper this gate cannot follow, is invisible to it.
// What makes that acceptable is that it stops the shape every offender in this
// tree actually had — `name=%q` written inline — and that Label is now the
// only exported escaper, so the alternative has to be reached for deliberately.

import (
	"fmt"
	"io/fs"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// labelWithQuoteVerb matches a Prometheus label pair written with Go's %q:
// `name=%q` inside a format string, however the pair is introduced — after the
// opening brace, after a comma, or at the start of a concatenated fragment.
var labelWithQuoteVerb = regexp.MustCompile(`[{,"]([a-z_][a-z0-9_]*)=%q`)

// labelWithAnyVerb matches a label pair written with ANY format verb, which is
// the denominator that keeps this gate honest: if the corpus scan silently
// stopped matching, both counts would fall to zero together and the
// prohibition would pass while seeing nothing.
var labelWithAnyVerb = regexp.MustCompile(`[{,"]([a-z_][a-z0-9_]*)=%[a-z]`)

// expositionWaiver is how a writer that genuinely must format a label value
// itself says so. It carries a reason, because a reasonless waiver is the
// second way a census fails short.
const expositionWaiver = "//metrics:unescaped"

// minLabelWriters is a floor, not a count. A scan that found nothing would
// report PASS with no failing assertion, which is the one way this gate must
// not break: it would read a smaller tree, see no %q, and say so.
//
// The tree emits label pairs from the HTTP, job, overlay, AI, MCP-app, license
// and comms sections. Set well below that, so ordinary movement does not
// require an edit and a corpus collapse still does.
const minLabelWriters = 8

func TestNoLabelValueIsEscapedWithGoQuoting(t *testing.T) {
	t.Parallel()

	writers, offenders := 0, []string{}
	expositionFiles(t, func(path, code string) {
		lines := strings.Split(code, "\n")
		for i, line := range lines {
			if labelWithAnyVerb.MatchString(line) {
				writers++
			}
			match := labelWithQuoteVerb.FindStringSubmatch(line)
			if match == nil || waived(lines, i) {
				continue
			}
			offenders = append(offenders,
				fmt.Sprintf("%s:%d renders %s=%%q", filepath.ToSlash(path), i+1, match[1]))
		}
	})

	if writers < minLabelWriters {
		t.Fatalf("this gate found only %d lines writing a Prometheus label, below the floor of %d.\n\n"+
			"That is not a pass. Either the exposition shrank dramatically or the scan stopped "+
			"matching what the tree writes — and an under-recognising census reports PASS with "+
			"nothing to notice.", writers, minLabelWriters)
	}
	if len(offenders) > 0 {
		t.Errorf("%d label value(s) are escaped with Go's %%q rather than httpserver.Label:\n\t%s\n\n"+
			"Prometheus defines three escapes; %%q also emits \\t, \\r, \\xNN and \\uNNNN, and its "+
			"parser answers an invalid escape by rejecting the ENTIRE scrape. Use httpserver.Label "+
			"(internal/platform/httpserver/histogram.go), or write %s with a reason on the line "+
			"above if this value provably cannot carry one.",
			len(offenders), strings.Join(offenders, "\n\t"), expositionWaiver)
	}
}

// waived reports whether the line above carries the waiver marker.
func waived(lines []string, i int) bool {
	if i == 0 {
		return false
	}
	above := strings.TrimSpace(lines[i-1])
	if !strings.HasPrefix(above, expositionWaiver) {
		return false
	}
	// A reasonless waiver is itself a finding, so an empty one does not count
	// as a waiver at all and the line stays an offender.
	return strings.TrimSpace(strings.TrimPrefix(above, expositionWaiver)) != ""
}

// expositionFiles yields every non-test Go file in the backend tree with its
// source, comments included and nothing excluded.
//
// Stripping comments first would be the obvious move and is the wrong one: the
// waiver this gate honours IS a comment, so a scan that could not see comments
// could not see a waiver either. Nothing needs excluding — the forbidden shape
// requires a brace, comma or quote before the label name, which prose does not
// produce, and this file's own essay is proof that it does not.
func expositionFiles(t *testing.T, visit func(path, code string)) {
	t.Helper()
	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		visit(path, readFile(t, path))
		return nil
	})
	if err != nil {
		t.Fatalf("walking the source tree: %v", err)
	}
}
