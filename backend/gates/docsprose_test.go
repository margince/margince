// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind prohibition H2

//go:build !integration

package gates

// Every Markdown page meets the house prose bar in
// docs/reference/docs-prose-style.md. The rules are the mechanical half of that
// page: each one names a pattern a reader skims past and an agent reproduces,
// measured on prose with code removed. A line that truly needs a banned form
// carries `<!-- prose:allow <rule> <reason> -->` on the line above it.

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var (
	barLexicon = regexp.MustCompile(`(?i)\b(honest|honestly|honesty|genuine|genuinely|deliberate|deliberately|` +
		`on purpose|quietly|load-bearing|exactly|is the point|the whole point|earns its place|earns its keep)\b`)
	barCapsWord = regexp.MustCompile(`(?:^|[^\w/.-])([A-Z][A-Z'’]+)(?:$|[^\w/.-])`)
	barNegation = []*regexp.Regexp{
		regexp.MustCompile(`\b(is|are|was|were) not [^.\n]{2,80}\.\s+(It|They|That|This) (is|are|was|were)\b`),
		regexp.MustCompile(`(?i)\bnot (just|only|merely|simply) [^.\n]{1,80}\bbut\b`),
	}
	barResidue = regexp.MustCompile(`(?i)\b(used to be|previously|this change|this PR|first cut|` +
		`at the time of writing|Task \d+|(was|were) (renamed|removed|replaced|retired) in)\b`)
	barPreamble = regexp.MustCompile(`(?i)\bthis (page|document|doc|guide|section) ` +
		`(explains|describes|covers|walks you through|is about|documents)\b`)
	barCount = regexp.MustCompile(`(?i)\b(\d{1,4}|(?:five|six|seven|eight|nine|ten|eleven|twelve|thirteen|fourteen|` +
		`fifteen|sixteen|seventeen|eighteen|nineteen|twenty|thirty|forty|fifty|sixty|seventy|eighty|ninety)` +
		`(?:[- ](?:one|two|three|four|five|six|seven|eight|nine))?)\s+(?:[a-z-]+\s+)?(modules|AI tasks|` +
		`activity kinds|job kinds|tools|scopes|gates|tables|endpoints|shards|RBAC objects|event types|` +
		`field types|triggers|consumer groups)\b`)
	barLeadIn   = regexp.MustCompile(`(?i)\b(two|three|four|five|six|seven|eight|nine|ten)\b[^:\n]{0,60}:\s*$`)
	barListItem = regexp.MustCompile(`^(\s*)([-*+]|\d+\.)\s+`)
	barBold     = regexp.MustCompile(`\*\*([^*\n]+)\*\*|__([^_\n]+)__`)
	barAllow    = regexp.MustCompile(`<!--\s*prose:allow\s+([a-z,]+)\s+\S.*?-->`)
	barWord     = regexp.MustCompile(`[A-Za-z][A-Za-z'’-]*`)
	barSentence = regexp.MustCompile(`([.!?][*_)"”’]*)\s+([*_("“]*[A-Z])`)
)

// barEmphasis is ordinary English that only ever reaches ALL CAPS as shouting.
// Acronyms are not listed, so they never match.
var barEmphasis = wordSet(`one not own only same both every never always all no any none before after first
last this that the whole inside outside read write new old each must and or here there which from is are was twice
once either neither nothing everything also still yet again more less most least really very then now until unless
without with does do can cannot will may should hold holds itself what why how who where when per across
above below into onto over under ever`)

var barLeadWords = map[string]int{"two": 2, "three": 3, "four": 4, "five": 5, "six": 6, "seven": 7, "eight": 8,
	"nine": 9, "ten": 10}

// The Contributor Covenant is adopted text and docscodeofconduct_test.go holds
// it verbatim, so its wording is not ours to restyle.
var barExempt = map[string]bool{"CODE_OF_CONDUCT.md": true}

// History is the content of a changelog and of an evidence record, and the PR
// template asks about "this change"; everywhere else it is residue.
func residueAllowed(rel string) bool {
	return rel == "CHANGELOG.md" || rel == ".github/PULL_REQUEST_TEMPLATE.md" ||
		strings.HasPrefix(rel, "docs/evidence/")
}

