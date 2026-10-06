// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind prohibition H2

//go:build !integration

package gates

// A page whose first line is <!-- prose:plain --> is written in plain words: short
// sentences and a vocabulary of fewer than 1,000 general words plus
// named technical terms. A page that must stay short adds max-words=N to the
// marker. The method and how to add a word are in
// docs/reference/docs-prose-style.md#plain-pages.

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

const (
	plainMarker        = "prose:plain"
	plainWordsFile     = "docs/reference/plain-words.txt"
	technicalNamesFile = "docs/reference/technical-names.txt"
	plainWordCap       = 999
	plainStepWords     = 20
	plainSentenceWords = 25
	plainParagraphMax  = 6
)

// plainRequired are pages that must stay enrolled, with the most words each may
// hold: the README is the first page a stranger reads, so it can neither leave
// the bar nor drop its length limit.
var plainRequired = map[string]int{"README.md": 1000}

var (
	plainToken    = regexp.MustCompile(`\p{L}[\p{L}'’-]*`)
	plainHeader   = regexp.MustCompile(`^<!--\s*prose:plain(?:\s+max-words=(\d+))?\s*-->$`)
	plainPathLink = regexp.MustCompile(`\[[^\]\s]*[./][^\]\s]*\]`)
	plainStepItem = regexp.MustCompile(`^\s*\d+\.\s+`)
)

func readWordList(t *testing.T, rel string) []string {
	t.Helper()
	f, err := os.Open(filepath.Join(docsTreeRoot, rel))
	if err != nil {
		t.Fatalf("open %s: %v", rel, err)
	}
	defer func() {
		if cerr := f.Close(); cerr != nil {
			t.Errorf("close %s: %v", rel, cerr)
		}
	}()
	var words []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		if w := strings.TrimSpace(sc.Text()); w != "" && !strings.HasPrefix(w, "#") {
			words = append(words, w)
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("read %s: %v", rel, err)
	}
	return words
}

// plainForms are the inflections a listed base word may take, so the list
// holds "deal" and a page may write "deals" or "dealing".
func plainForms(word string) []string {
	forms := []string{word}
	for _, suffix := range []string{"s", "es", "ed", "d", "ing", "er", "ers", "est", "ly", "'s", "’s"} {
		base, ok := strings.CutSuffix(word, suffix)
		if !ok || len(base) < 2 {
			continue
		}
		forms = append(forms, base, base+"e")
		if strings.HasSuffix(base, "i") {
			forms = append(forms, strings.TrimSuffix(base, "i")+"y")
		}
		if n := len(base); n > 2 && base[n-1] == base[n-2] {
			forms = append(forms, base[:n-1])
		}
	}
	return forms
}

// plainVocab keeps the two lists apart: a technical name matches only as
// written, so "Go" does not admit the ordinary word "go".
type plainVocab struct {
	general map[string]bool
	names   map[string]bool
}

// plainMatch returns the list entries a word resolves to, one per hyphenated
// part, or false when any part is unlisted.
func plainMatch(word string, vocab plainVocab) ([]string, bool) {
	trimmed := strings.Trim(word, "'’-")
	if vocab.names[trimmed] {
		return []string{trimmed}, true
	}
	var keys []string
	for _, part := range strings.Split(strings.ToLower(trimmed), "-") {
		found := false
		for _, form := range plainForms(part) {
			if vocab.general[form] {
				keys, found = append(keys, form), true
				break
			}
		}
		if !found {
			return nil, false
		}
	}
	return keys, true
}

type plainResult struct {
	words   int
	unknown map[string]int
	long    []string
	used    map[string]bool
	crowded []int
}

func plainCheck(doc string, vocab plainVocab) plainResult {
	res := plainResult{unknown: map[string]int{}, used: map[string]bool{}}
	lines := barLines(doc)
	for _, l := range lines {
		// A link whose text is a path or a host names a file, like inline code does.
		for _, w := range plainToken.FindAllString(plainPathLink.ReplaceAllString(l.text, " CODE "), -1) {
			if w == "CODE" {
				continue
			}
			res.words++
			if keys, ok := plainMatch(w, vocab); ok {
				for _, k := range keys {
					res.used[k] = true
				}
				continue
			}
			res.unknown[w]++
		}
	}
	for _, p := range barParagraphs(lines) {
		if strings.HasPrefix(p.text, "|") || strings.HasPrefix(p.text, "#") {
			continue
		}
		limit := plainSentenceWords
		if raw := strings.Split(doc, "\n"); p.n <= len(raw) && plainStepItem.MatchString(raw[p.n-1]) {
			limit = plainStepWords
		}
		sentences := splitSentences(p.text)
		for _, s := range sentences {
			if n := len(barWord.FindAllString(s, -1)); n > limit {
				res.long = append(res.long, strings.TrimSpace(s))
			}
		}
		if len(sentences) > plainParagraphMax {
			res.crowded = append(res.crowded, p.n)
		}
	}
	return res
}

// plainEnrolment reads a page's first line: whether it is a plain page, and the
// most words it may hold, where 0 means no length limit.
func plainEnrolment(doc string) (enrolled bool, maxWords int) {
	first, _, _ := strings.Cut(doc, "\n")
	m := plainHeader.FindStringSubmatch(strings.TrimSpace(first))
	if m == nil {
		return false, 0
	}
	if m[1] != "" {
		n, err := strconv.Atoi(m[1])
		if err != nil {
			return true, 1
		}
		return true, n
	}
	return true, 0
}

