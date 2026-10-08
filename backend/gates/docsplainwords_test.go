// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind prohibition H2

//go:build !integration

package gates

// A page whose first line is <!-- prose:plain --> or <!-- prose:plain max-words=N -->
// is written in plain words: short sentences and a vocabulary of fewer than 1,000
// general words plus named technical terms, and with the optional limit, at most N words.
// The method and how to add a word are in docs/reference/docs-prose-style.md#plain-pages.

import (
	"bufio"
	"fmt"
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
	plainWordsFile     = "docs/plain-words.txt"
	plainProjectFile   = "docs/plain-words-project.txt"
	glossaryFile       = "docs/reference/glossary.md"
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
	plainToken  = regexp.MustCompile(`\p{L}[\p{L}'’-]*`)
	plainHeader = regexp.MustCompile(`^<!--\s*prose:plain(?:\s+max-words=(\d+))?\s*-->$`)
	// plainScreenText is a bold label or a quoted message: the words the reader
	// sees on the screen, which a page must copy exactly, so like inline code
	// they are names and not prose to simplify.
	plainScreenText = regexp.MustCompile(`\*\*[^*\n]+\*\*|"[^"\n]+"|“[^”\n]+”`)
	plainPathLink   = regexp.MustCompile(`\[[^\]\s]*[./][^\]\s]*\]`)
	plainStepItem   = regexp.MustCompile(`^\s*\d+\.\s+`)
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
		if !ok || !plainStem(base, suffix) {
			continue
		}
		forms = append(forms, base)
		switch suffix {
		case "ing", "ed", "er", "ers", "est":
			forms = append(forms, base+"e")
			if n := len(base); n > 2 && base[n-1] == base[n-2] {
				forms = append(forms, base[:n-1])
			}
		}
		if strings.HasSuffix(base, "i") && suffix != "s" && suffix != "ing" {
			forms = append(forms, strings.TrimSuffix(base, "i")+"y")
		}
	}
	return forms
}

// plainStem says whether base can be what is left of a word once suffix is cut.
// A stem has a vowel, so "thing" is not "th" + "ing"; a bare "d" follows an "e",
// so "band" is not "ban" + "d"; and "-ly", "-er" and "-est" need a stem of four
// letters, so "apply", "reply" and "offer" do not reduce to "app", "rep", "off".
func plainStem(base, suffix string) bool {
	if !strings.ContainsAny(base, "aeiouy") {
		return false
	}
	switch suffix {
	case "d":
		return strings.HasSuffix(base, "e")
	case "ly", "er", "ers", "est":
		return len(base) >= 4
	}
	return len(base) >= 2
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
		text := plainScreenText.ReplaceAllString(plainPathLink.ReplaceAllString(l.text, " CODE "), " CODE ")
		for _, w := range plainToken.FindAllString(text, -1) {
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
			// An unreadable limit must not pass as enrolled, so plainRequired names the marker.
			return false, 0
		}
		return true, n
	}
	return true, 0
}

func plainTooLong(res plainResult, maxWords int) bool { return maxWords > 0 && res.words > maxWords }

// plainPools gives each docs area its own list of fewer than 1,000 general
// words, so a reader of one area meets a small vocabulary. The embedded copy of
// the handbook reads the same list as its source. The entry pages a newcomer
// opens first share the root list; every other page reads the project list.
var plainPools = []struct{ prefix, list string }{
	{"docs/handbook/", "docs/handbook/plain-words.txt"},
	{"backend/internal/modules/knowledge/handbook/", "docs/handbook/plain-words.txt"},
	{"docs/how-to/", "docs/how-to/plain-words.txt"},
	{"docs/tutorials/", "docs/how-to/plain-words.txt"},
	{"docs/explanation/", "docs/explanation/plain-words.txt"},
	{"docs/reference/", "docs/reference/plain-words.txt"},
	{"README.md", plainWordsFile},
	{"CONTRIBUTING.md", plainWordsFile},
	{"SECURITY.md", plainWordsFile},
	{"SUPPORT.md", plainWordsFile},
	{"docs/README.md", plainWordsFile},
	{"", plainProjectFile},
}

