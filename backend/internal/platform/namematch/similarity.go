// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package namematch

import "github.com/margince/margince/backend/internal/shared/kernel/textcut"

// The dedupe fuzzy tier's metric (PO-F-1/PO-F-2 `name_sim`) is standard
// Jaro-Winkler: prefix scale 0.1, max prefix 4, no boost threshold
// (PO-PARAM-JW-1). It is pinned so the spec's worked examples reproduce.
const (
	jaroWinklerPrefixScale = 0.1
	jaroWinklerMaxPrefix   = 4
)

// ScoringMaxRunes bounds what the metric compares. Jaro is quadratic, and a
// display name has no length limit. The scoring runs under the company-name
// lock, so an unbounded score would stall every company-name writer.
//
// The cap is here and not in Fold, because Fold also builds exact-match keys.
// Truncating there would make two long names one key.
const ScoringMaxRunes = 256

// boundedForScoring caps one side of a comparison. Two names that differ only
// past the bound score as identical, which for names this long is the same
// answer any metric would give.
func boundedForScoring(s string) string {
	return textcut.Runes(s, ScoringMaxRunes)
}

// JaroWinkler applies the Winkler prefix boost unconditionally. The pinned
// variant has no boost threshold (PO-PARAM-JW-1).
func JaroWinkler(a, b string) float64 {
	a, b = boundedForScoring(a), boundedForScoring(b)
	j := jaro(a, b)
	if j == 0 {
		return 0
	}
	ra, rb := []rune(a), []rune(b)
	prefix := 0
	for prefix < len(ra) && prefix < len(rb) && prefix < jaroWinklerMaxPrefix && ra[prefix] == rb[prefix] {
		prefix++
	}
	return j + float64(prefix)*jaroWinklerPrefixScale*(1-j)
}

// jaro is the standard Jaro similarity: matches inside a half-length
// window, discounted by half the transpositions among them.
func jaro(a, b string) float64 {
	ra, rb := []rune(a), []rune(b)
	if len(ra) == 0 && len(rb) == 0 {
		return 1
	}
	if len(ra) == 0 || len(rb) == 0 {
		return 0
	}

	window := max(max(len(ra), len(rb))/2-1, 0)

	matchedA := make([]bool, len(ra))
	matchedB := make([]bool, len(rb))
	matches := 0
	for i, r := range ra {
		lo := max(0, i-window)
		hi := min(len(rb)-1, i+window)
		for j := lo; j <= hi; j++ {
			if matchedB[j] || rb[j] != r {
				continue
			}
			matchedA[i], matchedB[j] = true, true
			matches++
			break
		}
	}
	if matches == 0 {
		return 0
	}

	// Transpositions: matched runes that pair up out of order. Each such
	// pair is counted twice by this walk, hence the halving below.
	transpositions := 0
	k := 0
	for i := range ra {
		if !matchedA[i] {
			continue
		}
		for !matchedB[k] {
			k++
		}
		if ra[i] != rb[k] {
			transpositions++
		}
		k++
	}

	m := float64(matches)
	return (m/float64(len(ra)) + m/float64(len(rb)) + (m-float64(transpositions)/2)/m) / 3
}