func wordSet(s string) map[string]bool {
	set := map[string]bool{}
	for _, w := range strings.Fields(s) {
		set[w] = true
	}
	return set
}

type barLine struct {
	n       int
	text    string
	allowed map[string]bool
}

type barFinding struct {
	line int
	rule string
	what string
}

var (
	barInlineCode = regexp.MustCompile("`[^`]*`")
	barLinkTarget = regexp.MustCompile(`\]\([^)]*\)`)
	barURL        = regexp.MustCompile(`<?https?://[^\s>]+>?`)
	barComment    = regexp.MustCompile(`<!--.*?-->`)
	barFence      = regexp.MustCompile("^(```|~~~)")
)

// barLines strips what is not prose: fenced code, inline code, link targets,
// URLs and HTML comments. A waiver covers its own line and the next, so a
// table row can carry one at its end.
func barLines(doc string) []barLine {
	var out []barLine
	inFence, inComment := false, false
	allowNext := map[string]bool{}
	for i, raw := range strings.Split(doc, "\n") {
		if barFence.MatchString(strings.TrimSpace(raw)) {
			inFence = !inFence
			allowNext = map[string]bool{}
			continue
		}
		if inFence {
			continue
		}
		allowed := allowNext
		allowNext = map[string]bool{}
		for _, m := range barAllow.FindAllStringSubmatch(raw, -1) {
			for _, rule := range strings.Split(m[1], ",") {
				allowNext[rule] = true
				allowed[rule] = true
			}
		}
		line := raw
		if inComment {
			end := strings.Index(line, "-->")
			if end < 0 {
				continue
			}
			line, inComment = line[end+3:], false
		}
		line = barComment.ReplaceAllString(line, "")
		if open := strings.Index(line, "<!--"); open >= 0 {
			line, inComment = line[:open], true
		}
		line = barInlineCode.ReplaceAllString(line, " CODE ")
		line = barLinkTarget.ReplaceAllString(line, "]")
		line = barURL.ReplaceAllString(line, " ")
		out = append(out, barLine{n: i + 1, text: line, allowed: allowed})
	}
	return out
}

func checkHouseBar(rel, doc string, generated bool) []barFinding {
	lines := barLines(doc)
	var found []barFinding
	add := func(l barLine, rule, what string) {
		if !l.allowed[rule] {
			found = append(found, barFinding{l.n, rule, what})
		}
	}
	found = append(found, emDashFindings(lines)...)
	for _, l := range lines {
		for _, m := range barLexicon.FindAllString(l.text, -1) {
			add(l, "lexicon", m)
		}
		for _, w := range capsEmphasis(l.text) {
			add(l, "caps", w)
		}
		for _, m := range barBold.FindAllStringSubmatch(l.text, -1) {
			if span := m[1] + m[2]; len(barWord.FindAllString(span, -1)) > 8 {
				add(l, "bold", span)
			}
		}
		if !residueAllowed(rel) {
			for _, m := range barResidue.FindAllString(l.text, -1) {
				add(l, "residue", m)
			}
		}
		for _, m := range barPreamble.FindAllString(l.text, -1) {
			add(l, "preamble", m)
		}
		if !generated {
			found = append(found, countFindings(l)...)
		}
	}
	found = append(found, paragraphFindings(lines)...)
	return append(found, leadInFindings(doc, lines)...)
}

// barEmptyCell is a table cell holding only a dash: a "no value" marker that
// envcontract_test.go requires in the configuration reference, not a clause joiner.
var barEmptyCell = regexp.MustCompile(`\|\s*—(\s+\([^)|]*\))?\s*\|`)

// emDashFindings allows a few dashes per thousand words: the target is none,
// and the ceiling only spares a page one well-placed aside.
func emDashFindings(lines []barLine) []barFinding {
	words, dashes := 0, 0
	var at []barFinding
	for _, l := range lines {
		words += len(barWord.FindAllString(l.text, -1))
		text := barEmptyCell.ReplaceAllString(barEmptyCell.ReplaceAllString(l.text, "| |"), "| |")
		if n := strings.Count(text, "—"); n > 0 && !l.allowed["emdash"] {
			dashes += n
			at = append(at, barFinding{l.n, "emdash", "—"})
		}
	}
	if ceiling := max(2, 3*words/1000); dashes > ceiling {
		at[0].what = fmt.Sprintf("%d em dashes, ceiling %d for %d words", dashes, ceiling, words)
		return at
	}
	return nil
}

