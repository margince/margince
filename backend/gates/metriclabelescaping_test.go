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
// TWO SHAPES, because the tree writes labels two ways and a gate that saw only
// one would be blind to the surface it was written for. A format string spells
// `name=%q`; the AI renderer CONCATENATES, `"model=" + escaper(value)`, which
// is precisely where the operator-typed model id lands. The concatenated arm
// is an allowlist rather than a denylist: the fragment must be followed by
// Label, because there is no way to enumerate every wrong escaper and a new
// one would otherwise arrive unseen.
//
// WHAT IT DOES NOT SEE, stated because a prohibition that overclaims is worse
// than one that is narrow: a value formatted with %q into a variable on an
// earlier line and then concatenated in reaches neither arm. That gap is not
// rationalised away — it is what the allowlist narrows, since the concatenated
// arm rejects anything that is not a Label call, including a bare variable.

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

// labelByConcatenation matches the other shape: a label pair opened inside a
// string fragment that ENDS there, with the value appended after it —
// `"...{model=" + something` or `",tier=" + something`.
var labelByConcatenation = regexp.MustCompile(`[{,"]([a-z_][a-z0-9_]*)="\s*\+\s*([A-Za-z0-9_.]+)\(`)

// escapers are the calls a concatenated label value may be wrapped in. An
// allowlist, not a denylist: a wrong escaper nobody has thought of yet must
// fail rather than pass unrecognised.
var escapers = map[string]bool{"Label": true, "httpserver.Label": true}

// labelWithAnyVerb matches a label pair written either way. It is the
// denominator keeping this gate honest: if the corpus scan silently stopped
// matching, both counts would fall to zero together and the prohibition would
// pass while seeing nothing.
var labelWithAnyVerb = regexp.MustCompile(`[{,"]([a-z_][a-z0-9_]*)=(?:%[a-z]|"\s*\+)`)

// declaresAFamily selects the files this gate judges: the ones that name a
// margince_ family in a string literal, which is what an exposition writer does
// and a log line or a CLI report does not.
//
// It matches the family literal ALONE, with nothing required after it. An
// earlier spelling demanded a `{` or a space next, on the assumption that a
// name is always followed by its labels — and that assumption cost the gate the
// one file it was written for: the AI renderer passes its names as bare
// arguments to a header helper (`counterHeader(w, "margince_ai_calls_total",`)
// and writes its series through `"%s{%s}"`, so it matched neither shape and the
// concatenated arm below matched nothing in the whole tree.
var declaresAFamily = regexp.MustCompile(`"margince_[a-z0-9_]+`)

// expositionWaiver is how a writer that genuinely must format a label value
// itself says so. It carries a reason, because a reasonless waiver is the
// second way a census fails short.
const expositionWaiver = "//metrics:unescaped"

// The floors, and there are TWO because one total cannot tell the arms apart.
//
// A scan that found nothing would report PASS with no failing assertion, which
// is the one way this gate must not break. But a single total hides the failure
// that actually happened here: the %q-shaped writers alone satisfied a total of
// 8 while the concatenated arm saw ZERO lines in the entire tree, so the
// allowlist that arm exists to enforce was vacuous and nobody could tell.
//
// minLabelWriters is derived from the sections that must each contribute at
// least one labelled line — HTTP, jobs, overlay, AI, MCP-app, license, capture,
// comms — so a collapse to any single section trips it.
// minConcatenatedWriters is 1 because exactly one surface builds labels that
// way today; it must never fall to nothing unseen.
const (
	minLabelWriters        = 8
	minConcatenatedWriters = 1
)

