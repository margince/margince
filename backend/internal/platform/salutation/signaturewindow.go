// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package salutation

import (
	"iter"
	"slices"

	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// SignatureLineCount bounds the signature read and its cost estimate.
const SignatureLineCount = 25

// SignatureWindow is the run of sender lines a signature can sit in, at most
// maxLines long. It opens at the last sign-off or the first sig-dash, whichever
// comes first, and keeps the head of a longer run.
//
// The head, because a signature puts its name, title and numbers at the top and
// its legal footer at the foot. Without either boundary the window is the tail.
func SignatureWindow(lines []string, maxLines int) []string {
	for window := range SignatureWindows(lines, maxLines) {
		return window
	}
	return nil
}

// SignatureWindows offers earlier sign-offs when a later company footer hides the signer.
func SignatureWindows(lines []string, maxLines int) iter.Seq[[]string] {
	return func(yield func([]string) bool) {
		if maxLines <= 0 {
			return
		}
		starts := signatureBoundaries(lines)
		for _, start := range slices.Backward(starts) {
			if !yield(lines[start:min(len(lines), start+maxLines)]) {
				return
			}
		}
		yield(lines[max(0, len(lines)-maxLines):])
	}
}

func signatureBoundaries(lines []string) []int {
	var starts []int
	dash := -1
	for i, line := range lines {
		if isClosing(line) {
			starts = append(starts, i)
		}
		if dash < 0 && textlang.IsSigDash(line) {
			dash = i
		}
	}
	if dash >= 0 && (len(starts) == 0 || dash < starts[len(starts)-1]) {
		for len(starts) > 0 && starts[len(starts)-1] >= dash {
			starts = starts[:len(starts)-1]
		}
		starts = append(starts, dash)
	}
	return starts
}