// capsEmphasis skips a word whose neighbour is also capitals, so SQL and
// keyword runs ("NOT NULL", "ON CONFLICT DO UPDATE") read as code.
func capsEmphasis(text string) []string {
	var out []string
	for _, loc := range barCapsWord.FindAllStringSubmatchIndex(text, -1) {
		w := text[loc[2]:loc[3]]
		lower := strings.TrimRight(strings.ToLower(w), "'’s")
		if !barEmphasis[lower] && !barEmphasis[strings.ToLower(w)] {
			continue
		}
		before := strings.Fields(text[:loc[2]])
		after := strings.Fields(text[loc[3]:])
		if (len(before) > 0 && isCapsToken(before[len(before)-1])) || (len(after) > 0 && isCapsToken(after[0])) {
			continue
		}
		out = append(out, w)
	}
	return out
}

func isCapsToken(tok string) bool {
	tok = strings.TrimRight(tok, ",.;:")
	return len(tok) > 1 && strings.ToUpper(tok) == tok && strings.ToLower(tok) != tok
}

func countFindings(l barLine) []barFinding {
	var out []barFinding
	for _, m := range barCount.FindAllStringSubmatch(l.text, -1) {
		if n := m[1]; len(n) == 1 && n < "5" {
			continue
		}
		if !l.allowed["count"] {
			out = append(out, barFinding{l.n, "count", m[0]})
		}
	}
	return out
}

// paragraphFindings judges sentences and contrasts across wrapped lines, since
// both routinely span several source lines.
func paragraphFindings(lines []barLine) []barFinding {
	var out []barFinding
	for _, p := range barParagraphs(lines) {
		for _, rx := range barNegation {
			for _, m := range rx.FindAllString(p.text, -1) {
				if !p.allowed["negation"] {
					out = append(out, barFinding{p.n, "negation", m})
				}
			}
		}
		if strings.HasPrefix(p.text, "|") || p.allowed["sentence"] {
			continue
		}
		for _, s := range splitSentences(p.text) {
			if n := len(barWord.FindAllString(s, -1)); n > 40 {
				out = append(out, barFinding{p.n, "sentence", fmt.Sprintf("%d words", n)})
			}
		}
	}
	return out
}

func splitSentences(text string) []string {
	var out []string
	for {
		loc := barSentence.FindStringSubmatchIndex(text)
		if loc == nil {
			return append(out, text)
		}
		out = append(out, text[:loc[3]])
		text = text[loc[4]:]
	}
}

func barParagraphs(lines []barLine) []barLine {
	var out []barLine
	var cur []string
	var head barLine
	flush := func() {
		if len(cur) > 0 {
			out = append(out, barLine{n: head.n, text: strings.Join(cur, " "), allowed: head.allowed})
			cur = nil
		}
	}
	for _, l := range lines {
		st := strings.TrimSpace(l.text)
		block := st == "" || strings.HasPrefix(st, "#") || strings.HasPrefix(st, "|") || barListItem.MatchString(st)
		if block {
			flush()
		}
		switch {
		case st == "":
		case strings.HasPrefix(st, "#"), strings.HasPrefix(st, "|"):
			out = append(out, barLine{n: l.n, text: st, allowed: l.allowed})
		default:
			if len(cur) == 0 {
				head = l
			}
			cur = append(cur, strings.TrimLeft(barListItem.ReplaceAllString(st, ""), "> "))
		}
	}
	flush()
	return out
}

