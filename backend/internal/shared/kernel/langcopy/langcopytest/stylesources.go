// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package langcopytest

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

// The word lists are read from the files that own them on every run. So the Go
// check and the catalog gates cannot hold two different lists.
const (
	englishGatePath    = "frontend/src/i18n/copy-style.test.ts"
	vietnamesePagePath = "docs/reference/ui-copy-style-vi.md"
)

// englishPatternAnchors names where each mirrored rule's regex literal sits in
// the English catalog gate: the first regex literal after the anchor.
var englishPatternAnchors = map[string]string{
	ruleEnContraction: "const contraction =",
	ruleEnSpelling:    "const british =",
	ruleEnWe:          `it("never speaks as we"`,
	ruleEnI:           `it("says I only inside the onboarding conversation"`,
}

// sources is what the style rules read from outside the Go tree.
type sources struct {
	english  map[string]*regexp.Regexp
	retired  *regexp.Regexp
	ordinary *regexp.Regexp
}

var loadSources = sync.OnceValues(func() (sources, error) {
	root, err := repositoryRoot()
	if err != nil {
		return sources{}, err
	}
	gate, err := os.ReadFile(filepath.Join(root, englishGatePath))
	if err != nil {
		return sources{}, fmt.Errorf("reading the English copy gate: %w", err)
	}
	page, err := os.ReadFile(filepath.Join(root, vietnamesePagePath))
	if err != nil {
		return sources{}, fmt.Errorf("reading the Vietnamese style page: %w", err)
	}
	english, err := englishPatterns(string(gate))
	if err != nil {
		return sources{}, err
	}
	retired, ordinary, err := vietnameseVocabulary(string(page))
	if err != nil {
		return sources{}, err
	}
	return sources{english: english, retired: retired, ordinary: ordinary}, nil
})

// repositoryRoot is the parent of the go.mod above the test's working
// directory. Not gatekit.ModuleRoot: gatekit is for _test.go files only.
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

// regexLiteral is one JavaScript regex literal and its flags. A character
// class may hold a slash, so it is read as a unit.
var regexLiteral = regexp.MustCompile(`/((?:\\.|\[(?:\\.|[^\]\\])*\]|[^/\\\[\n])+)/([a-z]*)`)

func englishPatterns(gate string) (map[string]*regexp.Regexp, error) {
	patterns := make(map[string]*regexp.Regexp, len(englishPatternAnchors))
	for rule, anchor := range englishPatternAnchors {
		_, after, found := strings.Cut(gate, anchor)
		if !found {
			return nil, fmt.Errorf("%s no longer holds %s, which %s is read from", englishGatePath, anchor, rule)
		}
		literal := regexLiteral.FindStringSubmatch(after)
		if literal == nil {
			return nil, fmt.Errorf("%s has no regex literal after %s", englishGatePath, anchor)
		}
		compiled, err := jsRegex(literal[1], literal[2])
		if err != nil {
			return nil, fmt.Errorf("%s: the %s pattern: %w", englishGatePath, rule, err)
		}
		patterns[rule] = compiled
	}
	return patterns, nil
}

// jsRegex compiles a JavaScript pattern. The mirrored patterns use only syntax
// both engines read alike; one that does not fails to compile here.
func jsRegex(source, flags string) (*regexp.Regexp, error) {
	prefix := ""
	for _, flag := range flags {
		switch flag {
		case 'i':
			prefix = "(?i)"
		case 'u':
		default:
			return nil, fmt.Errorf("flag %q has no Go reading", flag)
		}
	}
	return regexp.Compile(prefix + source)
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
	_, section, _ := strings.Cut(page, "\n## Vocabulary\n")
	section, _, _ = strings.Cut(section, "\n## ")
	var words, kept []string
	header := true
	for _, line := range strings.Split(section, "\n") {
		if !strings.HasPrefix(line, "|") || strings.HasPrefix(line, "|---") {
			continue
		}
		if header {
			header = false
			continue
		}
		cells := strings.Split(line, "|")
		if len(cells) < 4 {
			continue
		}
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
	if len(words) == 0 {
		return nil, nil, fmt.Errorf("%s: the Vocabulary table's Never column sets no word in code", vietnamesePagePath)
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

// wholeWord matches an alternation with no letter on either side, in any case.
// It consumes the character on each side, which a yes-or-no match and a
// replacement by a space can both afford.
func wholeWord(alternation string) *regexp.Regexp {
	return regexp.MustCompile(`(?i)(?:^|[^\p{L}])(?:` + alternation + `)(?:$|[^\p{L}])`)
}
