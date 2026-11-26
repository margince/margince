// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package langcopytest

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync"
)

// Each run reads the word lists and patterns from the files that own them. So
// the Go check and the catalog gates cannot hold two different lists.
const (
	englishGatePath      = "frontend/src/i18n/copy-style.test.ts"
	vietnameseCensusPath = "frontend/census/rules/copy-style-vi.census.ts"
	vietnamesePagePath   = "docs/reference/ui-copy-style-vi.md"

	// gatekit.TreeDigestVar of frontend and docs, named here because gatekit is
	// imported by tests only.
	frontendDigestVar = "TREE_DIGEST_FRONTEND"
	docsDigestVar     = "TREE_DIGEST_DOCS"
)

// Names of the patterns that are read and are not a rule of their own.
const (
	patternDash       = "dash"
	patternEnQuoted   = "en.quoted"
	patternViQuoted   = "vi.quoted"
	patternToiWe      = "vi.toi-chung-toi"
	patternSaysMe     = "vi.says-me"
	patternReflexive  = "vi.reflexive"
	patternWebAddress = "vi.web-address"
	patternFileName   = "vi.file-name"
)

// englishPatternLeads names the code right before each mirrored rule's regex
// literal in the English catalog gate. Spaces in a lead stand for any spacing.
var englishPatternLeads = map[string]string{
	ruleEnContraction: "const contraction =",
	ruleEnSpelling:    "const british =",
	ruleEnWe:          `it("never speaks as we", () => { expect( offenders(`,
	ruleEnI:           `it("says I only inside the onboarding conversation", () => { expect( offenders(`,
	patternEnQuoted:   "const unquoted = (value: string) => value.replace(",
	ruleEnAmpersand:   `const unquoted = (value: string) => value.replace(/“[^”]*”/g, ""); expect(offenders(`,
}

// vietnamesePatternLeads does the same for the Vietnamese census, where a
// pattern is a regex literal or a call of its word().
var vietnamesePatternLeads = map[string]string{
	ruleViToneOpen:        `"marks the tone of an open oa, oe and uy on the first vowel",`,
	ruleViToneClosed:      `"marks the tone on the second vowel before a final consonant",`,
	patternDash:           `"uses no en dash, em dash, double hyphen or spaced hyphen",`,
	ruleViExclamation:     `matching("uses no exclamation mark",`,
	ruleViToi:             `"says tôi only for the onboarding agent, a consent statement or an English me",`,
	patternToiWe:          `word("tôi"), { text: (value) => prose(value).replace(`,
	patternSaysMe:         `key.startsWith(voices.agentSpeaks) || spokenByTheReader(key) ||`,
	ruleViChungToi:        `"says chúng tôi only as the data controller or in a consent statement",`,
	ruleViChungTa:         `matching("never speaks as chúng ta or mình",`,
	patternReflexive:      `const REFLEXIVE =`,
	ruleViMemberAddress:   `"says quý khách and vui lòng only where an outsider reads",`,
	ruleViOutsiderAddress: `matching("never says bạn where an outsider reads",`,
	patternViQuoted:       `function unquoted(value: string): string { return prose(value).replace(`,
	patternWebAddress:     `const WEB_ADDRESS =`,
	patternFileName:       `const FILE_NAME =`,
}

// vietnamesePins are census code whose lookaround Go cannot read, ported by
// hand in stylerules.go. A change to one fails until its port follows.
var vietnamesePins = map[string]struct{ lead, code string }{
	"word": {`export function word(alternation: string, flags = "iu"): RegExp {`,
		"return new RegExp(`(?<![\\\\p{L}])(?:${alternation})(?![\\\\p{L}])`, flags);"},
	ruleViFormal: {`"never addresses anyone as quý vị, anh/chị or kính thưa",`,
		`word("quý vị|anh/chị|anh chị(?! em)|kính thưa"),`},
}

// sources is what the style rules read from outside the Go tree.
type sources struct {
	patterns map[string]*regexp.Regexp
	retired  *regexp.Regexp
	ordinary *regexp.Regexp
}