// leadInFindings holds a counted lead-in ("Three things:") to the list under
// it.
func leadInFindings(doc string, lines []barLine) []barFinding {
	raw := strings.Split(doc, "\n")
	var out []barFinding
	for _, l := range lines {
		m := barLeadIn.FindStringSubmatch(l.text)
		if m == nil || l.allowed["leadin"] {
			continue
		}
		want := barLeadWords[strings.ToLower(m[1])]
		if got := listItemsAfter(raw, l.n); got > 0 && got != want {
			out = append(out, barFinding{l.n, "leadin", fmt.Sprintf("says %s, the list has %d items", m[1], got)})
		}
	}
	return out
}

// listItemsAfter counts the items at the first list's own indent, starting
// after line n (1-based), across blank lines between items.
func listItemsAfter(raw []string, n int) int {
	items, indent := 0, -1
	for j := n; j < len(raw); j++ {
		r := raw[j]
		if m := barListItem.FindStringSubmatch(r); m != nil {
			if indent < 0 {
				indent = len(m[1])
			}
			if len(m[1]) == indent {
				items++
			}
			continue
		}
		if strings.TrimSpace(r) == "" {
			if indent < 0 || (j+1 < len(raw) && barListItem.MatchString(raw[j+1])) {
				continue
			}
			return items
		}
		if indent < 0 || !strings.HasPrefix(r, strings.Repeat(" ", indent+1)) {
			return items
		}
	}
	return items
}

func TestDocsProseMeetsTheHouseBar(t *testing.T) {
	t.Parallel()
	pages := 0
	for _, f := range trackedFiles(t) {
		if f.symlink || !strings.HasSuffix(f.path, ".md") || barExempt[f.path] {
			continue
		}
		path := filepath.Join(docsTreeRoot, f.path)
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", f.path, err)
		}
		pages++
		for _, v := range checkHouseBar(f.path, string(raw), isGeneratedDoc(t, path)) {
			t.Errorf("%s:%d: [%s] %s", f.path, v.line, v.rule, v.what)
		}
	}
	if pages < 100 {
		t.Fatalf("read %d Markdown pages; the tree holds well over 200, so this gate is reading the wrong root", pages)
	}
	if t.Failed() {
		t.Log("Rewrite each line to docs/reference/docs-prose-style.md. A form that must stay (a quoted UI label, " +
			"third-party text) takes `<!-- prose:allow <rule> <reason> -->` on the line above.")
	}
}

// Each rule is planted once, so a regex that stops matching fails here rather
// than reading a smaller tree and reporting PASS.
func TestDocsProseRulesFireOnPlantedDefects(t *testing.T) {
	t.Parallel()
	planted := strings.Join([]string{
		"This page explains the relay. It is honest about load.",
		"The relay holds ONE lock per row, and the order matters here.",
		"**This bold span runs well past the eight word limit for a span.**",
		"A retry is not a failure. It is the normal path.",
		"The relay used to be synchronous.",
		"The installation ships nineteen modules.",
		"Three things follow:",
		"",
		"- one",
		"- two",
		"",
		"one two three four five six seven eight nine ten eleven twelve thirteen fourteen fifteen sixteen " +
			"seventeen eighteen nineteen twenty one two three four five six seven eight nine ten eleven twelve " +
			"thirteen fourteen fifteen sixteen seventeen eighteen nineteen twenty end.",
		"a — b — c — d",
	}, "\n")
	fired := map[string]bool{}
	for _, f := range checkHouseBar("docs/planted.md", planted, false) {
		fired[f.rule] = true
	}
	for _, rule := range []string{"preamble", "lexicon", "caps", "bold", "negation", "residue", "count", "leadin",
		"sentence", "emdash"} {
		if !fired[rule] {
			t.Errorf("rule %q did not fire on its planted defect", rule)
		}
	}
	clean := "<!-- prose:allow lexicon quoting the screen -->\nThe card reads \"Honest limits\".\n" +
		"A `NOT NULL` column and NOT NULL in prose are SQL.\nThe relay retries twice.\n" +
		"| honest | quietly | <!-- prose:allow lexicon a row quoting the list --> |\n" +
		"| `X` | — | — (required) | — |\n| `Y` | — | — | — |"
	if got := checkHouseBar("docs/clean.md", clean, false); len(got) > 0 {
		t.Errorf("clean prose was flagged: %+v", got)
	}
}