func TestNoLabelValueIsEscapedWithGoQuoting(t *testing.T) {
	t.Parallel()

	writers, concatenated, offenders := 0, 0, []string{}
	expositionFiles(t, func(path, code string) {
		found, seen, appended := scanLabelWrites(code)
		writers += seen
		concatenated += appended
		for _, f := range found {
			offenders = append(offenders, filepath.ToSlash(path)+":"+f)
		}
	})

	if concatenated < minConcatenatedWriters {
		t.Errorf("this gate saw %d concatenated label writes, below the floor of %d.\n\n"+
			"The concatenated arm is the one written for the AI renderer's operator- and "+
			"provider-supplied model label. Seeing none means it is enforcing an allowlist over "+
			"an empty set — a pass that proves nothing.", concatenated, minConcatenatedWriters)
	}

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

// scanLabelWrites answers one file's offending label writes and how many
// labelled exposition lines it saw at all.
//
// A pure function over source text so the gate's own fixtures can drive it:
// a prohibition nobody has watched fail is a prohibition nobody knows fires.
func scanLabelWrites(code string) (offenders []string, seen, concatenated int) {
	lines := strings.Split(code, "\n")
	for i, line := range lines {
		if labelWithAnyVerb.MatchString(line) {
			seen++
		}
		match := labelByConcatenation.FindStringSubmatch(line)
		if match != nil {
			concatenated++
		}
		if waived(lines, i) {
			continue
		}
		if quoted := labelWithQuoteVerb.FindStringSubmatch(line); quoted != nil {
			offenders = append(offenders, fmt.Sprintf("%d renders %s=%%q", i+1, quoted[1]))
			continue
		}
		if match != nil && !escapers[match[2]] {
			offenders = append(offenders,
				fmt.Sprintf("%d appends %s= with %s(...) rather than Label(...)", i+1, match[1], match[2]))
		}
	}
	return offenders, seen, concatenated
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

// expositionFiles yields every non-test Go file that DECLARES a metric family,
// with its source, comments included.
//
// Stripping comments first would be the obvious move and is the wrong one: the
// waiver this gate honours IS a comment, so a scan that could not see comments
// could not see a waiver either. No file needs excluding by name — the
// forbidden shape requires a brace, comma or quote before the label name,
// which prose does not produce, and this file's own essay is proof of it.
func expositionFiles(t *testing.T, visit func(path, code string)) {
	t.Helper()
	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		code := readFile(t, path)
		if declaresAFamily.MatchString(code) {
			visit(path, code)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking the source tree: %v", err)
	}
}

// The gate's own falsification. Each case is a shape that has to fail or a
// shape that has to pass; without them the prohibition above could stop
// matching and report PASS over a tree full of offenders.
func TestTheEscapingGateFiresOnEveryShapeItClaims(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		code    string
		offends bool
	}{
		{"format verb with %q", `out.printf("margince_x{model=%q} %d\n", m, n)`, true},
		{"concatenated without an escaper", `w("margince_x{model=" + fmtQ(m) + "} 1")`, true},
		{"concatenated through Label", `w("margince_x{model=" + Label(m) + "} 1")`, false},
		{"concatenated through httpserver.Label", `w("margince_x{model=" + httpserver.Label(m) + "} 1")`, false},
		{"format verb with %s", `out.printf("margince_x{model=%s} %d\n", Label(m), n)`, false},
		{"a second label after the first", `out.printf("margince_x{a=%s,model=%q} 1\n", Label(a), m)`, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			offenders, seen, _ := scanLabelWrites(c.code)
			if got := len(offenders) > 0; got != c.offends {
				t.Errorf("offends = %v, want %v for:\n\t%s\n\tfindings: %v", got, c.offends, c.code, offenders)
			}
			if seen != 1 {
				t.Errorf("the denominator counted %d labelled lines, want 1 — it cannot see this shape, "+
					"so the floor would not notice a corpus collapse:\n\t%s", seen, c.code)
			}
		})
	}
}

// The waiver exists so a writer that provably cannot carry a hostile value can
// say so — and a reasonless one is itself a finding, or the escape hatch
// becomes the second place this census fails short.
func TestTheWaiverNeedsAReason(t *testing.T) {
	t.Parallel()
	withReason := expositionWaiver + " the value is a compile-time literal\n" +
		`out.printf("margince_x{model=%q} 1\n", m)`
	if offenders, _, _ := scanLabelWrites(withReason); len(offenders) != 0 {
		t.Errorf("a waiver carrying a reason was ignored: %v", offenders)
	}

	bare := expositionWaiver + "\n" + `out.printf("margince_x{model=%q} 1\n", m)`
	if offenders, _, _ := scanLabelWrites(bare); len(offenders) == 0 {
		t.Error("a reasonless waiver silenced the finding; the escape hatch is now the way through")
	}
}