// plainCaps is the most general words each list may hold. 999 is the target: a
// reader of one area meets fewer than 1,000 simple words. A cap above the target
// must equal its list's size, so a list that shrinks pins its cap lower with it.
var plainCaps = map[string]int{
	plainWordsFile:                     plainWordCap,
	"docs/how-to/plain-words.txt":      1254,
	"docs/explanation/plain-words.txt": 1547,
	"docs/handbook/plain-words.txt":    1353,
	"docs/reference/plain-words.txt":   1773,
	plainProjectFile:                   1970,
}

// glossaryLegacyMax is how many names may still lack a meaning: the names pages
// used before they joined the bar. It must equal that count, so it only goes down.
const glossaryLegacyMax = 447

const glossaryLegacyHeading = "## Names without a meaning yet"

var glossaryLegacyName = regexp.MustCompile("`([^`]+)`")

var glossaryRow = regexp.MustCompile(`^\|\s*([^|]+?)\s*\|\s*([^|]*?)\s*\|\s*$`)

// parseGlossary reads the glossary table: each term a plain page may use as a
// technical name, and a meaning a reader can learn it from. legacy counts the
// names in the last section, which carry no meaning.
func parseGlossary(doc string) (terms []string, legacy int, problems []string) {
	seen := map[string]bool{}
	inLegacy := false
	for _, line := range strings.Split(doc, "\n") {
		if strings.HasPrefix(line, "## ") {
			inLegacy = strings.TrimSpace(line) == glossaryLegacyHeading
			continue
		}
		if inLegacy {
			for _, m := range glossaryLegacyName.FindAllStringSubmatch(line, -1) {
				if seen[m[1]] {
					problems = append(problems, fmt.Sprintf("%q is listed twice", m[1]))
				}
				seen[m[1]] = true
				terms = append(terms, m[1])
				legacy++
			}
			continue
		}
		m := glossaryRow.FindStringSubmatch(strings.TrimSpace(line))
		if m == nil || m[1] == "Term" || strings.Trim(m[1], "-: ") == "" {
			continue
		}
		term := strings.Trim(m[1], "`")
		if seen[term] {
			problems = append(problems, fmt.Sprintf("%q is listed twice", term))
		}
		seen[term] = true
		if len(barWord.FindAllString(m[2], -1)) < 3 {
			problems = append(problems, fmt.Sprintf("%q needs a meaning of at least three words", term))
		}
		terms = append(terms, term)
	}
	return terms, legacy, problems
}

func plainPoolFor(rel string) string {
	for _, p := range plainPools {
		if strings.HasPrefix(rel, p.prefix) {
			return p.list
		}
	}
	return plainWordsFile
}

// loadPlainPool reads one general list and checks the rules every list keeps.
func loadPlainPool(t *testing.T, rel string, names []string) plainVocab {
	t.Helper()
	general := readWordList(t, rel)
	limit, ok := plainCaps[rel]
	if !ok {
		limit = plainWordCap
	}
	if len(general) > limit {
		t.Errorf("%s lists %d words; the cap is %d. Replace a word on a page with a listed one before adding another.",
			rel, len(general), limit)
	}
	if limit > plainWordCap && len(general) < limit {
		t.Errorf("%s lists %d words under a cap of %d; lower its plainCaps entry to %d so the list cannot grow back.",
			rel, len(general), limit, len(general))
	}
	if !sort.StringsAreSorted(general) {
		t.Errorf("%s is not sorted; keep one word per line in byte order so a diff shows what was added", rel)
	}
	vocab := plainVocab{general: map[string]bool{}, names: map[string]bool{}}
	for _, w := range general {
		vocab.general[strings.ToLower(w)] = true
	}
	for _, w := range names {
		vocab.names[w] = true
	}
	return vocab
}