var loadSources = sync.OnceValues(func() (sources, error) {
	root, err := repositoryRoot()
	if err != nil {
		return sources{}, err
	}
	// The read is the declaration: Go's test cache keys on each variable. CI
	// sets it to a digest of that tree.
	os.Getenv(frontendDigestVar)
	os.Getenv(docsDigestVar)
	read := func(path string) (string, error) {
		body, readErr := os.ReadFile(filepath.Join(root, path))
		return string(body), readErr
	}
	gate, gateErr := read(englishGatePath)
	census, censusErr := read(vietnameseCensusPath)
	page, pageErr := read(vietnamesePagePath)
	if err := errors.Join(gateErr, censusErr, pageErr); err != nil {
		return sources{}, fmt.Errorf("reading the copy style sources: %w", err)
	}
	patterns := map[string]*regexp.Regexp{}
	for _, owner := range []struct {
		path, text string
		leads      map[string]string
	}{{englishGatePath, gate, englishPatternLeads}, {vietnameseCensusPath, census, vietnamesePatternLeads}} {
		for name, lead := range owner.leads {
			if patterns[name], err = patternAfter(owner.text, lead); err != nil {
				return sources{}, fmt.Errorf("%s: the %s pattern: %w", owner.path, name, err)
			}
		}
	}
	for name, pin := range vietnamesePins {
		if err := pinned(census, pin.lead, pin.code); err != nil {
			return sources{}, fmt.Errorf("%s: %s is ported by hand: %w", vietnameseCensusPath, name, err)
		}
	}
	retired, ordinary, err := vietnameseVocabulary(page)
	if err != nil {
		return sources{}, err
	}
	return sources{patterns: patterns, retired: retired, ordinary: ordinary}, nil
})

// repositoryRoot is the parent of the go.mod above the working directory. Not
// gatekit.ModuleRoot: it needs a testing.TB, and loadSources serves every test.
func repositoryRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("finding the module root: %w", err)
	}
	for {
		if info, statErr := os.Stat(filepath.Join(dir, "go.mod")); statErr == nil && !info.IsDir() {
			return filepath.Dir(dir), nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("no go.mod above the working directory, so the style sources cannot be found")
		}
		dir = parent
	}
}

var (
	// regexLiteral is one JavaScript regex literal and its flags. A character
	// class may hold a slash, so it is read as a unit.
	regexLiteral = regexp.MustCompile(`^/((?:\\.|\[(?:\\.|[^\]\\])*\]|[^/\\\[\n])+)/([a-z]*)`)
	wordCall     = regexp.MustCompile(`^word\(\s*("(?:\\.|[^"\\])*")\s*(?:,\s*"([a-z]*)"\s*)?\)`)
	spacing      = regexp.MustCompile(`\s+`)
)

// leadIn finds the one place a lead stands in a source and answers the code
// after it, with the spacing in between dropped.
func leadIn(source, lead string) (string, error) {
	pieces := strings.Fields(lead)
	for i, piece := range pieces {
		pieces[i] = regexp.QuoteMeta(piece)
	}
	found := regexp.MustCompile(strings.Join(pieces, `\s*`)).FindAllStringIndex(source, -1)
	if len(found) != 1 {
		return "", fmt.Errorf("%q stands there %d times, and must stand once", lead, len(found))
	}
	return strings.TrimLeft(source[found[0][1]:], " \t\n"), nil
}

// patternAfter compiles the regex literal or word() call right after a lead.
func patternAfter(source, lead string) (*regexp.Regexp, error) {
	code, err := leadIn(source, lead)
	if err != nil {
		return nil, err
	}
	if literal := regexLiteral.FindStringSubmatch(code); literal != nil {
		return jsPattern(literal[1], literal[2])
	}
	call := wordCall.FindStringSubmatch(code)
	if call == nil {
		return nil, fmt.Errorf("no regex literal or word() call follows %q", lead)
	}
	alternation, err := strconv.Unquote(call[1])
	if err != nil {
		return nil, fmt.Errorf("the word() argument %s is not a plain string: %w", call[1], err)
	}
	flags := call[2]
	if flags == "" {
		flags = "iu"
	}
	return word(alternation, flags)
}

