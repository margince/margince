// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package credentialmask

import (
	"bytes"
	"regexp/syntax"
	"slices"
	"strings"
	"unicode"
)

// A rule's cues are strings one of which every match of the rule contains. They
// are read off the rule's own pattern, so a rule never runs where it cannot
// match and a new label is covered without a second list.
//
// Text and cues are compared case-folded, so a cue also holds for a pattern
// that matches case-sensitively. A rule whose pattern yields no cues runs on
// every text.

// maxCueSet bounds a set of exact strings; a larger set is given up for the
// cues of one of its parts.
const maxCueSet = 64

// maxCueClass is the largest character class read as a set of exact strings.
const maxCueClass = 8

// cuesOf derives the cues of one pattern.
func cuesOf(pattern string) [][]byte {
	re, err := syntax.Parse(pattern, syntax.Perl)
	if err != nil {
		panic("credentialmask: a rule pattern does not parse: " + err.Error())
	}
	set := required(re.Simplify())
	if set == nil {
		return nil
	}
	stems := make([]string, 0, len(set.strings))
	for _, cue := range set.strings {
		stems = append(stems, stem(cue))
	}
	slices.Sort(stems)
	stems = slices.Compact(stems)
	cues := make([][]byte, 0, len(stems))
	for _, cue := range stems {
		if !slices.ContainsFunc(stems, func(other string) bool { return other != cue && strings.Contains(cue, other) }) {
			cues = append(cues, []byte(cue))
		}
	}
	return cues
}

// stemLength is how much of a cue is looked for. Every part of a string a match
// must contain is one it must contain too, and a short stem covers the spelling
// variants of one label at once.
const stemLength = 4

func stem(cue string) string {
	for i := range cue {
		if i >= stemLength {
			return cue[:i]
		}
	}
	return cue
}

// cueSet is a set of folded strings. exact means the part matches exactly
// these strings; otherwise every match only contains one of them.
type cueSet struct {
	strings []string
	exact   bool
}

// required returns what every match of re must contain, or nil when nothing
// can be said.
func required(re *syntax.Regexp) *cueSet {
	switch re.Op {
	case syntax.OpLiteral:
		return &cueSet{strings: []string{fold(string(re.Rune))}, exact: true}
	case syntax.OpCharClass:
		return classCues(re.Rune)
	case syntax.OpEmptyMatch:
		return &cueSet{strings: []string{""}, exact: true}
	case syntax.OpCapture:
		return required(re.Sub[0])
	case syntax.OpPlus:
		return inexact(required(re.Sub[0]))
	case syntax.OpQuest:
		return questCues(required(re.Sub[0]))
	case syntax.OpRepeat:
		if re.Min == 0 {
			return nil
		}
		return inexact(required(re.Sub[0]))
	case syntax.OpAlternate:
		return alternateCues(re.Sub)
	case syntax.OpConcat:
		return concatCues(re.Sub)
	default:
		return nil
	}
}

func classCues(ranges []rune) *cueSet {
	var runes []string
	for i := 0; i < len(ranges); i += 2 {
		for r := ranges[i]; r <= ranges[i+1]; r++ {
			if len(runes) == maxCueClass {
				return nil
			}
			runes = append(runes, fold(string(r)))
		}
	}
	slices.Sort(runes)
	return &cueSet{strings: slices.Compact(runes), exact: true}
}

func inexact(set *cueSet) *cueSet {
	if set == nil || slices.Contains(set.strings, "") {
		return nil
	}
	return &cueSet{strings: set.strings}
}

func questCues(set *cueSet) *cueSet {
	if set == nil || !set.exact || len(set.strings) >= maxCueSet {
		return nil
	}
	return &cueSet{strings: append(slices.Clone(set.strings), ""), exact: true}
}

func alternateCues(subs []*syntax.Regexp) *cueSet {
	union := &cueSet{exact: true}
	for _, sub := range subs {
		set := required(sub)
		if set == nil {
			return nil
		}
		union.strings = append(union.strings, set.strings...)
		union.exact = union.exact && set.exact
	}
	slices.Sort(union.strings)
	union.strings = slices.Compact(union.strings)
	return union
}

// concatCues joins each run of exact parts into the strings it spells, then
// keeps the part whose shortest cue is longest.
func concatCues(subs []*syntax.Regexp) *cueSet {
	parts, exact := concatParts(subs)
	if exact && len(parts) == 1 {
		return parts[0]
	}
	var best *cueSet
	for _, part := range parts {
		if usable := inexact(part); usable != nil && (best == nil || shortest(usable) > shortest(best)) {
			best = usable
		}
	}
	return best
}

// concatParts is the cue set of each part of a concatenation, a run of exact
// parts joined into one. exact reports whether every part was exact.
func concatParts(subs []*syntax.Regexp) (parts []*cueSet, exact bool) {
	exact = len(subs) > 0
	var run *cueSet
	for _, sub := range subs {
		set := required(sub)
		exact = exact && set != nil && set.exact
		switch {
		case set == nil:
			parts, run = flushRun(parts, run), nil
		case !set.exact:
			parts, run = append(flushRun(parts, run), set), nil
		case run == nil:
			run = set
		default:
			if joined := product(run, set); joined != nil {
				run = joined
			} else {
				parts, run = flushRun(parts, run), set
			}
		}
	}
	return flushRun(parts, run), exact
}

func flushRun(parts []*cueSet, run *cueSet) []*cueSet {
	if run == nil {
		return parts
	}
	return append(parts, run)
}

func product(left, right *cueSet) *cueSet {
	if len(left.strings)*len(right.strings) > maxCueSet {
		return nil
	}
	joined := &cueSet{exact: true}
	for _, l := range left.strings {
		for _, r := range right.strings {
			joined.strings = append(joined.strings, l+r)
		}
	}
	return joined
}

func shortest(set *cueSet) int {
	length := -1
	for _, cue := range set.strings {
		if length < 0 || len(cue) < length {
			length = len(cue)
		}
	}
	return length
}

// fold maps every rune to the smallest rune of its case orbit, the way (?i)
// matches: K, k and the Kelvin sign fold alike, and so do S, s and the long s.
func fold(s string) string {
	return string(foldBytes([]byte(s)))
}

func foldBytes(text []byte) []byte {
	return bytes.Map(func(r rune) rune {
		if r <= unicode.MaxASCII {
			if 'a' <= r && r <= 'z' {
				return r - 'a' + 'A'
			}
			return r
		}
		least := r
		for f := unicode.SimpleFold(r); f != r; f = unicode.SimpleFold(f) {
			least = min(least, f)
		}
		return least
	}, text)
}

// cueScan answers whether one folded text holds a cue, and asks each cue once:
// rules share their labels, so most cues are asked by several.
type cueScan struct {
	folded []byte
	seen   map[string]bool
}

func newCueScan(text []byte) *cueScan {
	return &cueScan{folded: foldBytes(text), seen: map[string]bool{}}
}

// mayMatch reports whether the text holds one of the cues, or the rule has
// none.
func (c *cueScan) mayMatch(cues [][]byte) bool {
	if cues == nil {
		return true
	}
	for _, cue := range cues {
		held, asked := c.seen[string(cue)]
		if !asked {
			held = bytes.Contains(c.folded, cue)
			c.seen[string(cue)] = held
		}
		if held {
			return true
		}
	}
	return false
}
