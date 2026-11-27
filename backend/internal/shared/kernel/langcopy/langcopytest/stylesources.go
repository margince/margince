// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package langcopytest

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"sync"
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

// jsRegex is a JavaScript regex literal: its body and its flags.
type jsRegex struct{ source, flags string }

func (r jsRegex) compile() (*regexp.Regexp, error) { return jsPattern(r.source, r.flags) }

// styleSnapshot is what the style rules take from the frontend catalog gates
// and the Vietnamese page, as stylesources_gen.go holds it.
type styleSnapshot struct {
	patterns      map[string]jsRegex
	retiredWords  []string
	keptCompounds []string
}

// sources is the snapshot compiled for the style rules.
type sources struct {
	patterns map[string]*regexp.Regexp
	retired  *regexp.Regexp
	ordinary *regexp.Regexp
}

var loadSources = sync.OnceValues(func() (sources, error) {
	patterns := make(map[string]*regexp.Regexp, len(ownerSnapshot.patterns))
	for name, pattern := range ownerSnapshot.patterns {
		compiled, err := pattern.compile()
		if err != nil {
			return sources{}, fmt.Errorf("the %s pattern of stylesources_gen.go: %w", name, err)
		}
		patterns[name] = compiled
	}
	retired, ordinary, err := vocabulary(ownerSnapshot.retiredWords, ownerSnapshot.keptCompounds)
	if err != nil {
		return sources{}, fmt.Errorf("stylesources_gen.go: %w", err)
	}
	return sources{patterns: patterns, retired: retired, ordinary: ordinary}, nil
})

// word is the census's word(): an alternation with no letter on either side.
func word(alternation, flags string) jsRegex {
	return jsRegex{`(?<![\p{L}])(?:` + alternation + `)(?![\p{L}])`, flags}
}

// wholeWord is word() in any case, for a pattern this package writes itself.
func wholeWord(alternation string) *regexp.Regexp {
	pattern, err := word(alternation, "iu").compile()
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

// A lookaround atom: a class that is not negated, an escape, or one literal
// character. A metacharacter such as . is refused, since quoted it reads as a literal.
var (
	leadingBehind = regexp.MustCompile(`^\(\?<!(\[[^\]^][^\]]*\]|\\.|[^\\()\[\].^$*+?{}|])\)`)
	trailingAhead = regexp.MustCompile(`\(\?!(\[[^\]^][^\]]*\]|\\.|[^\\()\[\].^$*+?{}|])\)$`)
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

// An ASCII retired word is an English loan, which may take an English plural.
var printable = regexp.MustCompile(`^[\x20-\x7e]+$`)

// vocabulary reads the retired words as copy-style-vi.census.ts does, and keeps
// the compounds that hold one of them.
func vocabulary(words, kept []string) (retired, ordinary *regexp.Regexp, err error) {
	if len(words) == 0 {
		return nil, nil, errors.New("no Vietnamese word is retired")
	}
	alternatives := make([]string, len(words))
	for i, w := range words {
		alternatives[i] = regexp.QuoteMeta(w)
		if printable.MatchString(w) {
			alternatives[i] += "s?"
		}
	}
	retired = wholeWord(strings.Join(alternatives, "|"))
	var compounds []string
	for _, compound := range kept {
		if retired.MatchString(compound) {
			compounds = append(compounds, regexp.QuoteMeta(compound))
		}
	}
	if len(compounds) == 0 {
		return nil, nil, errors.New("no compound of a retired Vietnamese word is kept")
	}
	return retired, wholeWord(strings.Join(compounds, "|")), nil
}