// pinned fails unless code follows lead, spacing aside.
func pinned(source, lead, code string) error {
	after, err := leadIn(source, lead)
	if err != nil {
		return err
	}
	if want := spacing.ReplaceAllString(code, ""); !strings.HasPrefix(spacing.ReplaceAllString(after, ""), want) {
		return fmt.Errorf("the code after %q is no longer %s", lead, code)
	}
	return nil
}

// word is the census's word(): an alternation with no letter on either side.
func word(alternation, flags string) (*regexp.Regexp, error) {
	return jsPattern(`(?<![\p{L}])(?:`+alternation+`)(?![\p{L}])`, flags)
}

// wholeWord is word() in any case, for a pattern this package writes itself.
func wholeWord(alternation string) *regexp.Regexp {
	pattern, err := word(alternation, "iu")
	if err != nil {
		panic(err)
	}
	return pattern
}

// jsPattern compiles a JavaScript pattern. A one-character lookaround at an
// alternative's edge becomes that character consumed; any other is refused.
func jsPattern(source, flags string) (*regexp.Regexp, error) {
	prefix := ""
	for _, flag := range flags {
		switch flag {
		case 'i':
			prefix = "(?i)"
		case 'u', 'g':
		default:
			return nil, fmt.Errorf("flag %q has no Go reading", flag)
		}
	}
	if !strings.ContainsRune(flags, 'u') && strings.Contains(source, `\p`) {
		return nil, fmt.Errorf(`/%s/ uses \p without the u flag, where JavaScript reads the letter p`, source)
	}
	alternatives := topLevel(source)
	for i, alternative := range alternatives {
		rewritten, err := withoutLookarounds(alternative)
		if err != nil {
			return nil, fmt.Errorf("/%s/: %w", source, err)
		}
		alternatives[i] = rewritten
	}
	return regexp.Compile(prefix + strings.Join(alternatives, "|"))
}

// topLevel splits a pattern at each | outside a group or a class.
func topLevel(source string) []string {
	var parts []string
	depth, class, start := 0, false, 0
	for i := 0; i < len(source); i++ {
		switch c := source[i]; {
		case c == '\\':
			i++
		case class:
			class = c != ']'
		case c == '[':
			class = true
		case c == '(':
			depth++
		case c == ')':
			depth--
		case c == '|' && depth == 0:
			parts = append(parts, source[start:i])
			start = i + 1
		}
	}
	return append(parts, source[start:])
}

// A lookaround atom: a class that is not negated, an escape, or one character.
var (
	leadingBehind = regexp.MustCompile(`^\(\?<!(\[[^\]^][^\]]*\]|\\.|[^\\()\[\]])\)`)
	trailingAhead = regexp.MustCompile(`\(\?!(\[[^\]^][^\]]*\]|\\.|[^\\()\[\]])\)$`)
)

func withoutLookarounds(alternative string) (string, error) {
	if m := leadingBehind.FindStringSubmatch(alternative); m != nil {
		alternative = `(?:^|` + complement(m[1]) + `)` + alternative[len(m[0]):]
	}
	if m := trailingAhead.FindStringSubmatch(alternative); m != nil {
		alternative = alternative[:len(alternative)-len(m[0])] + `(?:$|` + complement(m[1]) + `)`
	}
	return jsBody(alternative)
}

