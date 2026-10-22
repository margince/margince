// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package editdistance counts the edits between two sequences, and finds the
// name a misspelling most likely meant.
package editdistance

import "slices"

// Levenshtein is the fewest insertions, deletions and substitutions of one
// element that turn a into b. It keeps two rows of the matrix, not all of it.
func Levenshtein[T comparable](a, b []T) int {
	previous := make([]int, len(b)+1)
	current := make([]int, len(b)+1)
	for j := range previous {
		previous[j] = j
	}
	for i := 1; i <= len(a); i++ {
		current[0] = i
		for j := 1; j <= len(b); j++ {
			substitution := previous[j-1]
			if a[i-1] != b[j-1] {
				substitution++
			}
			current[j] = min(previous[j]+1, current[j-1]+1, substitution)
		}
		previous, current = current, previous
	}
	return previous[len(b)]
}

// Nearest is the candidate fewest rune edits from name, when that is at most
// limit and not name itself. A tie goes to the first in sorted order, so the
// same input always gets the same answer.
func Nearest(name string, candidates []string, limit int) (string, bool) {
	best, bestDistance := "", limit+1
	for _, candidate := range slices.Sorted(slices.Values(candidates)) {
		if candidate == name {
			continue
		}
		if d := Levenshtein([]rune(name), []rune(candidate)); d < bestDistance {
			best, bestDistance = candidate, d
		}
	}
	return best, best != ""
}