func TestPlainPagesUseFewerThanAThousandWords(t *testing.T) {
	t.Parallel()
	glossary, err := os.ReadFile(filepath.Join(docsTreeRoot, glossaryFile))
	if err != nil {
		t.Fatalf("read %s: %v", glossaryFile, err)
	}
	names, legacy, problems := parseGlossary(string(glossary))
	for _, p := range problems {
		t.Errorf("%s: %s", glossaryFile, p)
	}
	if legacy > glossaryLegacyMax {
		t.Errorf("%s: %d names lack a meaning; at most %d may. Give a new name a meaning in the table.",
			glossaryFile, legacy, glossaryLegacyMax)
	}
	if legacy < glossaryLegacyMax {
		t.Errorf("%s: %d names lack a meaning; lower glossaryLegacyMax from %d to %d so the count cannot grow back.",
			glossaryFile, legacy, glossaryLegacyMax, legacy)
	}
	pools := map[string]plainVocab{}
	used := map[string]map[string]bool{}
	for _, p := range plainPools {
		if _, ok := pools[p.list]; !ok {
			pools[p.list] = loadPlainPool(t, p.list, names)
			used[p.list] = map[string]bool{}
		}
	}

	enrolled := map[string]bool{}
	limits := map[string]int{}
	usedNames := map[string]bool{}
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
		list := plainPoolFor(f.path)
		res := plainCheck(string(raw), pools[list])
		for w := range res.used {
			used[list][w] = true
			usedNames[w] = true
		}
		if plainTooLong(res, maxWords) {
			t.Errorf("%s has %d words; its marker allows at most %d. Link to a deeper page instead.", f.path, res.words, maxWords)
		}
		for w, n := range res.unknown {
			t.Errorf("%s uses %q (%d×), which is in neither %s nor %s. Use a listed word, or add a technical name.",
				f.path, w, n, list, glossaryFile)
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
	for list, vocab := range pools {
		for w := range vocab.general {
			if !used[list][w] {
				t.Errorf("%s lists %q, which no plain page that reads it uses; remove it", list, w)
			}
		}
	}
	for _, w := range names {
		if !usedNames[w] {
			t.Errorf("%s lists %q, which no plain page uses; remove it", glossaryFile, w)
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
	if got := plainCheck("Press **Circumnavigate now** to see \"Nothing found\".", vocab).unknown; got["Circumnavigate"]+got["Nothing"] != 0 {
		t.Errorf("a bold label or a quoted message was judged as prose: %v", got)
	}
	if res.unknown["circumnavigated"] == 0 {
		t.Error("an unlisted word was not reported")
	}
	if res.unknown["deals"] != 0 || res.unknown["opened"] != 0 {
		t.Errorf("an inflection of a listed word was reported: %v", res.unknown)
	}
	for word, base := range map[string]string{"making": "make", "stopped": "stop", "cities": "city", "used": "use", "quickly": "quick"} {
		if _, ok := plainMatch(word, plainVocab{general: map[string]bool{base: true}}); !ok {
			t.Errorf("%q was not read as a form of %q", word, base)
		}
	}
	for word, base := range map[string]string{"thing": "the", "apply": "app", "band": "ban", "offer": "off", "reply": "rep", "bring": "br"} {
		if _, ok := plainMatch(word, plainVocab{general: map[string]bool{base: true}}); ok {
			t.Errorf("%q passed as a form of the unrelated word %q", word, base)
		}
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
	if ok, _ := plainEnrolment("<!-- prose:plain max-words=99999999999999999999 -->\n# A"); ok {
		t.Error("a marker whose limit does not parse was enrolled")
	}
	for rel, want := range map[string]string{
		"docs/handbook/records.md":                               "docs/handbook/plain-words.txt",
		"backend/internal/modules/knowledge/handbook/records.md": "docs/handbook/plain-words.txt",
		"docs/how-to/add-a-job.md":                               "docs/how-to/plain-words.txt",
		"docs/tutorials/getting-started.md":                      "docs/how-to/plain-words.txt",
		"README.md":                                              plainWordsFile,
		"docs/README.md":                                         plainWordsFile,
		"docs/principles/derive-the-obligation.md":               plainProjectFile,
	} {
		if got := plainPoolFor(rel); got != want {
			t.Errorf("%s reads %s, want %s", rel, got, want)
		}
	}
	if terms, legacy, problems := parseGlossary("| Term | Meaning |\n|---|---|\n| `pgvector` | Postgres vector search. |\n\n" +
		glossaryLegacyHeading + "\n\n`nonce`, `cron`\n"); len(problems) != 0 || len(terms) != 3 || legacy != 2 {
		t.Errorf("a meaning row and two legacy names were not all read: %v %d %v", terms, legacy, problems)
	}
	if _, _, problems := parseGlossary("| Term | Meaning |\n|---|---|\n| `pgvector` | Postgres vector search. |\n| boundary | edge |"); len(problems) != 1 {
		t.Errorf("a glossary row with a two-word meaning was not reported: %v", problems)
	}
	if ok, _ := plainEnrolment("# A\n`<!-- prose:plain -->`"); ok {
		t.Error("a page quoting the marker below its first line was enrolled")
	}
}