// complement is a class of one character the atom does not match.
func complement(atom string) string {
	switch {
	case strings.HasPrefix(atom, "["):
		return "[^" + atom[1:]
	case strings.HasPrefix(atom, `\`):
		return "[^" + atom + "]"
	default:
		return "[^" + regexp.QuoteMeta(atom) + "]"
	}
}

// jsSpace is the class body of JavaScript's \s. Go's \s is ASCII alone, so
// a no-break space would read as a letter.
const jsSpace = `\t\n\v\f\r \p{Zs}\x{2028}\x{2029}\x{FEFF}`

// jsBody widens \s and \S to JavaScript's reading and refuses a lookaround
// left in the pattern.
func jsBody(pattern string) (string, error) {
	var out strings.Builder
	class := false
	for i := 0; i < len(pattern); i++ {
		c := pattern[i]
		switch {
		case c == '\\' && i+1 < len(pattern):
			i++
			switch next := pattern[i]; {
			case next == 's' && class:
				out.WriteString(jsSpace)
			case next == 's':
				out.WriteString("[" + jsSpace + "]")
			case next == 'S' && !class:
				out.WriteString("[^" + jsSpace + "]")
			case next == 'S':
				return "", errors.New(`\S inside a class has no Go reading`)
			default:
				out.WriteByte(c)
				out.WriteByte(next)
			}
			continue
		case class:
			class = c != ']'
		case c == '[':
			class = true
		case c == '(' && slices.ContainsFunc([]string{"(?=", "(?!", "(?<=", "(?<!"}, func(open string) bool {
			return strings.HasPrefix(pattern[i:], open)
		}):
			return "", errors.New("a lookaround inside an alternative, or over more than one character, has no Go reading")
		}
		out.WriteByte(c)
	}
	return out.String(), nil
}

var (
	codeSpan     = regexp.MustCompile("`([^`]+)`")
	keptCompound = regexp.MustCompile(`\(([^()]*) (?:keep theirs|keeps it)\)`)
	listJoin     = regexp.MustCompile(`, | and `)
	printable    = regexp.MustCompile(`^[\x20-\x7e]+$`)
)

// vietnameseVocabulary reads the Vocabulary table as copy-style-vi.census.ts
// does. A code span in the Never column is retired. A "(… keep theirs)" note
// names the compounds that keep it.
func vietnameseVocabulary(page string) (retired, ordinary *regexp.Regexp, err error) {
	_, section, found := strings.Cut(page, "\n## Vocabulary\n")
	if !found {
		return nil, nil, fmt.Errorf("%s has no Vocabulary section", vietnamesePagePath)
	}
	section, _, _ = strings.Cut(section, "\n## ")
	var words, kept []string
	header, ticks := 0, 0
	for _, line := range strings.Split(section, "\n") {
		if !strings.HasPrefix(line, "|") || strings.HasPrefix(line, "|---") {
			continue
		}
		cells := strings.Split(line, "|")
		switch {
		case header == 0 && len(cells) < 5:
			return nil, nil, fmt.Errorf("%s: the Vocabulary table has no Never column: %q", vietnamesePagePath, line)
		case header == 0:
			header = len(cells)
			continue
		case len(cells) != header:
			return nil, nil, fmt.Errorf("%s: the Vocabulary row %q has %d cells and the header %d",
				vietnamesePagePath, line, len(cells), header)
		}
		ticks += strings.Count(cells[3], "`")
		for _, m := range codeSpan.FindAllStringSubmatch(cells[3], -1) {
			alternative := regexp.QuoteMeta(m[1])
			if printable.MatchString(m[1]) {
				alternative += "s?"
			}
			words = append(words, alternative)
		}
		for _, m := range keptCompound.FindAllStringSubmatch(cells[3], -1) {
			kept = append(kept, listJoin.Split(m[1], -1)...)
		}
	}
	if len(words) == 0 || ticks != 2*len(words) {
		return nil, nil, fmt.Errorf("%s: the Never column of the Vocabulary table holds %d backticks and %d words in code; "+
			"every retired word sits between two", vietnamesePagePath, ticks, len(words))
	}
	retired = wholeWord(strings.Join(words, "|"))
	var compounds []string
	for _, compound := range kept {
		if retired.MatchString(compound) {
			compounds = append(compounds, regexp.QuoteMeta(compound))
		}
	}
	if len(compounds) == 0 {
		return nil, nil, fmt.Errorf("%s: the Vocabulary table keeps no compound of a retired word", vietnamesePagePath)
	}
	return retired, wholeWord(strings.Join(compounds, "|")), nil
}