func plainTooLong(res plainResult, maxWords int) bool { return maxWords > 0 && res.words > maxWords }

func TestPlainPagesUseFewerThanAThousandWords(t *testing.T) {
	t.Parallel()
	general := readWordList(t, plainWordsFile)
	names := readWordList(t, technicalNamesFile)
	if len(general) > plainWordCap {
		t.Errorf("%s lists %d words; the cap is %d. Replace a word on a page with a listed one before adding another.",
			plainWordsFile, len(general), plainWordCap)
	}
	vocab := plainVocab{general: map[string]bool{}, names: map[string]bool{}}
	for _, w := range general {
		vocab.general[strings.ToLower(w)] = true
	}
	for _, w := range names {
		vocab.names[w] = true
	}
	if !sort.StringsAreSorted(general) {
		t.Errorf("%s is not sorted; keep one word per line in byte order so a diff shows what was added", plainWordsFile)
	}

	used := map[string]bool{}
	enrolled := map[string]bool{}
	limits := map[string]int{}
	for _, f := range trackedFiles(t) {
		if f.symlink || !strings.HasSuffix(f.path, ".md") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(docsTreeRoot, f.path))
		if err != nil {
			t.Fatalf("read %s: %v", f.path, err)
		}
		ok, maxWords := plainEnrolment(string(raw))
		if !ok {
			continue
		}
		enrolled[f.path] = true
		limits[f.path] = maxWords
		res := plainCheck(string(raw), vocab)
		for w := range res.used {
			used[w] = true
		}
		if plainTooLong(res, maxWords) {
			t.Errorf("%s has %d words; its marker allows at most %d. Link to a deeper page instead.", f.path, res.words, maxWords)
		}
		for w, n := range res.unknown {
			t.Errorf("%s uses %q (%d×), which is in neither %s nor %s. Use a listed word, or add a technical name.",
				f.path, w, n, plainWordsFile, technicalNamesFile)
		}
		for _, s := range res.long {
			t.Errorf("%s: sentence over the limit (%d words for a numbered step, %d otherwise): %q",
				f.path, plainStepWords, plainSentenceWords, s)
		}
		for _, line := range res.crowded {
			t.Errorf("%s:%d: paragraph has more than %d sentences; split it", f.path, line, plainParagraphMax)
		}
	}
	for rel, want := range plainRequired {
		if !enrolled[rel] || limits[rel] == 0 || limits[rel] > want {
			t.Errorf("%s must start with <!-- %s max-words=%d --> (or a lower limit): it is the first page a reader opens",
				rel, plainMarker, want)
		}
	}
	for _, list := range []struct {
		rel   string
		words []string
	}{{plainWordsFile, general}, {technicalNamesFile, names}} {
		for _, w := range list.words {
			if !used[w] && (list.rel == technicalNamesFile || !used[strings.ToLower(w)]) {
				t.Errorf("%s lists %q, which no plain page uses; remove it", list.rel, w)
			}
		}
	}
}

// Each rule fires on a planted page, so a broken matcher fails here and not by
// reading every page as clean.
func TestPlainPageRulesFireOnPlantedDefects(t *testing.T) {
	t.Parallel()
	vocab := plainVocab{
		general: map[string]bool{"the": true, "deal": true, "is": true, "open": true, "a": true, "word": true},
		names:   map[string]bool{"Go": true},
	}
	planted := "The deals is opened. The deal is circumnavigated.\n\n" +
		"1. " + strings.Repeat("word ", 21) + "\n\n" +
		strings.Repeat("A deal is open. ", 7)
	res := plainCheck(planted, vocab)
	if res.unknown["circumnavigated"] == 0 {
		t.Error("an unlisted word was not reported")
	}
	if res.unknown["deals"] != 0 || res.unknown["opened"] != 0 {
		t.Errorf("an inflection of a listed word was reported: %v", res.unknown)
	}
	if len(res.long) == 0 {
		t.Error("a 21-word numbered step was not reported")
	}
	if len(res.crowded) == 0 {
		t.Error("a seven-sentence paragraph was not reported")
	}
	if got := plainCheck("Go is open. A deal is open to go.", vocab).unknown["go"]; got == 0 {
		t.Error("a technical name admitted its lowercase twin as a general word")
	}
	if got := plainCheck("Die Straße ist offen.", vocab).words; got != 4 {
		t.Errorf("non-ASCII words were not counted: got %d of 4", got)
	}
	if !plainTooLong(plainCheck(strings.Repeat("word ", 11), vocab), 10) {
		t.Error("a page over its word limit was not reported")
	}
	if plainTooLong(plainCheck(strings.Repeat("word ", 11), vocab), 0) {
		t.Error("a page with no word limit was reported as too long")
	}
	for doc, want := range map[string]int{
		"<!-- prose:plain -->\n# A":                0,
		"<!-- prose:plain max-words=1000 -->\n# A": 1000,
	} {
		if ok, got := plainEnrolment(doc); !ok || got != want {
			t.Errorf("marker %q read as enrolled=%v limit=%d, want limit %d", doc, ok, got, want)
		}
	}
	if ok, _ := plainEnrolment("# A\n`<!-- prose:plain -->`"); ok {
		t.Error("a page quoting the marker below its first line was enrolled")
	}
}
